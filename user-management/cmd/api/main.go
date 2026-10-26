// Command api runs the User Management REST API.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/auth"
	httpadapter "github.com/jingjoeh/test-assignment/user-management/internal/adapter/http"
	mongoadapter "github.com/jingjoeh/test-assignment/user-management/internal/adapter/mongo"
	"github.com/jingjoeh/test-assignment/user-management/internal/service"
	"github.com/jingjoeh/test-assignment/user-management/internal/worker"
)

const (
	connectTimeout    = 10 * time.Second
	dbCallTimeout     = 5 * time.Second
	disconnectTimeout = 5 * time.Second
	userCountInterval = 10 * time.Second // req. 6

	// HTTP server timeouts (A5a, D17). Constants, not configuration.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), logger, os.Getenv); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// run wires the application and serves until the server fails or ctx is
// done. Cleanup (stopping the worker and ticker, disconnecting Mongo) is
// deferred, so it runs however run returns.
func run(ctx context.Context, logger *slog.Logger, getenv func(string) string) error {
	// Config is checked before anything else, so a bad JWT_SECRET fails
	// without touching the database.
	cfg, err := LoadConfig(getenv)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	connectCtx, cancelConnect := context.WithTimeout(ctx, connectTimeout)
	defer cancelConnect()
	client, err := mongoadapter.Connect(connectCtx, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
		defer cancel()
		if err := client.Disconnect(disconnectCtx); err != nil {
			logger.Error("mongo disconnect failed", "error", err)
			return
		}
		logger.Info("mongo disconnected")
	}()

	repo := mongoadapter.NewUserRepository(client.Database(cfg.MongoDB), dbCallTimeout)
	if err := repo.EnsureIndexes(ctx); err != nil {
		return err
	}

	tokens := auth.NewJWT(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	authService, err := service.NewAuthService(repo, auth.NewBcryptHasher(bcrypt.DefaultCost), tokens)
	if err != nil {
		return err
	}
	userService := service.NewUserService(repo)
	handler := httpadapter.NewRouter(authService, userService, tokens, logger)

	ticker := time.NewTicker(userCountInterval)
	defer ticker.Stop()
	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go worker.RunUserCount(workerCtx, ticker.C, repo, logger)

	srv := newServer(":"+cfg.Port, handler)
	logger.Info("server listening", "addr", srv.Addr, "mongo_db", cfg.MongoDB)
	return srv.ListenAndServe()
}

// newServer returns an http.Server with explicit timeouts; the zero values
// would mean no limits at all (A5a).
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

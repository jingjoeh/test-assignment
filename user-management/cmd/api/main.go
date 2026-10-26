// Command api runs the User Management REST API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	shutdownTimeout   = 15 * time.Second // longer than writeTimeout, so in-flight requests can finish
	userCountInterval = 10 * time.Second // req. 6

	// HTTP server timeouts (A5a, D17). Constants, not configuration.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// SIGINT (Ctrl+C) and SIGTERM (docker stop) start a graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// After the first signal, restore the default behavior, so a second
		// Ctrl+C kills the process if the shutdown hangs.
		<-ctx.Done()
		stop()
	}()

	if err := run(ctx, logger, os.Getenv); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}

// run wires the application and serves until the server fails or ctx is
// done. On ctx done it shuts down in order: the HTTP server (letting
// in-flight requests finish), then the worker, then Mongo. The cleanup is
// deferred, so it also runs when startup fails part-way.
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
	// The worker gets its own context, not ctx, so it keeps running while
	// in-flight requests drain and is stopped explicitly after the server.
	workerCtx, cancelWorker := context.WithCancel(context.WithoutCancel(ctx))
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.RunUserCount(workerCtx, ticker.C, repo, logger)
	}()
	stopWorker := func() {
		cancelWorker()
		<-workerDone // Count may be mid-call; wait before Mongo disconnects
	}
	defer stopWorker()

	srv := newServer(":"+cfg.Port, handler)
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("http listen: %w", err)
	}
	logger.Info("server listening", "addr", ln.Addr().String(), "mongo_db", cfg.MongoDB)

	if err := serve(ctx, srv, ln, shutdownTimeout, logger); err != nil {
		return err
	}
	stopWorker()
	logger.Info("worker stopped")
	return nil // the deferred disconnect closes Mongo last
}

// serve runs srv on ln until ctx is done, then shuts it down gracefully:
// it stops accepting connections and waits up to timeout for in-flight
// requests to finish. If they don't finish in time, it force-closes their
// connections before returning, so the caller can release shared resources
// (the worker, Mongo) without clients still waiting on them. It returns an
// error if the server fails before ctx is done or the shutdown times out.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration, logger *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown started", "timeout", timeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Shutdown leaves connections that are still active open when it
		// times out; close them so their clients get an immediate error.
		logger.Warn("shutdown timed out, closing connections", "error", err)
		_ = srv.Close()
		return fmt.Errorf("http shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	logger.Info("http server stopped")
	return nil
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

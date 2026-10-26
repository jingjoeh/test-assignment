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
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/auth"
	grpcadapter "github.com/jingjoeh/test-assignment/user-management/internal/adapter/grpc"
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

// run wires the application and serves HTTP and gRPC until one of them fails
// or ctx is done. It then shuts down in order: both servers in parallel
// (letting in-flight requests finish), then the worker, then Mongo. The
// cleanup is deferred, so it also runs when startup fails part-way.
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
	grpcSrv := grpcadapter.NewServer(authService, userService, tokens, logger)
	grpcLn, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		ln.Close()
		return fmt.Errorf("grpc listen: %w", err)
	}
	logger.Info("server listening", "addr", ln.Addr().String(), "grpc_addr", grpcLn.Addr().String(), "mongo_db", cfg.MongoDB)

	if err := serveAll(ctx, logger,
		func(ctx context.Context) error { return serve(ctx, srv, ln, shutdownTimeout, logger) },
		func(ctx context.Context) error { return serveGRPC(ctx, grpcSrv, grpcLn, shutdownTimeout, logger) },
	); err != nil {
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

// serveAll runs every server function until ctx is done or one of them
// fails. A failure cancels the others, so they shut down too. Shutdowns run
// in parallel, so the total stays within one shutdown timeout. It returns
// every error that occurred.
func serveAll(ctx context.Context, logger *slog.Logger, servers ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make([]error, len(servers))
	var wg sync.WaitGroup
	for i, run := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if errs[i] = run(ctx); errs[i] != nil {
				logger.Error("server failed, stopping the others", "error", errs[i])
				cancel()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// serveGRPC runs srv on ln until ctx is done, then stops it gracefully: no
// new calls are accepted and in-flight calls get up to timeout to finish,
// after which their connections are closed. It mirrors serve for HTTP.
func serveGRPC(ctx context.Context, srv *grpc.Server, ln net.Listener, timeout time.Duration, logger *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("grpc server: %w", err)
	case <-ctx.Done():
	}

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-stopped:
	case <-timer.C:
		// Stop closes every connection, so clients get an error at once. It
		// runs in the background because it can block: the pending
		// GracefulStop holds the server's lock while it waits for a stuck
		// handler, and Stop needs that lock again after closing the
		// connections (grpc-go v1.84, Server.stop). Stop always closes the
		// connections first, since GracefulStop only keeps the lock once they
		// are gone. Like a stuck HTTP handler, the rest ends when the process
		// exits.
		logger.Warn("grpc shutdown timed out, closing connections")
		go srv.Stop()
		return fmt.Errorf("grpc shutdown: %w", context.DeadlineExceeded)
	}
	logger.Info("grpc server stopped")
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

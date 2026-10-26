package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/grpc/userpb"
)

// blockingUserService answers GetUser only when released, so a test can hold
// a call in flight across the start of a shutdown.
type blockingUserService struct {
	userpb.UnimplementedUserServiceServer
	started chan struct{}
	release chan struct{}
}

func (s *blockingUserService) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	close(s.started)
	<-s.release
	return &userpb.GetUserResponse{User: &userpb.User{Id: req.GetId()}}, nil
}

func startServeGRPC(t *testing.T, ctx context.Context, svc userpb.UserServiceServer, timeout time.Duration) (string, <-chan error, *bytes.Buffer) {
	t.Helper()
	srv := grpc.NewServer()
	userpb.RegisterUserServiceServer(srv, svc)
	ln := listen(t)
	logs := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- serveGRPC(ctx, srv, ln, timeout, slog.New(slog.NewJSONHandler(logs, nil))) }()
	return ln.Addr().String(), done, logs
}

func dialGRPC(t *testing.T, addr string) userpb.UserServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return userpb.NewUserServiceClient(conn)
}

func TestServeGRPCLetsInFlightCallFinish(t *testing.T) {
	svc := &blockingUserService{started: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	addr, done, logs := startServeGRPC(t, ctx, svc, 10*time.Second)
	client := dialGRPC(t, addr)

	type result struct {
		id  string
		err error
	}
	resp := make(chan result, 1)
	go func() {
		r, err := client.GetUser(context.Background(), &userpb.GetUserRequest{Id: "u1"})
		resp <- result{id: r.GetUser().GetId(), err: err}
	}()

	<-svc.started
	cancel()
	select {
	case err := <-done:
		t.Fatalf("serveGRPC returned (%v) while a call was still in flight", err)
	default:
	}

	close(svc.release)
	if got := <-resp; got.err != nil || got.id != "u1" {
		t.Fatalf("in-flight call = %+v, want success for u1", got)
	}
	if err := waitErr(t, done); err != nil {
		t.Fatalf("serveGRPC = %v, want nil", err)
	}
	if !strings.Contains(logs.String(), "grpc server stopped") {
		t.Errorf("logs missing \"grpc server stopped\":\n%s", logs)
	}
}

func TestServeGRPCShutdownTimeout(t *testing.T) {
	svc := &blockingUserService{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(svc.release) })
	ctx, cancel := context.WithCancel(context.Background())
	addr, done, logs := startServeGRPC(t, ctx, svc, time.Millisecond)
	client := dialGRPC(t, addr)

	callErr := make(chan error, 1)
	go func() {
		_, err := client.GetUser(context.Background(), &userpb.GetUserRequest{Id: "u1"})
		callErr <- err
	}()
	<-svc.started
	cancel()

	err := waitErr(t, done)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "grpc shutdown") {
		t.Fatalf("serveGRPC = %v, want a grpc shutdown deadline error", err)
	}
	if !strings.Contains(logs.String(), "grpc shutdown timed out, closing connections") {
		t.Errorf("logs missing the timeout warning:\n%s", logs)
	}
	select {
	case err := <-callErr:
		if err == nil {
			t.Error("stuck call succeeded, want an error after its connection was closed")
		}
	case <-time.After(10 * time.Second): // failsafe only
		t.Fatal("stuck call's connection was not closed")
	}
}

func TestServeGRPCReturnsServerError(t *testing.T) {
	ln := listen(t)
	ln.Close()
	done := make(chan error, 1)
	go func() {
		done <- serveGRPC(context.Background(), grpc.NewServer(), ln, time.Second, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	}()
	if err := waitErr(t, done); err == nil || !strings.Contains(err.Error(), "grpc server") {
		t.Fatalf("serveGRPC = %v, want a grpc server error", err)
	}
}

func TestServeAllStopsOthersWhenOneFails(t *testing.T) {
	boom := errors.New("listener broke")
	otherStopped := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- serveAll(context.Background(), slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
			func(context.Context) error { return boom },
			func(ctx context.Context) error {
				<-ctx.Done() // only returns because the failure cancelled it
				close(otherStopped)
				return nil
			},
		)
	}()

	err := waitErr(t, done)
	if !errors.Is(err, boom) {
		t.Fatalf("serveAll = %v, want %v", err, boom)
	}
	select {
	case <-otherStopped:
	default:
		t.Error("the other server was not stopped")
	}
}

func TestServeAllCleanShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waitForCtx := func(ctx context.Context) error { <-ctx.Done(); return nil }
	done := make(chan error, 1)
	go func() {
		done <- serveAll(ctx, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), waitForCtx, waitForCtx)
	}()
	cancel()
	if err := waitErr(t, done); err != nil {
		t.Fatalf("serveAll = %v, want nil", err)
	}
}

func TestServeAllJoinsErrors(t *testing.T) {
	a, b := errors.New("http failed"), errors.New("grpc failed")
	err := serveAll(context.Background(), slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		func(context.Context) error { return a },
		func(context.Context) error { return b },
	)
	if !errors.Is(err, a) || !errors.Is(err, b) {
		t.Fatalf("serveAll = %v, want both errors", err)
	}
}

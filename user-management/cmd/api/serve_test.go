package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// These tests run serve on a real TCP listener. They are ordered with
// channels, not sleeps: a handler reports when a request has started and
// blocks until the test releases it.

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func startServe(t *testing.T, ctx context.Context, h http.Handler, ln net.Listener, timeout time.Duration) (<-chan error, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, newServer(ln.Addr().String(), h), ln, timeout, slog.New(slog.NewJSONHandler(logs, nil)))
	}()
	return done, logs
}

func waitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second): // failsafe only
		t.Fatal("server did not return")
		return nil
	}
}

// C4 Verify: a request that is in flight when shutdown starts still completes,
// and serve returns only after it has.
func TestServeLetsInFlightRequestFinish(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done, logs := startServe(t, ctx, h, ln, 10*time.Second)

	type result struct {
		status int
		body   string
		err    error
	}
	resp := make(chan result, 1)
	go func() {
		r, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			resp <- result{err: err}
			return
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		resp <- result{status: r.StatusCode, body: string(b)}
	}()

	<-started // the request is now in flight
	cancel()  // start the shutdown, as a signal would

	// serve must still be waiting for the in-flight request.
	select {
	case err := <-done:
		t.Fatalf("serve returned (%v) while a request was still in flight", err)
	default:
	}

	close(release)
	got := <-resp
	if got.err != nil || got.status != http.StatusOK || got.body != "finished" {
		t.Fatalf("in-flight request = %+v, want 200 \"finished\"", got)
	}
	if err := waitErr(t, done); err != nil {
		t.Fatalf("serve = %v, want nil after a clean shutdown", err)
	}
	for _, msg := range []string{"shutdown started", "http server stopped"} {
		if !strings.Contains(logs.String(), msg) {
			t.Errorf("logs missing %q:\n%s", msg, logs)
		}
	}

	// After shutdown, new connections are refused.
	if _, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
		t.Error("request after shutdown succeeded, want a connection error")
	}
}

func TestServeIdleShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done, _ := startServe(t, ctx, http.NotFoundHandler(), listen(t), 10*time.Second)
	cancel()
	if err := waitErr(t, done); err != nil {
		t.Fatalf("serve = %v, want nil", err)
	}
}

// If in-flight requests don't finish within the timeout, serve force-closes
// their connections and reports the timeout.
func TestServeShutdownTimeout(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) }) // the stuck handler itself is never released by serve
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done, logs := startServe(t, ctx, h, ln, time.Millisecond)

	clientErr := make(chan error, 1)
	go func() {
		r, err := http.Get("http://" + ln.Addr().String() + "/stuck")
		if err == nil {
			r.Body.Close()
		}
		clientErr <- err
	}()
	<-started
	cancel()

	err := waitErr(t, done)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "http shutdown") {
		t.Fatalf("serve = %v, want an http shutdown deadline error", err)
	}
	if !strings.Contains(logs.String(), "shutdown timed out, closing connections") {
		t.Errorf("logs missing the timeout warning:\n%s", logs)
	}

	// The stuck request's connection was closed, so its client gets an error
	// now instead of waiting for a response that will never come.
	select {
	case err := <-clientErr:
		if err == nil {
			t.Error("stuck request succeeded, want a closed-connection error")
		}
	case <-time.After(10 * time.Second): // failsafe only
		t.Fatal("stuck request's connection was not closed")
	}
}

// A server that can't serve (here: its listener is already closed) returns
// the error instead of waiting for a signal.
func TestServeReturnsServerError(t *testing.T) {
	ln := listen(t)
	ln.Close()
	done, _ := startServe(t, context.Background(), http.NotFoundHandler(), ln, time.Second)

	if err := waitErr(t, done); err == nil || !strings.Contains(err.Error(), "http server") {
		t.Fatalf("serve = %v, want an http server error", err)
	}
}

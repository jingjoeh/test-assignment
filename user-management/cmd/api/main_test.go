package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewServerSetsTimeouts(t *testing.T) {
	srv := newServer(":0", http.NotFoundHandler())

	for name, got := range map[string]time.Duration{
		"ReadHeaderTimeout": srv.ReadHeaderTimeout,
		"ReadTimeout":       srv.ReadTimeout,
		"WriteTimeout":      srv.WriteTimeout,
		"IdleTimeout":       srv.IdleTimeout,
	} {
		if got <= 0 {
			t.Errorf("%s = %s, want a positive timeout", name, got)
		}
	}
	if srv.ReadHeaderTimeout != 5*time.Second || srv.ReadTimeout != 10*time.Second ||
		srv.WriteTimeout != 10*time.Second || srv.IdleTimeout != 60*time.Second {
		t.Errorf("timeouts = %s/%s/%s/%s, want 5s/10s/10s/60s (A5a)",
			srv.ReadHeaderTimeout, srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout)
	}
}

// B8 Verify: a short JWT_SECRET fails during config loading, before any
// connection to Mongo is attempted. The URI points at a port nothing listens
// on; reaching it would give a connection error instead, after the connect
// timeout.
func TestRunRejectsShortSecretBeforeConnecting(t *testing.T) {
	env := map[string]string{
		"MONGO_URI":  "mongodb://127.0.0.1:1",
		"MONGO_DB":   "users",
		"JWT_SECRET": "too-short",
		"JWT_ISSUER": "user-management",
	}
	logs := &bytes.Buffer{}

	start := time.Now()
	err := run(context.Background(), slog.New(slog.NewJSONHandler(logs, nil)), func(k string) string { return env[k] })

	if err == nil || !strings.Contains(err.Error(), "invalid configuration") || !strings.Contains(err.Error(), "JWT_SECRET must be at least 32 bytes") {
		t.Fatalf("run error = %v, want an invalid configuration error about JWT_SECRET", err)
	}
	if strings.Contains(err.Error(), "mongo") {
		t.Errorf("run error = %v, must not come from Mongo", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("run took %s, want it to fail immediately without connecting", elapsed)
	}
}

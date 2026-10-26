package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var errTest = errors.New("test: repository down")

// requestLogs returns the "http request" entries from JSON slog output.
func requestLogs(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line is not JSON: %v; line = %s", err, line)
		}
		if e["msg"] == "http request" {
			entries = append(entries, e)
		}
	}
	return entries
}

// steppingClock returns times that advance by step on every call.
func steppingClock(step time.Duration) func() time.Time {
	t := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(step)
		return t
	}
}

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
	}{
		{"explicit status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }, 201},
		{"no WriteHeader, no body", func(w http.ResponseWriter, r *http.Request) {}, 200},
		{"body without WriteHeader", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }, 200},
		{"first status wins", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			w.WriteHeader(http.StatusInternalServerError) // ignored by net/http too
		}, 418},
		{"status after body is ignored", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
			w.WriteHeader(http.StatusNotFound)
		}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := &bytes.Buffer{}
			h := logging(slog.New(slog.NewJSONHandler(logs, nil)), steppingClock(25*time.Millisecond))(tt.handler)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/users/abc?token=secret", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("client got status %d, want %d", rec.Code, tt.wantStatus)
			}
			entries := requestLogs(t, logs)
			if len(entries) != 1 {
				t.Fatalf("got %d log entries, want exactly 1: %s", len(entries), logs)
			}
			e := entries[0]
			if e["method"] != "POST" || e["path"] != "/users/abc" || e["status"] != float64(tt.wantStatus) || e["duration_ms"] != float64(25) {
				t.Errorf("entry = %v, want POST /users/abc, status %d, duration_ms 25", e, tt.wantStatus)
			}
			if strings.Contains(logs.String(), "secret") {
				t.Errorf("query string leaked into the log: %s", logs)
			}
		})
	}
}

func TestStatusWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	if got := (&statusWriter{ResponseWriter: rec}).Unwrap(); got != rec {
		t.Errorf("Unwrap = %v, want the wrapped writer", got)
	}
}

// B5 Verify: through the real router, every request produces exactly one log
// entry with method, path, status and duration, including public routes,
// error responses, JWT rejections and the JSON 404/405.
func TestRouterLogsEveryRequestOnce(t *testing.T) {
	s := newTestServer(t, nil)
	alice := s.registerUser(t, "Alice", "alice@example.com")
	s.logs.Reset()

	requests := []struct {
		method, path, body, token string
	}{
		{http.MethodPost, "/auth/register", `{"name":"Bob","email":"bob@example.com","password":"s3cret-pass"}`, ""}, // 201 public
		{http.MethodPost, "/auth/login", `{"email":"bob@example.com","password":"wrong-pass"}`, ""},                  // 401 public
		{http.MethodPost, "/auth/register", `{`, ""},                                                                 // 400 bad JSON
		{http.MethodGet, "/users", "", ""},                                                                           // 401 from RequireAuth
		{http.MethodGet, "/users", "", anyToken},                                                                     // 200
		{http.MethodGet, "/users/not-an-id", "", anyToken},                                                           // 400
		{http.MethodPatch, "/users/" + alice, `{}`, anyToken},                                                        // 400
		{http.MethodDelete, "/users/" + alice, "", anyToken},                                                         // 204
		{http.MethodGet, "/nope", "", ""},                                                                            // 404
		{http.MethodGet, "/auth/login", "", ""},                                                                      // 405
	}
	var wantStatuses []int
	for _, rq := range requests {
		wantStatuses = append(wantStatuses, s.doAuth(t, rq.method, rq.path, rq.body, rq.token).Code)
	}

	// A 500 also logs the error separately; it still gets exactly one request entry.
	s.repo.Err = errTest
	wantStatuses = append(wantStatuses, s.doAuth(t, http.MethodGet, "/users", "", anyToken).Code)
	requests = append(requests, struct{ method, path, body, token string }{http.MethodGet, "/users", "", anyToken})

	entries := requestLogs(t, s.logs)
	if len(entries) != len(requests) {
		t.Fatalf("got %d request log entries for %d requests:\n%s", len(entries), len(requests), s.logs)
	}
	seen := map[int]bool{}
	for i, e := range entries {
		rq := requests[i]
		if e["method"] != rq.method || e["path"] != rq.path || e["status"] != float64(wantStatuses[i]) {
			t.Errorf("entry %d = %v, want %s %s status %d", i, e, rq.method, rq.path, wantStatuses[i])
		}
		if d, ok := e["duration_ms"].(float64); !ok || d < 0 {
			t.Errorf("entry %d duration_ms = %v, want a non-negative number", i, e["duration_ms"])
		}
		seen[wantStatuses[i]] = true
	}
	for _, status := range []int{200, 201, 204, 400, 401, 404, 405, 500} {
		if !seen[status] {
			t.Errorf("no request covered status %d", status)
		}
	}
}

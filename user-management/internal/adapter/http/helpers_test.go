package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jingjoeh/test-assignment/user-management/internal/fake"
	"github.com/jingjoeh/test-assignment/user-management/internal/port"
	"github.com/jingjoeh/test-assignment/user-management/internal/service"
)

type testServer struct {
	handler http.Handler
	repo    *fake.UserRepository
	logs    *bytes.Buffer
}

// newTestServer wires the real services and router to in-memory fakes. A nil
// tokens uses fake.Tokens.
func newTestServer(t *testing.T, tokens port.TokenIssuer) testServer {
	t.Helper()
	return newTestServerWith(t, &fake.Hasher{}, tokens)
}

// newTestServerWith is newTestServer with a chosen hasher, for tests that need
// the real bcrypt behavior.
func newTestServerWith(t *testing.T, hasher port.PasswordHasher, tokens port.TokenIssuer) testServer {
	t.Helper()
	if tokens == nil {
		tokens = &fake.Tokens{}
	}
	repo := fake.NewUserRepository()
	auth, err := service.NewAuthService(repo, hasher, tokens)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return testServer{handler: NewRouter(auth, logger), repo: repo, logs: logs}
}

func (s testServer) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

// decodeBody decodes a JSON response and checks its Content-Type.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("response is not a JSON object: %v; body = %s", err, rec.Body)
	}
	return m
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, wantStatus, rec.Body)
	}
	body := decodeBody(t, rec)
	msg, _ := body["error"].(string)
	if len(body) != 1 || !strings.Contains(msg, wantMsg) {
		t.Errorf("body = %v, want only {\"error\": ...containing %q}", body, wantMsg)
	}
}

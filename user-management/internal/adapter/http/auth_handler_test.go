package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/auth"
)

const aliceJSON = `{"name":"Alice","email":"alice@example.com","password":"s3cret-pass"}`

func TestRegisterCreated(t *testing.T) {
	s := newTestServer(t, nil)

	rec := s.do(t, http.MethodPost, "/auth/register", aliceJSON)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body)
	}
	body := decodeBody(t, rec)
	if body["name"] != "Alice" || body["email"] != "alice@example.com" || body["id"] == "" || body["createdAt"] == "" {
		t.Errorf("body = %v, want the created user", body)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "password") || strings.Contains(rec.Body.String(), "hashed:") {
		t.Errorf("response leaks the password or hash: %s", rec.Body)
	}
}

func TestRegisterErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{"validation", `{"name":"","email":"alice@example.com","password":"s3cret-pass"}`, 400, "name is required"},
		{"invalid email", `{"name":"Alice","email":"nope","password":"s3cret-pass"}`, 400, "valid email"},
		{"unknown field", `{"name":"Alice","email":"alice@example.com","password":"s3cret-pass","role":"admin"}`, 400, `unknown field "role"`},
		{"malformed JSON", `{"name":"Alice",`, 400, "valid JSON"},
		{"syntax error", `{"name" "Alice"}`, 400, "valid JSON"},
		{"trailing JSON", aliceJSON + `{"name":"Bob"}`, 400, "single JSON object"},
		{"trailing garbage", aliceJSON + ` x`, 400, "single JSON object"},
		{"empty body", ``, 400, "request body is required"},
		{"wrong type", `{"name":123,"email":"alice@example.com","password":"s3cret-pass"}`, 400, `field "name" has the wrong type`},
		{"array instead of object", `[]`, 400, "JSON object"},
		{"too large", `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`, 413, "at most 1048576 bytes"},
		{"too large after the first value", aliceJSON + strings.Repeat(" ", maxBodyBytes), 413, "at most 1048576 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, nil)
			assertError(t, s.do(t, http.MethodPost, "/auth/register", tt.body), tt.wantStatus, tt.wantMsg)
			if s.repo.Calls["Create"] != 0 {
				t.Errorf("repository Create was called for an invalid request")
			}
		})
	}
}

func TestRegisterTrailingWhitespaceIsAllowed(t *testing.T) {
	s := newTestServer(t, nil)
	if rec := s.do(t, http.MethodPost, "/auth/register", aliceJSON+"\n \t"); rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body = %s", rec.Code, rec.Body)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	s := newTestServer(t, nil)
	s.do(t, http.MethodPost, "/auth/register", aliceJSON)

	rec := s.do(t, http.MethodPost, "/auth/register", `{"name":"Other","email":"ALICE@example.com","password":"s3cret-pass"}`)
	assertError(t, rec, http.StatusConflict, "email already exists")
}

func TestRegisterInternalErrorIsGeneric(t *testing.T) {
	s := newTestServer(t, nil)
	s.repo.Err = errors.New("mongo: connection refused to 10.0.0.5")

	rec := s.do(t, http.MethodPost, "/auth/register", aliceJSON)

	assertError(t, rec, http.StatusInternalServerError, "internal server error")
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("response leaks the internal error: %s", rec.Body)
	}
	if !strings.Contains(s.logs.String(), "10.0.0.5") {
		t.Errorf("internal error was not logged; logs = %s", s.logs)
	}
}

func TestLoginOK(t *testing.T) {
	s := newTestServer(t, nil)
	s.do(t, http.MethodPost, "/auth/register", aliceJSON)

	rec := s.do(t, http.MethodPost, "/auth/login", `{"email":"alice@example.com","password":"s3cret-pass"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	body := decodeBody(t, rec)
	if token, _ := body["token"].(string); len(body) != 1 || !strings.HasPrefix(token, "token-for:") {
		t.Errorf("body = %v, want only {\"token\": ...}", body)
	}
}

// B3 Verify: login with the real JWT issuer returns an HS256 token.
func TestLoginReturnsHS256Token(t *testing.T) {
	s := newTestServer(t, auth.NewJWT([]byte(strings.Repeat("k", 32)), "user-management", time.Hour))
	s.do(t, http.MethodPost, "/auth/register", aliceJSON)

	rec := s.do(t, http.MethodPost, "/auth/login", `{"email":"alice@example.com","password":"s3cret-pass"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	token, _ := decodeBody(t, rec)["token"].(string)

	header, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
	if err != nil {
		t.Fatalf("decode token header: %v", err)
	}
	var h map[string]any
	if err := json.Unmarshal(header, &h); err != nil {
		t.Fatalf("unmarshal token header: %v", err)
	}
	if h["alg"] != "HS256" {
		t.Errorf("token alg = %v, want HS256", h["alg"])
	}
}

// B3 Verify: a wrong password and an unknown email get the same 401 response.
func TestLoginInvalidCredentialsAreIndistinguishable(t *testing.T) {
	s := newTestServer(t, nil)
	s.do(t, http.MethodPost, "/auth/register", aliceJSON)

	wrongPassword := s.do(t, http.MethodPost, "/auth/login", `{"email":"alice@example.com","password":"wrong-pass"}`)
	unknownEmail := s.do(t, http.MethodPost, "/auth/login", `{"email":"bob@example.com","password":"s3cret-pass"}`)

	assertError(t, wrongPassword, http.StatusUnauthorized, "invalid email or password")
	if wrongPassword.Code != unknownEmail.Code || wrongPassword.Body.String() != unknownEmail.Body.String() {
		t.Errorf("responses differ:\n wrong password: %d %s\n unknown email:  %d %s",
			wrongPassword.Code, wrongPassword.Body, unknownEmail.Code, unknownEmail.Body)
	}
}

// Regression, wired with the real bcrypt hasher: a 72-byte password plus a
// suffix must not log in, and must look exactly like a wrong password.
func TestLoginRejectsPasswordBeyondBcryptLimit(t *testing.T) {
	s := newTestServerWith(t, auth.NewBcryptHasher(bcrypt.MinCost), nil)
	password := strings.Repeat("p", 72)
	register := `{"name":"Alice","email":"alice@example.com","password":"` + password + `"}`
	if rec := s.do(t, http.MethodPost, "/auth/register", register); rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201; body = %s", rec.Code, rec.Body)
	}
	login := func(pw string) string { return `{"email":"alice@example.com","password":"` + pw + `"}` }

	if rec := s.do(t, http.MethodPost, "/auth/login", login(password)); rec.Code != http.StatusOK {
		t.Fatalf("login with the exact password: status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	overlong := s.do(t, http.MethodPost, "/auth/login", login(password+"anything"))
	wrong := s.do(t, http.MethodPost, "/auth/login", login(strings.Repeat("q", 72)))
	assertError(t, overlong, http.StatusUnauthorized, "invalid email or password")
	if overlong.Body.String() != wrong.Body.String() {
		t.Errorf("overlong password response %q differs from wrong password response %q", overlong.Body, wrong.Body)
	}
}

func TestLoginBadRequests(t *testing.T) {
	s := newTestServer(t, nil)
	assertError(t, s.do(t, http.MethodPost, "/auth/login", `{"email":"alice@example.com"}`), 400, "password is required")
	assertError(t, s.do(t, http.MethodPost, "/auth/login", `{"email":`), 400, "valid JSON")
	assertError(t, s.do(t, http.MethodPost, "/auth/login", `{"email":"a@b.c","password":"x","name":"n"}`), 400, `unknown field "name"`)
}

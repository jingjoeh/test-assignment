package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jingjoeh/test-assignment/user-management/internal/fake"
)

// anyToken is accepted by fake.Tokens. Under D3 any authenticated user may
// act on any user, so the token's subject doesn't matter here.
const anyToken = "token-for:" + "000000000000000000000001"

func TestListUsers(t *testing.T) {
	s := newTestServer(t, nil)

	rec := s.doAuth(t, http.MethodGet, "/users", "", anyToken)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list: status = %d, body = %s; want 200 and []", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	alice := s.registerUser(t, "Alice", "alice@example.com")
	bob := s.registerUser(t, "Bob", "bob@example.com")
	rec = s.doAuth(t, http.MethodGet, "/users", "", anyToken)
	var users []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode list: %v; body = %s", err, rec.Body)
	}
	if rec.Code != http.StatusOK || len(users) != 2 || users[0]["id"] != alice || users[1]["id"] != bob {
		t.Errorf("list: status = %d, users = %v; want alice then bob", rec.Code, users)
	}
	assertNoPassword(t, rec.Body.String())
}

func TestGetUser(t *testing.T) {
	s := newTestServer(t, nil)
	alice := s.registerUser(t, "Alice", "alice@example.com")

	rec := s.doAuth(t, http.MethodGet, "/users/"+alice, "", anyToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if body := decodeBody(t, rec); body["id"] != alice || body["name"] != "Alice" || len(body) != 4 {
		t.Errorf("body = %v, want alice's id, name, email, createdAt", body)
	}
	assertNoPassword(t, rec.Body.String())

	assertError(t, s.doAuth(t, http.MethodGet, "/users/not-an-id", "", anyToken), 400, "invalid user id")
	assertError(t, s.doAuth(t, http.MethodGet, "/users/"+fake.NewID(42), "", anyToken), 404, "user not found")
}

func TestUpdateUser(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantName  string
		wantEmail string
	}{
		{"name only", `{"name":"Alice B"}`, "Alice B", "alice@example.com"},
		{"email only", `{"email":"New@Example.com"}`, "Alice", "new@example.com"},
		{"both", `{"name":"Al","email":"al@example.com"}`, "Al", "al@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, nil)
			alice := s.registerUser(t, "Alice", "alice@example.com")

			rec := s.doAuth(t, http.MethodPatch, "/users/"+alice, tt.body, anyToken)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
			}
			body := decodeBody(t, rec)
			if body["name"] != tt.wantName || body["email"] != tt.wantEmail || body["id"] != alice {
				t.Errorf("body = %v, want name %q, email %q", body, tt.wantName, tt.wantEmail)
			}
			assertNoPassword(t, rec.Body.String())
		})
	}
}

func TestUpdateUserErrors(t *testing.T) {
	tests := []struct {
		name       string
		path       func(alice string) string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{"empty object", userPath, `{}`, 400, "at least one of name or email"},
		{"null fields", userPath, `{"name":null,"email":null}`, 400, "at least one of name or email"},
		{"password field", userPath, `{"password":"new-pass-123"}`, 400, `unknown field "password"`},
		{"typo field", userPath, `{"emial":"a@b.com"}`, 400, `unknown field "emial"`},
		{"invalid email", userPath, `{"email":"nope"}`, 400, "valid email"},
		{"blank name", userPath, `{"name":"  "}`, 400, "name is required"},
		{"trailing JSON", userPath, `{"name":"A"}{"name":"B"}`, 400, "single JSON object"},
		{"malformed JSON", userPath, `{"name":`, 400, "valid JSON"},
		{"empty body", userPath, ``, 400, "request body is required"},
		{"too large", userPath, `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`, 413, "at most"},
		{"duplicate email", userPath, `{"email":"bob@example.com"}`, 409, "email already exists"},
		{"malformed id", func(string) string { return "/users/not-an-id" }, `{"name":"X"}`, 400, "invalid user id"},
		{"missing id", func(string) string { return "/users/" + fake.NewID(42) }, `{"name":"X"}`, 404, "user not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, nil)
			alice := s.registerUser(t, "Alice", "alice@example.com")
			s.registerUser(t, "Bob", "bob@example.com")

			assertError(t, s.doAuth(t, http.MethodPatch, tt.path(alice), tt.body, anyToken), tt.wantStatus, tt.wantMsg)

			// A rejected update leaves alice unchanged.
			body := decodeBody(t, s.doAuth(t, http.MethodGet, "/users/"+alice, "", anyToken))
			if body["name"] != "Alice" || body["email"] != "alice@example.com" {
				t.Errorf("alice changed after a rejected update: %v", body)
			}
		})
	}
}

func userPath(id string) string { return "/users/" + id }

func TestDeleteUser(t *testing.T) {
	s := newTestServer(t, nil)
	alice := s.registerUser(t, "Alice", "alice@example.com")

	rec := s.doAuth(t, http.MethodDelete, "/users/"+alice, "", anyToken)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("delete: status = %d, body = %q; want 204 with an empty body", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "" {
		t.Errorf("204 response has Content-Type %q, want none", ct)
	}

	assertError(t, s.doAuth(t, http.MethodGet, "/users/"+alice, "", anyToken), 404, "user not found")
	assertError(t, s.doAuth(t, http.MethodDelete, "/users/"+alice, "", anyToken), 404, "user not found")
	assertError(t, s.doAuth(t, http.MethodDelete, "/users/not-an-id", "", anyToken), 400, "invalid user id")
	assertError(t, s.doAuth(t, http.MethodDelete, "/users/"+fake.NewID(42), "", anyToken), 404, "user not found")
}

func TestUsersRoutesRequireToken(t *testing.T) {
	s := newTestServer(t, nil)
	alice := s.registerUser(t, "Alice", "alice@example.com")

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/users", ""},
		{http.MethodGet, "/users/" + alice, ""},
		{http.MethodPatch, "/users/" + alice, `{"name":"Mallory"}`},
		{http.MethodDelete, "/users/" + alice, ""},
	}
	for _, rt := range routes {
		for _, token := range []string{"", "not-a-valid-token"} {
			rec := s.doAuth(t, rt.method, rt.path, rt.body, token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with token %q: status = %d, want 401", rt.method, rt.path, token, rec.Code)
				continue
			}
			assertError(t, rec, http.StatusUnauthorized, "token")
		}
	}

	// Nothing was changed or deleted by the rejected requests.
	body := decodeBody(t, s.doAuth(t, http.MethodGet, "/users/"+alice, "", anyToken))
	if body["name"] != "Alice" {
		t.Errorf("alice = %v, want unchanged", body)
	}
}

func TestUsersRoutesInternalErrorIsGeneric(t *testing.T) {
	s := newTestServer(t, nil)
	alice := s.registerUser(t, "Alice", "alice@example.com")
	s.repo.Err = errors.New("mongo: connection refused to 10.0.0.5")

	for _, rt := range []struct{ method, path, body string }{
		{http.MethodGet, "/users", ""},
		{http.MethodGet, "/users/" + alice, ""},
		{http.MethodPatch, "/users/" + alice, `{"name":"X"}`},
		{http.MethodDelete, "/users/" + alice, ""},
	} {
		rec := s.doAuth(t, rt.method, rt.path, rt.body, anyToken)
		assertError(t, rec, http.StatusInternalServerError, "internal server error")
		if strings.Contains(rec.Body.String(), "10.0.0.5") {
			t.Errorf("%s %s leaks the internal error: %s", rt.method, rt.path, rec.Body)
		}
	}
}

func assertNoPassword(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(strings.ToLower(body), "password") || strings.Contains(body, "hashed:") {
		t.Errorf("response leaks a password or hash: %s", body)
	}
}

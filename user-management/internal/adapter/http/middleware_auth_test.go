package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/auth"
)

func TestRequireAuth(t *testing.T) {
	secret := []byte(strings.Repeat("k", 32))
	tokens := auth.NewJWT(secret, "user-management", time.Hour)
	valid, err := tokens.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	signed := func(claims jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}
	now := time.Now()
	expired := signed(jwt.MapClaims{"sub": "user-1", "iss": "user-management", "iat": now.Add(-2 * time.Hour).Unix(), "exp": now.Add(-time.Hour).Unix()})
	wrongIssuer := signed(jwt.MapClaims{"sub": "user-1", "iss": "other", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})

	var gotUserID string
	protected := RequireAuth(tokens)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantMsg    string
	}{
		{"missing header", "", 401, "missing bearer token"},
		{"blank header", "   ", 401, "missing bearer token"},
		{"not bearer", "Basic dXNlcjpwYXNz", 401, "missing bearer token"},
		{"scheme only", "Bearer", 401, "missing bearer token"},
		{"empty token", "Bearer ", 401, "missing bearer token"},
		{"blank token", "Bearer    ", 401, "missing bearer token"},
		{"token without scheme", valid, 401, "missing bearer token"},
		{"extra field", "Bearer " + valid + " extra", 401, "missing bearer token"},
		{"two tokens", "Bearer " + valid + " " + valid, 401, "missing bearer token"},
		{"invalid token", "Bearer not.a.token", 401, "invalid or expired token"},
		{"expired token", "Bearer " + expired, 401, "invalid or expired token"},
		{"wrong issuer", "Bearer " + wrongIssuer, 401, "invalid or expired token"},
		{"valid token", "Bearer " + valid, 204, ""},
		{"lowercase scheme", "bearer " + valid, 204, ""},
		{"uppercase scheme", "BEARER " + valid, 204, ""},
		{"mixed-case scheme", "bEaReR " + valid, 204, ""},
		{"surrounding whitespace", "  Bearer " + valid + "  ", 204, ""},
		{"multiple spaces", "Bearer    " + valid, 204, ""},
		{"tab separator", "Bearer\t" + valid, 204, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID = ""
			req := httptest.NewRequest(http.MethodGet, "/users", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)

			if tt.wantStatus == http.StatusNoContent {
				if rec.Code != http.StatusNoContent || gotUserID != "user-1" {
					t.Errorf("status = %d, user ID in context = %q; want 204 and %q", rec.Code, gotUserID, "user-1")
				}
				return
			}
			assertError(t, rec, tt.wantStatus, tt.wantMsg)
			if rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want %q", rec.Header().Get("WWW-Authenticate"), "Bearer")
			}
			if gotUserID != "" {
				t.Errorf("protected handler ran for a rejected request")
			}
		})
	}
}

func TestUserIDFromContextMissing(t *testing.T) {
	if id, ok := UserIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); ok || id != "" {
		t.Errorf("UserIDFromContext = %q, %v; want \"\", false", id, ok)
	}
}

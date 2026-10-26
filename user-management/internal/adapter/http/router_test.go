package http

import (
	"net/http"
	"strings"
	"testing"
)

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	s := newTestServer(t, nil)
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/nope"},
		{http.MethodPost, "/auth/unknown"},
		{http.MethodGet, "/users/a/b"},
	} {
		rec := s.doAuth(t, rt.method, rt.path, "", anyToken)
		assertError(t, rec, http.StatusNotFound, "not found")
	}
}

func TestWrongMethodReturnsJSON405WithAllow(t *testing.T) {
	s := newTestServer(t, nil)
	tests := []struct {
		method, path string
		wantAllow    []string
	}{
		{http.MethodGet, "/auth/login", []string{"POST"}},
		{http.MethodGet, "/auth/register", []string{"POST"}},
		{http.MethodPost, "/users", []string{"GET", "HEAD"}},
		{http.MethodPut, "/users/000000000000000000000001", []string{"DELETE", "GET", "HEAD", "PATCH"}},
		{http.MethodPost, "/users/000000000000000000000001", []string{"DELETE", "GET", "HEAD", "PATCH"}},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			// No token: the 405 must not depend on authentication.
			rec := s.do(t, tt.method, tt.path, "")
			assertError(t, rec, http.StatusMethodNotAllowed, "method not allowed")

			got := map[string]bool{}
			for _, m := range splitAllow(rec.Header().Get("Allow")) {
				got[m] = true
			}
			if len(got) != len(tt.wantAllow) {
				t.Errorf("Allow = %q, want %v", rec.Header().Get("Allow"), tt.wantAllow)
			}
			for _, m := range tt.wantAllow {
				if !got[m] {
					t.Errorf("Allow = %q, missing %s", rec.Header().Get("Allow"), m)
				}
			}
		})
	}
}

func splitAllow(v string) []string {
	var out []string
	for _, f := range strings.Split(v, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ServeMux redirects non-canonical paths; the JSON wrapper must pass those
// responses through unchanged rather than turning them into errors.
func TestNonCanonicalPathRedirectPassesThrough(t *testing.T) {
	s := newTestServer(t, nil)
	rec := s.do(t, http.MethodGet, "/auth/../users", "")
	// The exact redirect code depends on the Go version (301 or 307).
	if rec.Code < 300 || rec.Code > 399 || rec.Header().Get("Location") != "/users" {
		t.Errorf("status = %d, Location = %q; want a redirect to /users", rec.Code, rec.Header().Get("Location"))
	}
	if ct := rec.Header().Get("Content-Type"); ct == "application/json" {
		t.Errorf("redirect was rewritten as a JSON error: %s", rec.Body)
	}
}

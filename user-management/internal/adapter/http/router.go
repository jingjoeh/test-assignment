package http

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/jingjoeh/test-assignment/user-management/internal/port"
	"github.com/jingjoeh/test-assignment/user-management/internal/service"
)

// NewRouter returns the API's HTTP handler. Every /users route requires a
// valid bearer token. Logging wraps everything, so every request is logged
// once, including 401s from RequireAuth and the JSON 404/405 responses.
func NewRouter(auth *service.AuthService, users *service.UserService, tokens port.TokenValidator, logger *slog.Logger) http.Handler {
	ah := &authHandler{auth: auth, logger: logger}
	uh := &userHandler{users: users, logger: logger}
	protected := RequireAuth(tokens)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", ah.register)
	mux.HandleFunc("POST /auth/login", ah.login)
	mux.Handle("GET /users", protected(http.HandlerFunc(uh.list)))
	mux.Handle("GET /users/{id}", protected(http.HandlerFunc(uh.get)))
	mux.Handle("PATCH /users/{id}", protected(http.HandlerFunc(uh.update)))
	mux.Handle("DELETE /users/{id}", protected(http.HandlerFunc(uh.delete)))
	return Logging(logger)(jsonMuxErrors(mux))
}

// jsonMuxErrors replaces ServeMux's plain-text 404 and 405 responses with the
// API's JSON error format. For a 405 it keeps the Allow header the mux
// computed. Requests that match a route go straight to the mux.
func jsonMuxErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		// No route matched. Run the mux's own fallback (404, 405 or a redirect
		// to the canonical path) into a buffer, then translate it.
		rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		h.ServeHTTP(rec, r)

		switch rec.status {
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "not found")
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", rec.header.Get("Allow"))
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		default:
			// Anything else (such as a redirect) is passed through unchanged.
			for k, v := range rec.header {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.status)
			_, _ = w.Write(rec.body.Bytes())
		}
	})
}

// bufferedResponse captures a response so it can be inspected before sending.
type bufferedResponse struct {
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if !b.wroteHeader {
		b.status, b.wroteHeader = status, true
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(p)
}

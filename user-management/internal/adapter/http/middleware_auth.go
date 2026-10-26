package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

type userIDKey struct{}

// UserIDFromContext returns the ID of the authenticated user, set by
// RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok
}

// RequireAuth rejects requests without a valid "Authorization: Bearer <token>"
// header with 401, and puts the token's user ID in the request context.
func RequireAuth(tokens port.TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w, "missing bearer token")
				return
			}
			userID, err := tokens.Validate(token)
			if err != nil {
				unauthorized(w, "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
		})
	}
}

// bearerToken extracts the token from an Authorization header value. The
// value must be exactly two whitespace-separated fields; the scheme is
// matched case-insensitively (RFC 7235), and surrounding or repeated
// whitespace is ignored.
func bearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer`)
	writeError(w, http.StatusUnauthorized, msg)
}

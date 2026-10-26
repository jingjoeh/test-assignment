package http

import (
	"log/slog"
	"net/http"

	"github.com/jingjoeh/test-assignment/user-management/internal/service"
)

// NewRouter returns the API's HTTP handler. The /users routes are added in B4
// and the logging middleware in B5.
func NewRouter(auth *service.AuthService, logger *slog.Logger) http.Handler {
	ah := &authHandler{auth: auth, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", ah.register)
	mux.HandleFunc("POST /auth/login", ah.login)
	return mux
}

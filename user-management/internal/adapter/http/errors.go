package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

// writeServiceError maps a service error to a status code. This is the only
// place domain errors become HTTP statuses. Unknown errors are logged and
// answered with a generic 500, so internal details never reach the client.
func writeServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrInvalidID):
		writeError(w, http.StatusBadRequest, domain.ErrInvalidID.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, domain.ErrNotFound.Error())
	case errors.Is(err, domain.ErrEmailExists):
		writeError(w, http.StatusConflict, domain.ErrEmailExists.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, domain.ErrInvalidCredentials.Error())
	default:
		logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

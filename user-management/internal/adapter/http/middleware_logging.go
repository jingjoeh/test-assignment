package http

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging logs exactly one entry per request with the method, path, response
// status and duration (req. 5). The path excludes the query string, so query
// values never reach the logs.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return logging(logger, time.Now)
}

// logging takes the clock as a parameter so tests can control the duration.
func logging(logger *slog.Logger, now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", float64(now().Sub(start).Microseconds())/1000,
			)
		})
	}
}

// statusWriter records the status code a handler sends. A handler that never
// calls WriteHeader sends 200, which is the initial value.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = status, true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

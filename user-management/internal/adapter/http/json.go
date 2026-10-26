package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxBodyBytes caps every request body (D17).
const maxBodyBytes = 1 << 20

type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON writes v as the JSON response body. Content-Type is set before
// the status, so every JSON response carries it.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// decodeJSON reads exactly one JSON value from the body into dst. It rejects
// bodies over 1 MiB, unknown fields (D11), empty or malformed JSON, and
// anything after the first value (D17). On failure it has already written the
// error response and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		writeDecodeError(w, err)
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeDecodeError(w, err)
			return false
		}
		writeError(w, http.StatusBadRequest, "request body must contain a single JSON object")
		return false
	}
	return true
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var (
		tooLarge  *http.MaxBytesError
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body must be at most %d bytes", maxBodyBytes))
	case errors.Is(err, io.EOF):
		writeError(w, http.StatusBadRequest, "request body is required")
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
	case errors.As(err, &typeErr) && typeErr.Field != "":
		writeError(w, http.StatusBadRequest, fmt.Sprintf("field %q has the wrong type", typeErr.Field))
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json has no typed error for unknown fields.
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "json: "))
	default:
		writeError(w, http.StatusBadRequest, "request body must be a JSON object")
	}
}

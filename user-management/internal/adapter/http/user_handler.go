package http

import (
	"log/slog"
	"net/http"

	"github.com/jingjoeh/test-assignment/user-management/internal/service"
)

type userHandler struct {
	users  *service.UserService
	logger *slog.Logger
}

// updateUserRequest uses pointers so a field that wasn't sent is left
// unchanged (D7).
type updateUserRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

// list handles GET /users.
func (h *userHandler) list(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, newUserResponse(u))
	}
	writeJSON(w, http.StatusOK, resp)
}

// get handles GET /users/{id}.
func (h *userHandler) get(w http.ResponseWriter, r *http.Request) {
	user, err := h.users.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(user))
}

// update handles PATCH /users/{id}.
func (h *userHandler) update(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := h.users.Update(r.Context(), r.PathValue("id"), service.UpdateInput{
		Name:  req.Name,
		Email: req.Email,
	})
	if err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(user))
}

// delete handles DELETE /users/{id}. Success is 204 with no body.
func (h *userHandler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.users.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package http

import (
	"time"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

// userResponse is the JSON shape of a user. It has no password field, so a
// hash can never be serialized by accident.
type userResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

func newUserResponse(u domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt.UTC(),
	}
}

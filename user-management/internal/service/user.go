package service

import (
	"context"
	"strings"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// UpdateInput is a partial update (D7). A nil field is left unchanged.
type UpdateInput struct {
	Name  *string
	Email *string
}

// UserService reads, updates and deletes users. Users are created only by
// AuthService.Register (D14), and this service never calls AuthService.
type UserService struct {
	users port.UserRepository
}

func NewUserService(users port.UserRepository) *UserService {
	return &UserService{users: users}
}

func (s *UserService) Get(ctx context.Context, id string) (domain.User, error) {
	return s.users.GetByID(ctx, id)
}

// List returns every user, oldest first (D8).
func (s *UserService) List(ctx context.Context) ([]domain.User, error) {
	return s.users.List(ctx)
}

// Update changes the user's name and/or email. At least one is required, and
// each one given is normalized and validated with the same rules as Register.
func (s *UserService) Update(ctx context.Context, id string, in UpdateInput) (domain.User, error) {
	if in.Name == nil && in.Email == nil {
		return domain.User{}, validationError("at least one of name or email is required")
	}

	var upd port.UserUpdate
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if err := validateName(name); err != nil {
			return domain.User{}, err
		}
		upd.Name = &name
	}
	if in.Email != nil {
		email := normalizeEmail(*in.Email)
		if err := validateEmail(email); err != nil {
			return domain.User{}, err
		}
		upd.Email = &email
	}
	return s.users.Update(ctx, id, upd)
}

// Delete removes the user permanently (D9).
func (s *UserService) Delete(ctx context.Context, id string) error {
	return s.users.Delete(ctx, id)
}

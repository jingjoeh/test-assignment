package port

import (
	"context"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

// UserUpdate holds the fields of a partial update (D7). A nil field is left
// unchanged.
type UserUpdate struct {
	Name  *string
	Email *string
}

// UserRepository persists users. Implementations return the domain sentinel
// errors: ErrInvalidID for a malformed ID, ErrNotFound for a missing user and
// ErrEmailExists for a duplicate email. Any other error is an infrastructure
// failure.
type UserRepository interface {
	// Create stores u and returns it with its generated ID.
	Create(ctx context.Context, u domain.User) (domain.User, error)
	GetByID(ctx context.Context, id string) (domain.User, error)
	// GetByEmail matches the email exactly; callers normalize it first.
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	// List returns every user, oldest first. It returns an empty, non-nil
	// slice when there are none.
	List(ctx context.Context) ([]domain.User, error)
	// Update applies the non-nil fields of upd and returns the updated user.
	Update(ctx context.Context, id string, upd UserUpdate) (domain.User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// maxPasswordBytes is bcrypt's input limit: it only uses the first 72 bytes.
const maxPasswordBytes = 72

// BcryptHasher implements port.PasswordHasher with bcrypt.
type BcryptHasher struct {
	cost int
}

var _ port.PasswordHasher = (*BcryptHasher)(nil)

// NewBcryptHasher returns a hasher using cost. Production uses
// bcrypt.DefaultCost; tests use bcrypt.MinCost to stay fast.
func NewBcryptHasher(cost int) *BcryptHasher {
	return &BcryptHasher{cost: cost}
}

func (h *BcryptHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: hash password: %w", err)
	}
	return string(hash), nil
}

func (h *BcryptHasher) Matches(hash, password string) (bool, error) {
	// bcrypt compares only the first 72 bytes, so without this check a stored
	// 72-byte password would also match itself plus any suffix. Registration
	// never stores a longer password (D21), so a longer one can't be correct.
	if len(password) > maxPasswordBytes {
		return false, nil
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return false, nil
	default:
		return false, fmt.Errorf("bcrypt: compare password: %w", err)
	}
}

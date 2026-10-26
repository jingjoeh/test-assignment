package domain

import "errors"

// Sentinel errors returned by services and repositories. Adapters map them to
// transport status codes in one place; callers compare with errors.Is.
var (
	ErrNotFound           = errors.New("user not found")
	ErrInvalidID          = errors.New("invalid user id")
	ErrEmailExists        = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrValidation is wrapped with the specific problem, for example
	// fmt.Errorf("%w: name is required", ErrValidation).
	ErrValidation = errors.New("validation failed")
)

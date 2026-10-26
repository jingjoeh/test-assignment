package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// RegisterInput is the data needed to create a user.
type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

// AuthService registers users and logs them in. It is the only place that
// creates users (D14).
type AuthService struct {
	users  port.UserRepository
	hasher port.PasswordHasher
	tokens port.TokenIssuer
	now    func() time.Time

	// dummyHash is compared against when the email is unknown, so a login for
	// an unknown email costs the same as one with a wrong password and the
	// response time doesn't reveal which emails are registered.
	dummyHash string
}

// NewAuthService builds the service. It hashes a dummy password once, so it
// can fail if the hasher fails.
func NewAuthService(users port.UserRepository, hasher port.PasswordHasher, tokens port.TokenIssuer) (*AuthService, error) {
	dummy, err := hasher.Hash("dummy-password-for-timing")
	if err != nil {
		return nil, fmt.Errorf("auth service: prepare dummy hash: %w", err)
	}
	return &AuthService{users: users, hasher: hasher, tokens: tokens, now: time.Now, dummyHash: dummy}, nil
}

// Register validates the input, hashes the password and stores the new user.
func (s *AuthService) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	if err := validateName(name); err != nil {
		return domain.User{}, err
	}
	if err := validateEmail(email); err != nil {
		return domain.User{}, err
	}
	if err := validatePassword(in.Password); err != nil {
		return domain.User{}, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, err
	}
	return s.users.Create(ctx, domain.User{
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    s.now().UTC(),
	})
}

// Login checks the credentials and returns an access token. An unknown email
// and a wrong password both return ErrInvalidCredentials.
func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	email = normalizeEmail(email)
	// Only presence is checked, so a login never reveals the registration rules.
	if email == "" {
		return "", validationError("email is required")
	}
	if password == "" {
		return "", validationError("password is required")
	}

	user, err := s.users.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		_, _ = s.hasher.Matches(s.dummyHash, password)
		return "", domain.ErrInvalidCredentials
	case err != nil:
		return "", err
	}

	ok, err := s.hasher.Matches(user.PasswordHash, password)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", domain.ErrInvalidCredentials
	}
	return s.tokens.Issue(user.ID)
}

package service

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

const (
	maxNameLength = 100
	// maxEmailBytes is the longest address SMTP can carry (RFC 5321). It is
	// checked on the normalized email, in bytes.
	maxEmailBytes     = 254
	minPasswordLength = 8
	// bcrypt only uses the first 72 bytes and rejects longer input, so a longer
	// password is a validation error rather than a 500.
	maxPasswordBytes = 72
)

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{domain.ErrValidation}, args...)...)
}

// validateName expects an already trimmed name.
func validateName(name string) error {
	switch n := utf8.RuneCountInString(name); {
	case n == 0:
		return validationError("name is required")
	case n > maxNameLength:
		return validationError("name must be at most %d characters", maxNameLength)
	}
	return nil
}

// validateEmail expects an already normalized email. Register uses it, and
// so will Update (B4), so both apply the same rules.
func validateEmail(email string) error {
	if email == "" {
		return validationError("email is required")
	}
	if len(email) > maxEmailBytes {
		return validationError("email must be at most %d bytes", maxEmailBytes)
	}
	// ParseAddress also accepts forms like "Alice <a@b.com>"; require a bare address.
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return validationError("email must be a valid email address")
	}
	return nil
}

func validatePassword(password string) error {
	switch {
	case password == "":
		return validationError("password is required")
	case utf8.RuneCountInString(password) < minPasswordLength:
		return validationError("password must be at least %d characters", minPasswordLength)
	case len(password) > maxPasswordBytes:
		return validationError("password must be at most %d bytes", maxPasswordBytes)
	}
	return nil
}

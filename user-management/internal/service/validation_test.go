package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

// emailOfLength builds a syntactically valid ASCII address of exactly n bytes.
func emailOfLength(n int) string {
	local := strings.Repeat("a", 64)
	return local + "@" + strings.Repeat("b", n-len(local)-len("@.com")) + ".com"
}

func TestValidateEmailLength(t *testing.T) {
	at254, at255 := emailOfLength(254), emailOfLength(255)
	if len(at254) != 254 || len(at255) != 255 {
		t.Fatalf("helper built %d and %d bytes, want 254 and 255", len(at254), len(at255))
	}

	if err := validateEmail(at254); err != nil {
		t.Errorf("validateEmail(254 bytes) = %v, want nil", err)
	}
	if err := validateEmail(at255); !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "at most 254 bytes") {
		t.Errorf("validateEmail(255 bytes) = %v, want ErrValidation about 254 bytes", err)
	}

	// The limit counts bytes, not characters: 64 two-byte characters plus a
	// 126-byte domain is 255 bytes but only 191 characters.
	multiByte := strings.Repeat("é", 64) + "@" + strings.Repeat("b", 122) + ".com"
	if len(multiByte) != 255 || utf8.RuneCountInString(multiByte) > 254 {
		t.Fatalf("multi-byte address is %d bytes / %d characters, want 255 / at most 254",
			len(multiByte), utf8.RuneCountInString(multiByte))
	}
	if err := validateEmail(multiByte); !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "at most 254 bytes") {
		t.Errorf("validateEmail(255 bytes, 191 characters) = %v, want ErrValidation about 254 bytes", err)
	}
}

// The limit applies to the normalized email, so surrounding spaces and upper
// case don't push a 254-byte address over it.
func TestRegisterEmailLengthAfterNormalization(t *testing.T) {
	f := newAuthFixture(t)
	raw := "   " + strings.ToUpper(emailOfLength(254)) + "   "

	u, err := f.svc.Register(context.Background(), RegisterInput{Name: "Alice", Email: raw, Password: "s3cret-pass"})
	if err != nil {
		t.Fatalf("Register(%d raw bytes, 254 normalized) = %v, want nil", len(raw), err)
	}
	if u.Email != emailOfLength(254) {
		t.Errorf("stored email = %q, want the normalized 254-byte address", u.Email)
	}

	_, err = f.svc.Register(context.Background(), RegisterInput{Name: "Bob", Email: " " + emailOfLength(255) + " ", Password: "s3cret-pass"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("Register(255 bytes normalized) = %v, want ErrValidation", err)
	}
}

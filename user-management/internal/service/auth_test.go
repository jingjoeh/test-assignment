package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/fake"
)

var fixedNow = time.Date(2026, 10, 26, 15, 0, 0, 0, time.FixedZone("ICT", 7*60*60))

type authFixture struct {
	svc    *AuthService
	repo   *fake.UserRepository
	hasher *fake.Hasher
	tokens *fake.Tokens
}

func newAuthFixture(t *testing.T) authFixture {
	t.Helper()
	f := authFixture{repo: fake.NewUserRepository(), hasher: &fake.Hasher{}, tokens: &fake.Tokens{}}
	svc, err := NewAuthService(f.repo, f.hasher, f.tokens)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	svc.now = func() time.Time { return fixedNow }
	f.svc = svc
	return f
}

func (f authFixture) register(t *testing.T, name, email, password string) domain.User {
	t.Helper()
	u, err := f.svc.Register(context.Background(), RegisterInput{Name: name, Email: email, Password: password})
	if err != nil {
		t.Fatalf("Register(%q): %v", email, err)
	}
	return u
}

func TestRegisterStoresNormalizedUser(t *testing.T) {
	f := newAuthFixture(t)

	u := f.register(t, "  Alice  ", "  Alice@Example.COM ", "s3cret-pass")

	stored, err := f.repo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored != u {
		t.Errorf("returned user %+v differs from stored %+v", u, stored)
	}
	if stored.Name != "Alice" {
		t.Errorf("Name = %q, want trimmed %q", stored.Name, "Alice")
	}
	if stored.Email != "alice@example.com" {
		t.Errorf("Email = %q, want normalized %q", stored.Email, "alice@example.com")
	}
	if stored.PasswordHash == "s3cret-pass" || stored.PasswordHash != "hashed:s3cret-pass" {
		t.Errorf("PasswordHash = %q, want the hasher's output, not the plain password", stored.PasswordHash)
	}
	if stored.CreatedAt.IsZero() || stored.CreatedAt.Location() != time.UTC || !stored.CreatedAt.Equal(fixedNow) {
		t.Errorf("CreatedAt = %v, want %v in UTC", stored.CreatedAt, fixedNow)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "Alice", "alice@example.com", "s3cret-pass")

	for _, email := range []string{"alice@example.com", "ALICE@example.com", "  alice@example.com  "} {
		_, err := f.svc.Register(context.Background(), RegisterInput{Name: "Other", Email: email, Password: "s3cret-pass"})
		if !errors.Is(err, domain.ErrEmailExists) {
			t.Errorf("Register(%q) error = %v, want ErrEmailExists", email, err)
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	valid := RegisterInput{Name: "Alice", Email: "alice@example.com", Password: "s3cret-pass"}
	tests := []struct {
		name    string
		mutate  func(*RegisterInput)
		wantMsg string
	}{
		{"empty name", func(in *RegisterInput) { in.Name = "" }, "name is required"},
		{"blank name", func(in *RegisterInput) { in.Name = "   " }, "name is required"},
		{"name too long", func(in *RegisterInput) { in.Name = strings.Repeat("a", 101) }, "name must be at most 100 characters"},
		{"empty email", func(in *RegisterInput) { in.Email = "" }, "email is required"},
		{"email without @", func(in *RegisterInput) { in.Email = "alice.example.com" }, "valid email"},
		{"email with display name", func(in *RegisterInput) { in.Email = "Alice <alice@example.com>" }, "valid email"},
		{"empty password", func(in *RegisterInput) { in.Password = "" }, "password is required"},
		{"short password", func(in *RegisterInput) { in.Password = "1234567" }, "at least 8 characters"},
		{"password over 72 bytes", func(in *RegisterInput) { in.Password = strings.Repeat("p", 73) }, "at most 72 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthFixture(t)
			in := valid
			tt.mutate(&in)

			_, err := f.svc.Register(context.Background(), in)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantMsg)
			}
			if f.repo.Calls["Create"] != 0 {
				t.Errorf("repository Create was called %d times, want 0", f.repo.Calls["Create"])
			}
		})
	}
}

func TestRegisterAcceptsBoundaryValues(t *testing.T) {
	tests := []RegisterInput{
		{Name: strings.Repeat("a", 100), Email: "a@example.com", Password: "12345678"},
		{Name: strings.Repeat("ก", 100), Email: "b@example.com", Password: strings.Repeat("p", 72)},
		{Name: "B", Email: "first.last+tag@sub.example.co.th", Password: "ยาวพอแปดตัวอักษร"},
	}
	for _, in := range tests {
		f := newAuthFixture(t)
		if _, err := f.svc.Register(context.Background(), in); err != nil {
			t.Errorf("Register(%+v) = %v, want nil", in, err)
		}
	}
}

func TestRegisterPropagatesFailures(t *testing.T) {
	in := RegisterInput{Name: "Alice", Email: "alice@example.com", Password: "s3cret-pass"}

	t.Run("hasher error", func(t *testing.T) {
		f := newAuthFixture(t)
		boom := errors.New("hasher down")
		f.hasher.HashErr = boom

		if _, err := f.svc.Register(context.Background(), in); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
		if f.repo.Calls["Create"] != 0 {
			t.Errorf("repository Create was called after the hasher failed")
		}
	})

	t.Run("repository error", func(t *testing.T) {
		f := newAuthFixture(t)
		boom := errors.New("db down")
		f.repo.Err = boom

		if _, err := f.svc.Register(context.Background(), in); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})
}

func TestNewAuthServiceFailsWhenHasherFails(t *testing.T) {
	_, err := NewAuthService(fake.NewUserRepository(), &fake.Hasher{HashErr: errors.New("boom")}, &fake.Tokens{})
	if err == nil {
		t.Fatal("NewAuthService = nil error, want an error")
	}
}

func TestLoginSuccess(t *testing.T) {
	f := newAuthFixture(t)
	u := f.register(t, "Alice", "alice@example.com", "s3cret-pass")

	for _, email := range []string{"alice@example.com", "  ALICE@Example.com "} {
		token, err := f.svc.Login(context.Background(), email, "s3cret-pass")
		if err != nil {
			t.Fatalf("Login(%q): %v", email, err)
		}
		if userID, _ := f.tokens.Validate(token); userID != u.ID {
			t.Errorf("Login(%q) token subject = %q, want user ID %q", email, userID, u.ID)
		}
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "Alice", "alice@example.com", "s3cret-pass")

	_, wrongPassword := f.svc.Login(context.Background(), "alice@example.com", "wrong-pass")
	_, unknownEmail := f.svc.Login(context.Background(), "bob@example.com", "s3cret-pass")

	for name, err := range map[string]error{"wrong password": wrongPassword, "unknown email": unknownEmail} {
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("%s: error = %v, want ErrInvalidCredentials", name, err)
		}
	}
	if wrongPassword.Error() != unknownEmail.Error() {
		t.Errorf("errors differ: %q vs %q; they must be identical", wrongPassword, unknownEmail)
	}

	// The unknown-email path still runs a password comparison (against the
	// dummy hash), so it costs the same as a wrong password.
	compared := f.hasher.ComparedHashes()
	if len(compared) == 0 || compared[len(compared)-1] != f.svc.dummyHash {
		t.Errorf("compared hashes = %q, want the last one to be the dummy hash", compared)
	}
}

func TestLoginValidation(t *testing.T) {
	f := newAuthFixture(t)
	for _, tt := range []struct{ email, password, wantMsg string }{
		{"", "s3cret-pass", "email is required"},
		{"   ", "s3cret-pass", "email is required"},
		{"alice@example.com", "", "password is required"},
	} {
		_, err := f.svc.Login(context.Background(), tt.email, tt.password)
		if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), tt.wantMsg) {
			t.Errorf("Login(%q, %q) error = %v, want ErrValidation %q", tt.email, tt.password, err, tt.wantMsg)
		}
	}

	// Login checks presence only: a short password or odd email is just wrong
	// credentials, so the login endpoint doesn't reveal the registration rules.
	if _, err := f.svc.Login(context.Background(), "not-an-email", "short"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("Login(malformed values) error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginPropagatesFailures(t *testing.T) {
	t.Run("repository error is not turned into invalid credentials", func(t *testing.T) {
		f := newAuthFixture(t)
		boom := errors.New("db down")
		f.repo.Err = boom

		_, err := f.svc.Login(context.Background(), "alice@example.com", "s3cret-pass")
		if !errors.Is(err, boom) || errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("error = %v, want %v and not ErrInvalidCredentials", err, boom)
		}
	})

	t.Run("hasher compare error", func(t *testing.T) {
		f := newAuthFixture(t)
		f.register(t, "Alice", "alice@example.com", "s3cret-pass")
		boom := errors.New("corrupt hash")
		f.hasher.MatchErr = boom

		if _, err := f.svc.Login(context.Background(), "alice@example.com", "s3cret-pass"); !errors.Is(err, boom) {
			t.Errorf("error = %v, want %v", err, boom)
		}
	})

	t.Run("token issue error", func(t *testing.T) {
		f := newAuthFixture(t)
		f.register(t, "Alice", "alice@example.com", "s3cret-pass")
		boom := errors.New("signer down")
		f.tokens.IssueErr = boom

		if _, err := f.svc.Login(context.Background(), "alice@example.com", "s3cret-pass"); !errors.Is(err, boom) {
			t.Errorf("error = %v, want %v", err, boom)
		}
	})
}

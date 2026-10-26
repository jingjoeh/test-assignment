package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/fake"
)

type userFixture struct {
	authFixture
	users *UserService
}

func newUserFixture(t *testing.T) userFixture {
	t.Helper()
	af := newAuthFixture(t)
	return userFixture{authFixture: af, users: NewUserService(af.repo)}
}

func ptr(s string) *string { return &s }

func TestUserGet(t *testing.T) {
	f := newUserFixture(t)
	alice := f.register(t, "Alice", "alice@example.com", "s3cret-pass")

	got, err := f.users.Get(context.Background(), alice.ID)
	if err != nil || got != alice {
		t.Errorf("Get = %+v, %v; want %+v", got, err, alice)
	}
	if _, err := f.users.Get(context.Background(), fake.NewID(42)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := f.users.Get(context.Background(), "not-an-id"); !errors.Is(err, domain.ErrInvalidID) {
		t.Errorf("Get(malformed) error = %v, want ErrInvalidID", err)
	}
}

func TestUserList(t *testing.T) {
	f := newUserFixture(t)

	empty, err := f.users.List(context.Background())
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("List(empty) = %#v, %v; want an empty, non-nil slice", empty, err)
	}

	alice := f.register(t, "Alice", "alice@example.com", "s3cret-pass")
	bob := f.register(t, "Bob", "bob@example.com", "s3cret-pass")
	all, err := f.users.List(context.Background())
	if err != nil || len(all) != 2 || all[0] != alice || all[1] != bob {
		t.Errorf("List = %+v, %v; want [alice, bob] in creation order", all, err)
	}
}

func TestUserUpdate(t *testing.T) {
	tests := []struct {
		name      string
		in        UpdateInput
		wantName  string
		wantEmail string
	}{
		{"name only", UpdateInput{Name: ptr("  Alice B  ")}, "Alice B", "alice@example.com"},
		{"email only", UpdateInput{Email: ptr("  NEW@Example.com ")}, "Alice", "new@example.com"},
		{"both", UpdateInput{Name: ptr("Al"), Email: ptr("al@example.com")}, "Al", "al@example.com"},
		{"own current email", UpdateInput{Email: ptr("Alice@Example.com")}, "Alice", "alice@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newUserFixture(t)
			alice := f.register(t, "Alice", "alice@example.com", "s3cret-pass")

			got, err := f.users.Update(context.Background(), alice.ID, tt.in)
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if got.Name != tt.wantName || got.Email != tt.wantEmail {
				t.Errorf("Update = name %q, email %q; want %q, %q", got.Name, got.Email, tt.wantName, tt.wantEmail)
			}
			// Update leaves the ID, CreatedAt and PasswordHash unchanged.
			if got.ID != alice.ID || !got.CreatedAt.Equal(alice.CreatedAt) || got.PasswordHash != alice.PasswordHash {
				t.Errorf("Update changed fields it must not: got %+v, was %+v", got, alice)
			}
			stored, _ := f.users.Get(context.Background(), alice.ID)
			if stored != got {
				t.Errorf("stored user %+v differs from returned %+v", stored, got)
			}
		})
	}
}

func TestUserUpdateErrors(t *testing.T) {
	tests := []struct {
		name    string
		id      func(alice domain.User) string
		in      UpdateInput
		wantErr error
		wantMsg string
	}{
		{"no fields", aliceID, UpdateInput{}, domain.ErrValidation, "at least one of name or email"},
		{"blank name", aliceID, UpdateInput{Name: ptr("   ")}, domain.ErrValidation, "name is required"},
		{"name too long", aliceID, UpdateInput{Name: ptr(strings.Repeat("a", 101))}, domain.ErrValidation, "at most 100"},
		{"empty email", aliceID, UpdateInput{Email: ptr("")}, domain.ErrValidation, "email is required"},
		{"invalid email", aliceID, UpdateInput{Email: ptr("nope")}, domain.ErrValidation, "valid email"},
		{"email over 254 bytes", aliceID, UpdateInput{Email: ptr(emailOfLength(255))}, domain.ErrValidation, "at most 254 bytes"},
		{"duplicate email", aliceID, UpdateInput{Email: ptr("BOB@example.com")}, domain.ErrEmailExists, ""},
		{"missing user", func(domain.User) string { return fake.NewID(42) }, UpdateInput{Name: ptr("X")}, domain.ErrNotFound, ""},
		{"malformed id", func(domain.User) string { return "not-an-id" }, UpdateInput{Name: ptr("X")}, domain.ErrInvalidID, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newUserFixture(t)
			alice := f.register(t, "Alice", "alice@example.com", "s3cret-pass")
			f.register(t, "Bob", "bob@example.com", "s3cret-pass")
			callsBefore := f.repo.Calls["Update"]

			_, err := f.users.Update(context.Background(), tt.id(alice), tt.in)
			if !errors.Is(err, tt.wantErr) || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error = %v, want %v containing %q", err, tt.wantErr, tt.wantMsg)
			}
			if errors.Is(tt.wantErr, domain.ErrValidation) && f.repo.Calls["Update"] != callsBefore {
				t.Errorf("repository Update was called for invalid input")
			}
			if stored, _ := f.users.Get(context.Background(), alice.ID); stored != alice {
				t.Errorf("a failed update changed the user: %+v, was %+v", stored, alice)
			}
		})
	}
}

func aliceID(alice domain.User) string { return alice.ID }

func TestUserDelete(t *testing.T) {
	f := newUserFixture(t)
	alice := f.register(t, "Alice", "alice@example.com", "s3cret-pass")

	if err := f.users.Delete(context.Background(), alice.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// The user document is removed (D9).
	if _, err := f.users.Get(context.Background(), alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after Delete error = %v, want ErrNotFound", err)
	}
	if err := f.users.Delete(context.Background(), alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second Delete error = %v, want ErrNotFound", err)
	}
	if err := f.users.Delete(context.Background(), "not-an-id"); !errors.Is(err, domain.ErrInvalidID) {
		t.Errorf("Delete(malformed) error = %v, want ErrInvalidID", err)
	}
}

func TestUserServicePassesRepositoryErrorsThrough(t *testing.T) {
	f := newUserFixture(t)
	alice := f.register(t, "Alice", "alice@example.com", "s3cret-pass")
	boom := errors.New("db down")
	f.repo.Err = boom

	calls := map[string]func() error{
		"Get":  func() error { _, err := f.users.Get(context.Background(), alice.ID); return err },
		"List": func() error { _, err := f.users.List(context.Background()); return err },
		"Update": func() error {
			_, err := f.users.Update(context.Background(), alice.ID, UpdateInput{Name: ptr("X")})
			return err
		},
		"Delete": func() error { return f.users.Delete(context.Background(), alice.ID) },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, boom) {
			t.Errorf("%s error = %v, want %v", name, err, boom)
		}
	}
}

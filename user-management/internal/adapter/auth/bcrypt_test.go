package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasher(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)

	hash, err := h.Hash("correct horse")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "correct horse" || !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("Hash = %q, want a bcrypt hash", hash)
	}

	if ok, err := h.Matches(hash, "correct horse"); !ok || err != nil {
		t.Errorf("Matches(right password) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := h.Matches(hash, "wrong horse"); ok || err != nil {
		t.Errorf("Matches(wrong password) = %v, %v; want false, nil", ok, err)
	}
	if ok, err := h.Matches("not-a-bcrypt-hash", "correct horse"); ok || err == nil {
		t.Errorf("Matches(malformed hash) = %v, %v; want false and an error", ok, err)
	}
}

// Regression: bcrypt ignores bytes after the 72nd, so a 72-byte password plus
// any suffix used to match.
func TestBcryptHasherMatchesRejectsOverlongCandidate(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)
	password := strings.Repeat("p", 72)
	hash, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if ok, err := h.Matches(hash, password); !ok || err != nil {
		t.Errorf("Matches(exact 72 bytes) = %v, %v; want true, nil", ok, err)
	}
	for _, candidate := range []string{password + "x", password + "anything"} {
		if ok, err := h.Matches(hash, candidate); ok || err != nil {
			t.Errorf("Matches(%d bytes) = %v, %v; want false, nil", len(candidate), ok, err)
		}
	}
}

func TestBcryptHasherRejectsOverlongPassword(t *testing.T) {
	// The service rejects these first (max 72 bytes); this documents why.
	if _, err := NewBcryptHasher(bcrypt.MinCost).Hash(strings.Repeat("p", 73)); err == nil {
		t.Error("Hash(73 bytes) = nil error, want an error")
	}
}

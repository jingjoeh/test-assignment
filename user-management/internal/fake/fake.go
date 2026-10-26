// Package fake provides hand-written in-memory implementations of the ports
// for service and handler tests (D5). They behave like the real adapters where
// it matters: duplicate emails, malformed IDs and missing users return the same
// domain errors.
package fake

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// UserRepository is an in-memory port.UserRepository. Set Err to make every
// call fail with that error.
type UserRepository struct {
	mu     sync.Mutex
	users  []domain.User // insertion order, so List is oldest first
	nextID int
	Err    error
	// Calls counts method calls, so tests can assert a call didn't happen.
	Calls map[string]int
}

var _ port.UserRepository = (*UserRepository)(nil)

func NewUserRepository() *UserRepository {
	return &UserRepository{Calls: map[string]int{}}
}

// NewID returns a well-formed ID (24 hex characters, like an ObjectID) that
// no stored user has.
func NewID(n int) string {
	return fmt.Sprintf("%024x", n)
}

func validID(id string) bool {
	_, err := hex.DecodeString(id)
	return len(id) == 24 && err == nil
}

func (r *UserRepository) begin(method string) error {
	r.Calls[method]++
	return r.Err
}

func (r *UserRepository) indexOf(id string) int {
	for i, u := range r.users {
		if u.ID == id {
			return i
		}
	}
	return -1
}

func (r *UserRepository) emailTaken(email, exceptID string) bool {
	for _, u := range r.users {
		if u.Email == email && u.ID != exceptID {
			return true
		}
	}
	return false
}

func (r *UserRepository) Create(_ context.Context, u domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("Create"); err != nil {
		return domain.User{}, err
	}
	if r.emailTaken(u.Email, "") {
		return domain.User{}, domain.ErrEmailExists
	}
	r.nextID++
	u.ID = NewID(1_000_000 + r.nextID)
	r.users = append(r.users, u)
	return u, nil
}

func (r *UserRepository) GetByID(_ context.Context, id string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("GetByID"); err != nil {
		return domain.User{}, err
	}
	if !validID(id) {
		return domain.User{}, domain.ErrInvalidID
	}
	i := r.indexOf(id)
	if i < 0 {
		return domain.User{}, domain.ErrNotFound
	}
	return r.users[i], nil
}

func (r *UserRepository) GetByEmail(_ context.Context, email string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("GetByEmail"); err != nil {
		return domain.User{}, err
	}
	for _, u := range r.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (r *UserRepository) List(_ context.Context) ([]domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("List"); err != nil {
		return nil, err
	}
	out := make([]domain.User, len(r.users))
	copy(out, r.users)
	return out, nil
}

func (r *UserRepository) Update(_ context.Context, id string, upd port.UserUpdate) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("Update"); err != nil {
		return domain.User{}, err
	}
	if !validID(id) {
		return domain.User{}, domain.ErrInvalidID
	}
	i := r.indexOf(id)
	if i < 0 {
		return domain.User{}, domain.ErrNotFound
	}
	if upd.Email != nil && r.emailTaken(*upd.Email, id) {
		return domain.User{}, domain.ErrEmailExists
	}
	if upd.Name != nil {
		r.users[i].Name = *upd.Name
	}
	if upd.Email != nil {
		r.users[i].Email = *upd.Email
	}
	return r.users[i], nil
}

func (r *UserRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("Delete"); err != nil {
		return err
	}
	if !validID(id) {
		return domain.ErrInvalidID
	}
	i := r.indexOf(id)
	if i < 0 {
		return domain.ErrNotFound
	}
	r.users = append(r.users[:i], r.users[i+1:]...)
	return nil
}

func (r *UserRepository) Count(_ context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.begin("Count"); err != nil {
		return 0, err
	}
	return int64(len(r.users)), nil
}

// Hasher is a port.PasswordHasher that "hashes" by adding a prefix, so tests
// can see that the stored value isn't the plain password without paying for
// bcrypt.
type Hasher struct {
	// HashErr and MatchErr are set before the hasher is used.
	HashErr  error
	MatchErr error

	mu       sync.Mutex
	compared []string
}

var _ port.PasswordHasher = (*Hasher)(nil)

const hashPrefix = "hashed:"

func (h *Hasher) Hash(password string) (string, error) {
	if h.HashErr != nil {
		return "", h.HashErr
	}
	return hashPrefix + password, nil
}

func (h *Hasher) Matches(hash, password string) (bool, error) {
	h.mu.Lock()
	h.compared = append(h.compared, hash)
	h.mu.Unlock()
	if h.MatchErr != nil {
		return false, h.MatchErr
	}
	return hash == hashPrefix+password, nil
}

// ComparedHashes returns a copy of every hash Matches was called with, in
// call order.
func (h *Hasher) ComparedHashes() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.compared...)
}

// Tokens is a port.TokenIssuer and port.TokenValidator with readable tokens:
// "token-for:<userID>".
type Tokens struct {
	IssueErr error
}

var (
	_ port.TokenIssuer    = (*Tokens)(nil)
	_ port.TokenValidator = (*Tokens)(nil)
)

const tokenPrefix = "token-for:"

// ErrInvalidToken is returned by Validate for anything that isn't a fake token.
var ErrInvalidToken = errors.New("fake: invalid token")

func (t *Tokens) Issue(userID string) (string, error) {
	if t.IssueErr != nil {
		return "", t.IssueErr
	}
	return tokenPrefix + userID, nil
}

func (t *Tokens) Validate(token string) (string, error) {
	id, ok := strings.CutPrefix(token, tokenPrefix)
	if !ok || id == "" {
		return "", ErrInvalidToken
	}
	return id, nil
}

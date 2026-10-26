package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

func TestParseID(t *testing.T) {
	valid := bson.NewObjectID()

	got, err := parseID(valid.Hex())
	if err != nil {
		t.Fatalf("parseID(valid): unexpected error: %v", err)
	}
	if got != valid {
		t.Errorf("parseID(valid) = %s, want %s", got.Hex(), valid.Hex())
	}

	for _, id := range []string{"", "not-an-id", "123", valid.Hex() + "00", "zzzzzzzzzzzzzzzzzzzzzzzz"} {
		if _, err := parseID(id); !errors.Is(err, domain.ErrInvalidID) {
			t.Errorf("parseID(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestMapError(t *testing.T) {
	duplicate := mongo.WriteException{WriteErrors: []mongo.WriteError{{Code: 11000, Message: "E11000 duplicate key"}}}
	other := errors.New("connection reset")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"no documents", mongo.ErrNoDocuments, domain.ErrNotFound},
		{"duplicate key", duplicate, domain.ErrEmailExists},
		{"other error is wrapped", other, other},
		{"deadline is wrapped", context.DeadlineExceeded, context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapError("op", tt.err); !errors.Is(got, tt.want) {
				t.Errorf("mapError(%v) = %v, want errors.Is %v", tt.err, got, tt.want)
			}
		})
	}

	// Infrastructure failures must not be mistaken for domain outcomes.
	if got := mapError("op", other); errors.Is(got, domain.ErrNotFound) || errors.Is(got, domain.ErrEmailExists) {
		t.Errorf("mapError(other) = %v, must not map to a domain error", got)
	}
}

func TestDocumentRoundTrip(t *testing.T) {
	bangkok := time.FixedZone("ICT", 7*60*60)
	in := domain.User{
		Name:         "Alice",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		CreatedAt:    time.Date(2026, 10, 26, 15, 0, 0, 123456789, bangkok),
	}

	doc := toDocument(in)
	doc.ID = bson.NewObjectID()
	out := fromDocument(doc)

	if out.ID != doc.ID.Hex() {
		t.Errorf("ID = %q, want %q", out.ID, doc.ID.Hex())
	}
	if out.Name != in.Name || out.Email != in.Email || out.PasswordHash != in.PasswordHash {
		t.Errorf("fields = %+v, want name/email/hash from %+v", out, in)
	}
	wantTime := time.Date(2026, 10, 26, 8, 0, 0, 123000000, time.UTC)
	if !out.CreatedAt.Equal(wantTime) || out.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v in UTC truncated to milliseconds", out.CreatedAt, wantTime)
	}
}

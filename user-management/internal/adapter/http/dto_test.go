package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
)

func TestUserResponseJSON(t *testing.T) {
	bangkok := time.FixedZone("ICT", 7*60*60)
	u := domain.User{
		ID:           "65f1c0ffee0000000000abcd",
		Name:         "Alice",
		Email:        "alice@example.com",
		PasswordHash: "$2a$10$secrethashvalue",
		CreatedAt:    time.Date(2026, 10, 26, 15, 0, 0, 0, bangkok),
	}

	body, err := json.Marshal(newUserResponse(u))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	want := map[string]any{
		"id":        "65f1c0ffee0000000000abcd",
		"name":      "Alice",
		"email":     "alice@example.com",
		"createdAt": "2026-10-26T08:00:00Z",
	}
	if len(got) != len(want) {
		t.Errorf("JSON keys = %v, want exactly %v", keys(got), keys(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}

	if strings.Contains(strings.ToLower(string(body)), "password") || strings.Contains(string(body), u.PasswordHash) {
		t.Errorf("response leaks the password: %s", body)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

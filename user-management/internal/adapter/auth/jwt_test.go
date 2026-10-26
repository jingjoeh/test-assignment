package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	testSecret = []byte(strings.Repeat("k", 32))
	testNow    = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
)

const testIssuer = "user-management"

func newTestJWT() *JWT {
	j := NewJWT(testSecret, testIssuer, time.Hour)
	j.now = func() time.Time { return testNow }
	return j
}

// sign builds a token with arbitrary claims, so tests can produce tokens the
// issuer never would.
func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub": "user-1",
		"iss": testIssuer,
		"iat": testNow.Unix(),
		"exp": testNow.Add(time.Hour).Unix(),
	}
}

func decodeSegment(t *testing.T, token string, i int) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[i])
	if err != nil {
		t.Fatalf("decode segment %d: %v", i, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal segment %d: %v", i, err)
	}
	return m
}

func TestIssueAndValidate(t *testing.T) {
	j := newTestJWT()

	token, err := j.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if alg := decodeSegment(t, token, 0)["alg"]; alg != "HS256" {
		t.Errorf("header alg = %v, want HS256", alg)
	}
	claims := decodeSegment(t, token, 1)
	if len(claims) != 4 {
		t.Errorf("claims = %v, want exactly sub, iss, iat, exp", claims)
	}
	if claims["sub"] != "user-1" || claims["iss"] != testIssuer {
		t.Errorf("sub/iss = %v/%v", claims["sub"], claims["iss"])
	}
	if iat, exp := claims["iat"].(float64), claims["exp"].(float64); exp-iat != 3600 || int64(iat) != testNow.Unix() {
		t.Errorf("iat = %v, exp = %v, want exp = iat + 1h with iat = now", iat, exp)
	}

	userID, err := j.Validate(token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if userID != "user-1" {
		t.Errorf("Validate userID = %q, want %q", userID, "user-1")
	}
}

func TestValidateRejects(t *testing.T) {
	without := func(key string) jwt.MapClaims {
		c := validClaims()
		delete(c, key)
		return c
	}
	with := func(key string, value any) jwt.MapClaims {
		c := validClaims()
		c[key] = value
		return c
	}
	good := sign(t, jwt.SigningMethodHS256, testSecret, validClaims())
	tampered := func() string {
		parts := strings.Split(good, ".")
		payload, _ := json.Marshal(with("sub", "admin"))
		parts[1] = base64.RawURLEncoding.EncodeToString(payload)
		return strings.Join(parts, ".")
	}()

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not.a.token"},
		{"tampered payload", tampered},
		{"wrong secret", sign(t, jwt.SigningMethodHS256, []byte(strings.Repeat("x", 32)), validClaims())},
		{"wrong alg HS512", sign(t, jwt.SigningMethodHS512, testSecret, validClaims())},
		{"alg none", sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims())},
		{"expired", sign(t, jwt.SigningMethodHS256, testSecret, with("exp", testNow.Add(-time.Second).Unix()))},
		{"issued in the future", sign(t, jwt.SigningMethodHS256, testSecret, with("iat", testNow.Add(time.Hour).Unix()))},
		{"wrong issuer", sign(t, jwt.SigningMethodHS256, testSecret, with("iss", "someone-else"))},
		{"missing sub", sign(t, jwt.SigningMethodHS256, testSecret, without("sub"))},
		{"empty sub", sign(t, jwt.SigningMethodHS256, testSecret, with("sub", ""))},
		{"missing iss", sign(t, jwt.SigningMethodHS256, testSecret, without("iss"))},
		{"missing iat", sign(t, jwt.SigningMethodHS256, testSecret, without("iat"))},
		{"missing exp", sign(t, jwt.SigningMethodHS256, testSecret, without("exp"))},
	}
	j := newTestJWT()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, err := j.Validate(tt.token)
			if !errors.Is(err, ErrInvalidToken) {
				t.Errorf("Validate error = %v, want ErrInvalidToken", err)
			}
			if userID != "" {
				t.Errorf("Validate userID = %q, want empty", userID)
			}
		})
	}

	// Sanity check: the untouched token used above is valid.
	if _, err := j.Validate(good); err != nil {
		t.Fatalf("Validate(good) = %v, want nil", err)
	}
}

func TestValidateExpiresAfterTTL(t *testing.T) {
	j := newTestJWT()
	token, err := j.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	j.now = func() time.Time { return testNow.Add(59 * time.Minute) }
	if _, err := j.Validate(token); err != nil {
		t.Errorf("Validate before expiry = %v, want nil", err)
	}
	j.now = func() time.Time { return testNow.Add(time.Hour + time.Second) }
	if _, err := j.Validate(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Validate after expiry = %v, want ErrInvalidToken", err)
	}
}

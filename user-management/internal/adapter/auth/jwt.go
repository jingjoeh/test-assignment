package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// ErrInvalidToken is returned for any token that must be rejected. The cause
// is wrapped for logging but never shown to the client.
var ErrInvalidToken = errors.New("invalid token")

// JWT issues and validates HS256 access tokens with the claims sub, iss, iat
// and exp (D6, D17).
type JWT struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

var (
	_ port.TokenIssuer    = (*JWT)(nil)
	_ port.TokenValidator = (*JWT)(nil)
)

// NewJWT returns a token service. The config loader guarantees the secret is
// at least 32 bytes.
func NewJWT(secret []byte, issuer string, ttl time.Duration) *JWT {
	return &JWT{secret: secret, issuer: issuer, ttl: ttl, now: time.Now}
}

func (j *JWT) Issue(userID string) (string, error) {
	now := j.now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    j.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign token: %w", err)
	}
	return token, nil
}

func (j *JWT) Validate(token string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), // rejects "none" and every other alg
		jwt.WithIssuer(j.issuer),                                     // iss must be present and match
		jwt.WithExpirationRequired(),                                 // exp must be present and in the future
		jwt.WithIssuedAt(),                                           // iat, when present, must not be in the future
		jwt.WithTimeFunc(j.now),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	// The parser has no "required" option for these two, so check them here.
	if claims.IssuedAt == nil {
		return "", fmt.Errorf("%w: missing iat", ErrInvalidToken)
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("%w: missing sub", ErrInvalidToken)
	}
	return claims.Subject, nil
}

package port

// PasswordHasher hashes passwords and checks them against stored hashes.
type PasswordHasher interface {
	Hash(password string) (string, error)
	// Matches reports whether password matches hash. A mismatch is (false, nil);
	// an error means the check itself failed, for example a malformed hash.
	Matches(hash, password string) (bool, error)
}

// TokenIssuer creates access tokens for an authenticated user.
type TokenIssuer interface {
	Issue(userID string) (string, error)
}

// TokenValidator checks an access token and returns the user ID it was issued
// for. Any error means the token must be rejected.
type TokenValidator interface {
	Validate(token string) (userID string, err error)
}

package domain

import "time"

// User is the user entity from the brief. It carries no JSON or BSON tags:
// adapters map it to their own representations.
type User struct {
	// ID is an opaque string. Only the Mongo adapter knows it is an ObjectID (D15).
	ID           string
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

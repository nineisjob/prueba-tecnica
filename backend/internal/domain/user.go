package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID
	Email        string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// AuthUser is the reduced identity carried in request context after JWT validation.
type AuthUser struct {
	ID       uuid.UUID
	Username string
}

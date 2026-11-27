package domain

import (
	"time"

	"github.com/google/uuid"
)

// User — зарегистрированный пользователь.
type User struct {
	ID         uuid.UUID
	ExternalID string
	CreatedAt  time.Time
}

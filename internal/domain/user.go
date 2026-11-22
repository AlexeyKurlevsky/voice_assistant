package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID         uuid.UUID
	ExternalID string
	CreatedAt  time.Time
}

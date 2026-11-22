package domain

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID           uuid.UUID
	MeetingID    uuid.UUID
	Status       Status
	ErrorMessage string
	Attempts     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

package domain

import (
	"time"

	"github.com/google/uuid"
)

type Meeting struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	Title            string
	OriginalFilename string
	FilePath         string
	CreatedAt        time.Time
}

type MeetingListItem struct {
	ID        uuid.UUID
	Title     string
	Status    Status
	Summary   string
	CreatedAt time.Time
}

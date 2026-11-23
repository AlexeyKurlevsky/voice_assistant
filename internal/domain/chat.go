package domain

import (
	"time"

	"github.com/google/uuid"
)

// QA — сохранённая пара «вопрос-ответ» по материалам встречи.
type QA struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	MeetingID uuid.UUID
	Question  string
	Answer    string
	CreatedAt time.Time
}

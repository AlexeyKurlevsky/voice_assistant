package domain

import (
	"time"

	"github.com/google/uuid"
)

// Task — задача обработки одной встречи (1:1 с Meeting).
type Task struct {
	ID           uuid.UUID
	MeetingID    uuid.UUID
	Status       Status
	ErrorMessage string
	Attempts     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// StatusChange — запись из status_history.
// предыдущий статус восстанавливается по
// последовательности записей (см. docs/STATUS_MODEL.md).
type StatusChange struct {
	TaskID    uuid.UUID
	Status    Status
	CreatedAt time.Time
}

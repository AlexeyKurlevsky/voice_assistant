package domain

import (
	"time"

	"github.com/google/uuid"
)

// Task — задача обработки одной встречи (1:1 с Meeting).
// Повторная загрузка того же файла создаёт новую встречу и новую задачу.
type Task struct {
	ID           uuid.UUID
	MeetingID    uuid.UUID
	Status       Status
	ErrorMessage string // заполнен только при StatusFailed
	Attempts     int    // растёт при каждом created → processing
	CreatedAt    time.Time
	UpdatedAt    time.Time // момент последнего перехода статуса
}

// StatusChange — запись из status_history.
// OldStatus == "" — первая запись задачи (в БД old_status = NULL).
type StatusChange struct {
	TaskID    uuid.UUID
	OldStatus Status
	NewStatus Status
	CreatedAt time.Time
}

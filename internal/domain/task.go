package domain

import "time"

type Task struct {
	ID           int64
	MeetingID    int64
	Status       Status
	ErrorMessage string
	Attempts     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

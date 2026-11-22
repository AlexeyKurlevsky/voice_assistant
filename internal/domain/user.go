package domain

import "time"

type User struct {
	ID         int64
	ExternalID string
	CreatedAt  time.Time
}

package domain

import (
	"time"

	"github.com/google/uuid"
)

// User — зарегистрированный пользователь.
// ExternalID — то, что вводит пользователь (--user alice); ID — внутренний UUID.
// Разделение защищает от утечки чужих ExternalID через ошибки и логи.
type User struct {
	ID         uuid.UUID
	ExternalID string
	CreatedAt  time.Time
}

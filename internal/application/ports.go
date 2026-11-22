package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type UserRepo interface {
	EnsureExists(ctx context.Context, externalID string) (*domain.User, error)
	GetByExternalID(ctx context.Context, externalID string) (*domain.User, error)
}

type MeetingRepo interface {
	CreateWithTask(ctx context.Context, m *domain.Meeting) (*domain.Task, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.MeetingListItem, error)
}

type TaskRepo interface {
	MarkFailed(ctx context.Context, taskID uuid.UUID, errMsg string) error
}

type Enqueuer interface {
	Enqueue(ctx context.Context, taskID uuid.UUID) error
}

package application

import (
	"context"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type UserRepo interface {
	EnsureExists(ctx context.Context, externalID string) (*domain.User, error)
	GetByExternalID(ctx context.Context, externalID string) (*domain.User, error)
}

type MeetingRepo interface {
	// CreateWithTask — атомарно создаёт встречу и задачу обработки
	// (status=created) вместе с записью в status_history.
	CreateWithTask(ctx context.Context, m *domain.Meeting) (*domain.Task, error)

	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.MeetingListItem, error)
}

type TaskRepo interface {
	// MarkFailed — атомарно переводит задачу в status=failed
	// и записывает error_message в status_history.
	MarkFailed(ctx context.Context, taskID int64, errMsg string) error
}

// Enqueuer — минимальный контракт постановки задачи в очередь обработки.
// Реализация — application.WorkerPool (появится в 2b).
type Enqueuer interface {
	Enqueue(ctx context.Context, taskID int64) error
}

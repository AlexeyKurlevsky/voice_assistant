package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres/sqlcgen"
)

// ChatRepo — реализация application.ChatRepo.
type ChatRepo struct {
	q *sqlcgen.Queries
}

// NewChatRepo создаёт репозиторий поверх пула соединений.
func NewChatRepo(pool *pgxpool.Pool) *ChatRepo {
	return &ChatRepo{q: sqlcgen.New(pool)}
}

// Save сохраняет пару «вопрос-ответ» и заполняет qa.ID и qa.CreatedAt.
func (r *ChatRepo) Save(ctx context.Context, qa *domain.QA) error {
	row, err := r.q.SaveChatMessage(ctx, sqlcgen.SaveChatMessageParams{
		UserID:    toPgUUID(qa.UserID),
		MeetingID: toPgUUID(qa.MeetingID),
		Question:  qa.Question,
		Answer:    qa.Answer,
	})
	if err != nil {
		return fmt.Errorf("chat_repo.Save: %w", err)
	}
	qa.ID = toUUID(row.ID)
	qa.CreatedAt = row.CreatedAt
	return nil
}

// History — последние N пар по встрече, по убыванию времени.
func (r *ChatRepo) History(ctx context.Context, userID, meetingID uuid.UUID, limit int) ([]domain.QA, error) {
	rows, err := r.q.HistoryChatMessages(ctx, sqlcgen.HistoryChatMessagesParams{
		UserID:    toPgUUID(userID),
		MeetingID: toPgUUID(meetingID),
	})
	if err != nil {
		return nil, fmt.Errorf("chat_repo.History: %w", err)
	}
	out := make([]domain.QA, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.QA{
			ID:        toUUID(row.ID),
			UserID:    toUUID(row.UserID),
			MeetingID: toUUID(row.MeetingID),
			Question:  row.Question,
			Answer:    row.Answer,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres/sqlcgen"
)

type MeetingRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewMeetingRepo(pool *pgxpool.Pool) *MeetingRepo {
	return &MeetingRepo{pool: pool, q: sqlcgen.New(pool)}
}

func (r *MeetingRepo) CreateWithTask(ctx context.Context, m *domain.Meeting) (*domain.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("meeting_repo.CreateWithTask: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	mRow, err := qtx.CreateMeeting(ctx, sqlcgen.CreateMeetingParams{
		UserID:           toPgUUID(m.UserID),
		Title:            m.Title,
		OriginalFilename: m.OriginalFilename,
		FilePath:         m.FilePath,
	})
	if err != nil {
		return nil, fmt.Errorf("meeting_repo.CreateWithTask: insert meeting: %w", err)
	}
	m.ID = toUUID(mRow.ID)
	m.CreatedAt = mRow.CreatedAt

	tRow, err := qtx.CreateTask(ctx, sqlcgen.CreateTaskParams{
		MeetingID: mRow.ID,
		Status:    string(domain.StatusCreated),
	})
	if err != nil {
		return nil, fmt.Errorf("meeting_repo.CreateWithTask: insert task: %w", err)
	}

	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID:    tRow.ID,
		OldStatus: nil,
		NewStatus: string(domain.StatusCreated),
	}); err != nil {
		return nil, fmt.Errorf("meeting_repo.CreateWithTask: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("meeting_repo.CreateWithTask: commit: %w", err)
	}

	return &domain.Task{
		ID:        toUUID(tRow.ID),
		MeetingID: toUUID(tRow.MeetingID),
		Status:    domain.Status(tRow.Status),
		Attempts:  int(tRow.Attempts),
		CreatedAt: tRow.CreatedAt,
		UpdatedAt: tRow.UpdatedAt,
	}, nil
}

func (r *MeetingRepo) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.MeetingListItem, error) {
	rows, err := r.q.ListMeetingsByUser(ctx, sqlcgen.ListMeetingsByUserParams{
		UserID: toPgUUID(userID),
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("meeting_repo.ListByUser: %w", err)
	}

	out := make([]domain.MeetingListItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.MeetingListItem{
			ID:        toUUID(row.ID),
			Title:     row.Title,
			Status:    domain.Status(row.Status),
			Summary:   row.Summary,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

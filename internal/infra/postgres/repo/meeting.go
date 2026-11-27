package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres/sqlcgen"
)

// MeetingRepo — реализация application.MeetingRepo.
type MeetingRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewMeetingRepo создаёт репозиторий поверх пула соединений.
func NewMeetingRepo(pool *pgxpool.Pool) *MeetingRepo {
	return &MeetingRepo{pool: pool, q: sqlcgen.New(pool)}
}

// CreateWithTask — транзакция 1 (docs/DATA_MODEL.md §4).
// Атомарно: INSERT meetings + INSERT processing_tasks + INSERT status_history.
// Побочно заполняет m.ID и m.CreatedAt.
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
		ContentHash:      &m.ContentHash,
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
		TaskID: tRow.ID,
		Status: string(domain.StatusCreated),
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

// GetByContentHashForUser — встреча пользователя по SHA-256 содержимого.
// ErrNotFound, если встречи с таким хэшем нет.
func (r *MeetingRepo) GetByContentHashForUser(ctx context.Context, userID uuid.UUID, contentHash string) (*domain.Meeting, error) {
	row, err := r.q.GetMeetingByContentHashForUser(ctx, sqlcgen.GetMeetingByContentHashForUserParams{
		UserID:      toPgUUID(userID),
		ContentHash: &contentHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("meeting_repo.GetByContentHashForUser: %w", err)
	}
	return &domain.Meeting{
		ID:               toUUID(row.ID),
		UserID:           toUUID(row.UserID),
		Title:            row.Title,
		OriginalFilename: row.OriginalFilename,
		FilePath:         row.FilePath,
		CreatedAt:        row.CreatedAt,
	}, nil
}

// ListByUser возвращает встречи пользователя, created_at DESC.
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

// GetByIDForUser — встреча с проверкой прав.
// ErrNotFound и для «нет», и для «чужая» (ADR-7).
func (r *MeetingRepo) GetByIDForUser(ctx context.Context, meetingID, userID uuid.UUID) (*domain.Meeting, error) {
	row, err := r.q.GetMeetingForUser(ctx, sqlcgen.GetMeetingForUserParams{
		ID:     toPgUUID(meetingID),
		UserID: toPgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("meeting_repo.GetByIDForUser: %w", err)
	}
	return &domain.Meeting{
		ID:               toUUID(row.ID),
		UserID:           toUUID(row.UserID),
		Title:            row.Title,
		OriginalFilename: row.OriginalFilename,
		FilePath:         row.FilePath,
		CreatedAt:        row.CreatedAt,
	}, nil
}

// GetByTaskID — встреча по task_id без проверки прав.
// Для фонового processor'а; не использовать в пользовательских сценариях.
func (r *MeetingRepo) GetByTaskID(ctx context.Context, taskID uuid.UUID) (*domain.Meeting, error) {
	row, err := r.q.GetMeetingByTaskID(ctx, toPgUUID(taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("meeting_repo.GetByTaskID: %w", err)
	}
	return &domain.Meeting{
		ID:               toUUID(row.ID),
		UserID:           toUUID(row.UserID),
		Title:            row.Title,
		OriginalFilename: row.OriginalFilename,
		FilePath:         row.FilePath,
		CreatedAt:        row.CreatedAt,
	}, nil
}

// GetTranscript — транскрипция с проверкой прав.
// ErrNotFound, если встреча чужая или транскрипции ещё нет.
func (r *MeetingRepo) GetTranscript(ctx context.Context, meetingID, userID uuid.UUID) (string, error) {
	text, err := r.q.GetTranscriptForUser(ctx, sqlcgen.GetTranscriptForUserParams{
		MeetingID: toPgUUID(meetingID),
		UserID:    toPgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("meeting_repo.GetTranscript: %w", err)
	}
	return text, nil
}

// GetSummary — выжимка с проверкой прав.
func (r *MeetingRepo) GetSummary(ctx context.Context, meetingID, userID uuid.UUID) (string, error) {
	text, err := r.q.GetSummaryForUser(ctx, sqlcgen.GetSummaryForUserParams{
		MeetingID: toPgUUID(meetingID),
		UserID:    toPgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("meeting_repo.GetSummary: %w", err)
	}
	return text, nil
}

// Search — полнотекстовый поиск по встречам пользователя.
func (r *MeetingRepo) Search(ctx context.Context, userID uuid.UUID, query string) ([]domain.SearchHit, error) {
	rows, err := r.q.SearchMeetings(ctx, sqlcgen.SearchMeetingsParams{
		UserID: toPgUUID(userID),
		Query:  query,
	})
	if err != nil {
		return nil, fmt.Errorf("meeting_repo.Search: %w", err)
	}

	out := make([]domain.SearchHit, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SearchHit{
			ID:        toUUID(row.ID),
			Title:     row.Title,
			Status:    domain.Status(row.Status),
			Snippet:   row.Snippet,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

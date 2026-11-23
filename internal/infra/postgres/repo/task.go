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

// TaskRepo — реализация application.TaskRepo.
type TaskRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewTaskRepo создаёт репозиторий поверх пула соединений.
func NewTaskRepo(pool *pgxpool.Pool) *TaskRepo {
	return &TaskRepo{pool: pool, q: sqlcgen.New(pool)}
}

// MarkProcessing — переход created → processing.
// Если задача уже не в created — no-op (не ошибка).
func (r *TaskRepo) MarkProcessing(ctx context.Context, taskID uuid.UUID) error {
	return r.transition(ctx, taskID,
		func(q *sqlcgen.Queries) (int64, error) {
			return q.MarkTaskProcessing(ctx, toPgUUID(taskID))
		},
		domain.StatusProcessing,
	)
}

// MarkCompleted — переход summarized → completed.
// Если задача уже не в summarized — no-op.
func (r *TaskRepo) MarkCompleted(ctx context.Context, taskID uuid.UUID) error {
	return r.transition(ctx, taskID,
		func(q *sqlcgen.Queries) (int64, error) {
			return q.MarkTaskCompleted(ctx, toPgUUID(taskID))
		},
		domain.StatusCompleted,
	)
}

// MarkFailed — перевод в failed. Не трогает терминальные статусы:
// если задача уже completed или failed, UPDATE не сработает,
// и completed → failed (гонка двух воркеров) невозможна.
func (r *TaskRepo) MarkFailed(ctx context.Context, taskID uuid.UUID, errMsg string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("task_repo.MarkFailed: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	rows, err := qtx.MarkTaskFailed(ctx, sqlcgen.MarkTaskFailedParams{
		ID:           toPgUUID(taskID),
		ErrorMessage: &errMsg,
	})
	if err != nil {
		return fmt.Errorf("task_repo.MarkFailed: update: %w", err)
	}
	if rows == 0 {
		// Задача уже терминальная — не перезаписываем.
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("task_repo.MarkFailed: commit no-op: %w", err)
		}
		return nil
	}

	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID: toPgUUID(taskID),
		Status: string(domain.StatusFailed),
	}); err != nil {
		return fmt.Errorf("task_repo.MarkFailed: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.MarkFailed: commit: %w", err)
	}
	return nil
}

// transition — общий путь для «UPDATE status + INSERT history».
// Если UPDATE не затронул ни одной строки (задача уже в другом статусе),
// история не пишется — это no-op, не ошибка.
func (r *TaskRepo) transition(
	ctx context.Context,
	taskID uuid.UUID,
	doUpdate func(q *sqlcgen.Queries) (int64, error),
	newStatus domain.Status,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("task_repo.transition: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	rows, err := doUpdate(qtx)
	if err != nil {
		return fmt.Errorf("task_repo.transition: update: %w", err)
	}
	if rows == 0 {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("task_repo.transition: commit no-op: %w", err)
		}
		return nil
	}

	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID: toPgUUID(taskID),
		Status: string(newStatus),
	}); err != nil {
		return fmt.Errorf("task_repo.transition: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.transition: commit: %w", err)
	}
	return nil
}

// SaveTranscriptionAndMarkTranscribed: INSERT transcriptions +
// UPDATE status processing → transcribed + history.
// Идемпотентен: повторный INSERT — no-op (ON CONFLICT DO NOTHING),
// повторный UPDATE — 0 строк, история не пишется.
func (r *TaskRepo) SaveTranscriptionAndMarkTranscribed(ctx context.Context, taskID uuid.UUID, text string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	t, err := qtx.GetTaskByID(ctx, toPgUUID(taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("task_repo.SaveTranscription: get task: %w", err)
	}

	if err := qtx.InsertTranscription(ctx, sqlcgen.InsertTranscriptionParams{
		MeetingID: t.MeetingID,
		Text:      text,
	}); err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: insert: %w", err)
	}

	rows, err := qtx.MarkTaskTranscribed(ctx, toPgUUID(taskID))
	if err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: update status: %w", err)
	}
	if rows == 0 {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("task_repo.SaveTranscription: commit no-op: %w", err)
		}
		return nil
	}

	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID: toPgUUID(taskID),
		Status: string(domain.StatusTranscribed),
	}); err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: commit: %w", err)
	}
	return nil
}

// SaveSummaryAndMarkSummarized: INSERT summaries +
// UPDATE status transcribed → summarized + history.
// Идемпотентен аналогично SaveTranscription.
func (r *TaskRepo) SaveSummaryAndMarkSummarized(ctx context.Context, taskID uuid.UUID, text string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("task_repo.SaveSummary: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	t, err := qtx.GetTaskByID(ctx, toPgUUID(taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("task_repo.SaveSummary: get task: %w", err)
	}

	if err := qtx.InsertSummary(ctx, sqlcgen.InsertSummaryParams{
		MeetingID: t.MeetingID,
		Text:      text,
	}); err != nil {
		return fmt.Errorf("task_repo.SaveSummary: insert: %w", err)
	}

	rows, err := qtx.MarkTaskSummarized(ctx, toPgUUID(taskID))
	if err != nil {
		return fmt.Errorf("task_repo.SaveSummary: update status: %w", err)
	}
	if rows == 0 {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("task_repo.SaveSummary: commit no-op: %w", err)
		}
		return nil
	}

	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID: toPgUUID(taskID),
		Status: string(domain.StatusSummarized),
	}); err != nil {
		return fmt.Errorf("task_repo.SaveSummary: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.SaveSummary: commit: %w", err)
	}
	return nil
}

// GetByID — задача по ID без проверки прав.
func (r *TaskRepo) GetByID(ctx context.Context, taskID uuid.UUID) (*domain.Task, error) {
	row, err := r.q.GetTaskByID(ctx, toPgUUID(taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("task_repo.GetByID: %w", err)
	}
	return toDomainTask(row), nil
}

// GetByMeetingIDForUser — задача встречи с проверкой прав.
func (r *TaskRepo) GetByMeetingIDForUser(ctx context.Context, meetingID, userID uuid.UUID) (*domain.Task, error) {
	row, err := r.q.GetTaskForMeetingAndUser(ctx, sqlcgen.GetTaskForMeetingAndUserParams{
		MeetingID: toPgUUID(meetingID),
		UserID:    toPgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("task_repo.GetByMeetingIDForUser: %w", err)
	}
	return &domain.Task{
		ID:           toUUID(row.ID),
		MeetingID:    toUUID(row.MeetingID),
		Status:       domain.Status(row.Status),
		ErrorMessage: derefString(row.ErrorMessage),
		Attempts:     int(row.Attempts),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

// ListStatusHistory — история переходов, по возрастанию времени.
func (r *TaskRepo) ListStatusHistory(ctx context.Context, taskID uuid.UUID) ([]domain.StatusChange, error) {
	rows, err := r.q.ListStatusHistory(ctx, toPgUUID(taskID))
	if err != nil {
		return nil, fmt.Errorf("task_repo.ListStatusHistory: %w", err)
	}
	out := make([]domain.StatusChange, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.StatusChange{
			TaskID:    taskID,
			Status:    domain.Status(row.Status),
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

// ResetStaleProcessing — recovery: processing → created, возвращает task_id[].
func (r *TaskRepo) ResetStaleProcessing(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.q.ResetStaleProcessing(ctx)
	if err != nil {
		return nil, fmt.Errorf("task_repo.ResetStaleProcessing: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, id := range rows {
		out = append(out, toUUID(id))
	}
	return out, nil
}

// ListPending — задачи в незавершённых статусах (для дозахвата на старте).
func (r *TaskRepo) ListPending(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.q.ListPendingTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("task_repo.ListPending: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, id := range rows {
		out = append(out, toUUID(id))
	}
	return out, nil
}

// toDomainTask маппит sqlcgen.ProcessingTask в domain.Task.
func toDomainTask(row sqlcgen.ProcessingTask) *domain.Task {
	return &domain.Task{
		ID:           toUUID(row.ID),
		MeetingID:    toUUID(row.MeetingID),
		Status:       domain.Status(row.Status),
		ErrorMessage: derefString(row.ErrorMessage),
		Attempts:     int(row.Attempts),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

// derefString разыменовывает *string, возвращая "" для nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

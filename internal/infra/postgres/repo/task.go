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

// MarkProcessing — T2: created → processing, attempts++.
func (r *TaskRepo) MarkProcessing(ctx context.Context, taskID uuid.UUID) error {
	return r.transition(ctx, taskID,
		func(q *sqlcgen.Queries) error {
			return q.MarkTaskProcessing(ctx, toPgUUID(taskID))
		},
		domain.StatusProcessing,
	)
}

// MarkCompleted — T5: summarized → completed.
func (r *TaskRepo) MarkCompleted(ctx context.Context, taskID uuid.UUID) error {
	return r.transition(ctx, taskID,
		func(q *sqlcgen.Queries) error {
			return q.MarkTaskCompleted(ctx, toPgUUID(taskID))
		},
		domain.StatusCompleted,
	)
}

// MarkFailed — T6: перевод в failed с error_message.
func (r *TaskRepo) MarkFailed(ctx context.Context, taskID uuid.UUID, errMsg string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("task_repo.MarkFailed: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)
	cur, err := qtx.GetTaskStatus(ctx, toPgUUID(taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("task_repo.MarkFailed: get status: %w", err)
	}

	if err := qtx.MarkTaskFailed(ctx, sqlcgen.MarkTaskFailedParams{
		ID:           toPgUUID(taskID),
		ErrorMessage: &errMsg,
	}); err != nil {
		return fmt.Errorf("task_repo.MarkFailed: update: %w", err)
	}

	oldStatus := cur
	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID:    toPgUUID(taskID),
		OldStatus: &oldStatus,
		NewStatus: string(domain.StatusFailed),
	}); err != nil {
		return fmt.Errorf("task_repo.MarkFailed: insert history: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.MarkFailed: commit: %w", err)
	}
	return nil
}

// transition — общий путь для «UPDATE + INSERT history» переходов.
func (r *TaskRepo) transition(
	ctx context.Context,
	taskID uuid.UUID,
	doUpdate func(q *sqlcgen.Queries) error,
	newStatus domain.Status,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("task_repo.transition: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	cur, err := qtx.GetTaskStatus(ctx, toPgUUID(taskID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("task_repo.transition: get status: %w", err)
	}

	if err := doUpdate(qtx); err != nil {
		return fmt.Errorf("task_repo.transition: update: %w", err)
	}

	oldStatus := cur
	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID:    toPgUUID(taskID),
		OldStatus: &oldStatus,
		NewStatus: string(newStatus),
	}); err != nil {
		return fmt.Errorf("task_repo.transition: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.transition: commit: %w", err)
	}
	return nil
}

// SaveTranscriptionAndMarkTranscribed — T3.
// Атомарно: INSERT transcriptions + UPDATE status + INSERT history.
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

	if err := qtx.MarkTaskTranscribed(ctx, toPgUUID(taskID)); err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: update status: %w", err)
	}

	oldStatus := string(t.Status)
	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID:    toPgUUID(taskID),
		OldStatus: &oldStatus,
		NewStatus: string(domain.StatusTranscribed),
	}); err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: insert history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("task_repo.SaveTranscription: commit: %w", err)
	}
	return nil
}

// SaveSummaryAndMarkSummarized — T4.
// Атомарно: INSERT summaries + UPDATE status + INSERT history.
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

	if err := qtx.MarkTaskSummarized(ctx, toPgUUID(taskID)); err != nil {
		return fmt.Errorf("task_repo.SaveSummary: update status: %w", err)
	}

	oldStatus := string(t.Status)
	if err := qtx.InsertStatusHistory(ctx, sqlcgen.InsertStatusHistoryParams{
		TaskID:    toPgUUID(taskID),
		OldStatus: &oldStatus,
		NewStatus: string(domain.StatusSummarized),
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

// ListStatusHistory — история переходов задачи, по возрастанию времени.
func (r *TaskRepo) ListStatusHistory(ctx context.Context, taskID uuid.UUID) ([]domain.StatusChange, error) {
	rows, err := r.q.ListStatusHistory(ctx, toPgUUID(taskID))
	if err != nil {
		return nil, fmt.Errorf("task_repo.ListStatusHistory: %w", err)
	}
	out := make([]domain.StatusChange, 0, len(rows))
	for _, row := range rows {
		old := ""
		if row.OldStatus != nil {
			old = *row.OldStatus
		}
		out = append(out, domain.StatusChange{
			TaskID:    toUUID(row.TaskID),
			OldStatus: domain.Status(old),
			NewStatus: domain.Status(row.NewStatus),
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

// ResetStaleProcessing — recovery: processing → created, возвращает task_id[].
// В status_history не пишет (это восстановление, не бизнес-переход).
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

// ListPending — задачи в статусе created (для дозахвата на старте).
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

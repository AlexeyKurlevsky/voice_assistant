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

type TaskRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewTaskRepo(pool *pgxpool.Pool) *TaskRepo {
	return &TaskRepo{pool: pool, q: sqlcgen.New(pool)}
}

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

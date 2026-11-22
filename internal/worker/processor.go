package worker

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

func NewProcessor(log *slog.Logger) application.TaskProcessor {
	return func(ctx context.Context, taskID uuid.UUID) error {
		log.Info("worker: task picked (stub, processor in next batch)", "task_id", taskID)
		return nil
	}
}

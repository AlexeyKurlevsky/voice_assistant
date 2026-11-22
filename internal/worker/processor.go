// Package worker содержит фоновую обработку задач: pipeline
// transcribe → summarize → finalize, вызываемый WorkerPool'ом.
package worker

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

// NewProcessor возвращает application.TaskProcessor — обработчик одной задачи.
//
// Стаб: логирует task_id и возвращает nil. Реальный pipeline
// (MarkProcessing → SpeechClient.Transcribe → SaveTranscription →
// LLMClient.Summarize → SaveSummary → MarkCompleted) появится в батче 3b.
func NewProcessor(log *slog.Logger) application.TaskProcessor {
	return func(ctx context.Context, taskID uuid.UUID) error {
		log.Info("worker: task picked (stub, processor in next batch)", "task_id", taskID)
		return nil
	}
}

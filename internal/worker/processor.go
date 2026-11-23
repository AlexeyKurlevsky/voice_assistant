// Package worker содержит фоновую обработку задач: pipeline
// transcribe → summarize → finalize, вызываемый WorkerPool'ом.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// Config — параметры процессора.
type Config struct {
	// UploadDir — корень, относительно которого открывается meeting.FilePath.
	UploadDir string

	// SpeechTimeout — таймаут на SpeechClient.Transcribe.
	SpeechTimeout time.Duration

	// LLMTimeout — таймаут на LLMClient.Summarize.
	LLMTimeout time.Duration
}

// NewProcessor возвращает application.TaskProcessor — обработчик одной задачи.
//
// Pipeline: created → processing → transcribed → summarized → completed.
// На любую ошибку — MarkFailed с причиной.
//
// Recovery: если задача пришла в статусе summarized (крэш между
// SaveSummaryAndMarkSummarized и MarkCompleted), пропускает speech и LLM
// и сразу выполняет MarkCompleted — без повторного вызова платных API.
func NewProcessor(
	taskRepo application.TaskRepo,
	meetingRepo application.MeetingRepo,
	speech application.SpeechClient,
	llm application.LLMClient,
	cfg Config,
	log *slog.Logger,
) application.TaskProcessor {
	return func(ctx context.Context, taskID uuid.UUID) error {
		return process(ctx, taskID, taskRepo, meetingRepo, speech, llm, cfg, log)
	}
}

func process(
	ctx context.Context,
	taskID uuid.UUID,
	taskRepo application.TaskRepo,
	meetingRepo application.MeetingRepo,
	speech application.SpeechClient,
	llm application.LLMClient,
	cfg Config,
	log *slog.Logger,
) error {
	task, err := taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("worker: get task: %w", err)
	}

	if task.Status.IsTerminal() {
		log.Info("worker: task terminal, skip", "task_id", taskID, "status", task.Status)
		return nil
	}

	// Recovery: summarized → completed без повторного вызова LLM.
	if task.Status == domain.StatusSummarized {
		if err := taskRepo.MarkCompleted(ctx, taskID); err != nil {
			return fmt.Errorf("worker: mark completed (recovery): %w", err)
		}
		log.Info("worker: task finalized (recovery)", "task_id", taskID)
		return nil
	}

	// created → processing
	if task.Status == domain.StatusCreated {
		if err := taskRepo.MarkProcessing(ctx, taskID); err != nil {
			return fmt.Errorf("worker: mark processing: %w", err)
		}
		task.Status = domain.StatusProcessing
	}

	meeting, err := meetingRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return fail(ctx, taskRepo, taskID, fmt.Errorf("get meeting: %w", err), log)
	}

	// Транскрипция: получаем или создаём.
	var transcript string
	if task.Status == domain.StatusTranscribed {
		// Recovery: транскрипция уже сохранена в прошлый раз.
		transcript, err = meetingRepo.GetTranscript(ctx, meeting.ID, meeting.UserID)
		if err != nil {
			return fail(ctx, taskRepo, taskID, fmt.Errorf("get transcript: %w", err), log)
		}
	} else {
		filePath := filepath.Join(cfg.UploadDir, meeting.FilePath)
		f, err := os.Open(filePath)
		if err != nil {
			return fail(ctx, taskRepo, taskID, fmt.Errorf("open file %s: %w", filePath, err), log)
		}
		defer f.Close()

		speechCtx, cancel := context.WithTimeout(ctx, cfg.SpeechTimeout)
		transcript, err = speech.Transcribe(speechCtx, meeting.OriginalFilename, f)
		cancel()
		if err != nil {
			return fail(ctx, taskRepo, taskID, fmt.Errorf("speech: %w", err), log)
		}

		if err := taskRepo.SaveTranscriptionAndMarkTranscribed(ctx, taskID, transcript); err != nil {
			return fail(ctx, taskRepo, taskID, fmt.Errorf("save transcription: %w", err), log)
		}
	}

	// LLM: выжимка.
	llmCtx, cancel := context.WithTimeout(ctx, cfg.LLMTimeout)
	summary, err := llm.Summarize(llmCtx, transcript)
	cancel()
	if err != nil {
		return fail(ctx, taskRepo, taskID, fmt.Errorf("llm summarize: %w", err), log)
	}

	if err := taskRepo.SaveSummaryAndMarkSummarized(ctx, taskID, summary); err != nil {
		return fail(ctx, taskRepo, taskID, fmt.Errorf("save summary: %w", err), log)
	}

	if err := taskRepo.MarkCompleted(ctx, taskID); err != nil {
		return fmt.Errorf("worker: mark completed: %w", err)
	}

	log.Info("worker: task completed", "task_id", taskID)
	return nil
}

// fail помечает задачу failed с отдельным контекстом (переживает уже
// отменённый родительский ctx) и возвращает исходную ошибку.
// Не логирует сама — WorkerPool логирует возвращённое значение.
func fail(
	ctx context.Context,
	taskRepo application.TaskRepo,
	taskID uuid.UUID,
	cause error,
	log *slog.Logger,
) error {
	failCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := taskRepo.MarkFailed(failCtx, taskID, cause.Error()); err != nil {
		log.Error("worker: mark failed", "task_id", taskID, "cause", cause, "err", err)
	}
	return cause
}

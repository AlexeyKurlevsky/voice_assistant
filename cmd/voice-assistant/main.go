// Command voice-assistant — CLI для загрузки и обработки встреч.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/cli"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/config"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/llm"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres/repo"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/speech"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/logger"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/worker"
	"github.com/google/uuid"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}

	log := logger.New(cfg)
	slog.SetDefault(log)

	os.Exit(run(cfg, log))
}

func run(cfg *config.Config, log *slog.Logger) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Пул соединений
	pool, err := postgres.NewPool(ctx, cfg.DBDSN, cfg.DBMaxConns)
	if err != nil {
		log.Error("postgres pool", "err", err)
		return 3
	}
	defer pool.Close()

	// Миграции
	if cfg.MigrateOnStart {
		if err := postgres.Migrate(ctx, cfg.DBDSN); err != nil {
			log.Error("migrate", "err", err)
			return 3
		}
	}

	// Внешние клиенты (пока mock)
	speechClient, err := speech.New(cfg.SpeechProvider, speech.Options{
		MockDelay: cfg.MockSpeechDelay,
	})
	if err != nil {
		log.Error("speech factory", "err", err)
		return 3
	}
	llmClient, err := llm.New(cfg.LLMProvider, llm.Options{
		MockDelay: cfg.MockLLMDelay,
	})
	if err != nil {
		log.Error("llm factory", "err", err)
		return 3
	}

	// Репозитории
	userRepo := repo.NewUserRepo(pool)
	meetingRepo := repo.NewMeetingRepo(pool)
	taskRepo := repo.NewTaskRepo(pool)

	// Worker pool с реальным процессором
	processor := worker.NewProcessor(
		taskRepo, meetingRepo, speechClient, llmClient,
		worker.Config{
			UploadDir:     cfg.UploadDir,
			SpeechTimeout: cfg.SpeechTimeout,
			LLMTimeout:    cfg.LLMTimeout,
		},
		log,
	)
	wp := application.NewWorkerPool(
		cfg.WorkerConcurrency,
		cfg.JobQueueCapacity,
		processor,
		log,
	)
	wp.Start(ctx)

	// Recovery при старте: зависшие processing → created + добрать
	// created/transcribed/summarized, оставшиеся с прошлого запуска.
	recoverTasks(ctx, taskRepo, wp, log)

	// Сервисы
	userService := application.NewUserService(userRepo)
	meetingService := application.NewMeetingService(
		meetingRepo, taskRepo, wp, cfg.UploadDir, cfg.MaxUploadBytes, log,
	)

	// CLI
	deps := &cli.Deps{
		UserService:    userService,
		MeetingService: meetingService,
	}
	root := cli.NewRoot(deps)

	cliErr := root.ExecuteContext(ctx)

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := wp.Stop(shutdownCtx); err != nil {
		log.Warn("worker pool stop", "err", err)
	}

	if cliErr != nil && !errors.Is(cliErr, context.Canceled) {
		log.Error("cli", "err", cliErr)
		return exitCodeFor(cliErr)
	}
	return 0
}

// recoverTasks сбрасывает «зависшие» processing → created и ставит в очередь
// все незавершённые задачи (created/transcribed/summarized).
// Дедупликация обязательна: ResetStaleProcessing переводит задачи в created,
// и ListPendingTasks сразу же их возвращает — без дедупликации задача
// попадает в очередь дважды, и два воркера берут её параллельно.
func recoverTasks(ctx context.Context, taskRepo application.TaskRepo, wp *application.WorkerPool, log *slog.Logger) int {
	seen := make(map[uuid.UUID]bool)
	total := 0

	stale, err := taskRepo.ResetStaleProcessing(ctx)
	if err != nil {
		log.Warn("recover: reset stale", "err", err)
	} else {
		for _, id := range stale {
			if seen[id] {
				continue
			}
			seen[id] = true
			if err := wp.Enqueue(ctx, id); err != nil {
				log.Warn("recover: enqueue stale", "task_id", id, "err", err)
			} else {
				log.Info("recover: requeued stale task", "task_id", id)
				total++
			}
		}
	}

	pending, err := taskRepo.ListPending(ctx)
	if err != nil {
		log.Warn("recover: list pending", "err", err)
		return total
	}
	for _, id := range pending {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := wp.Enqueue(ctx, id); err != nil {
			log.Warn("recover: enqueue pending", "task_id", id, "err", err)
		} else {
			log.Info("recover: requeued pending task", "task_id", id)
			total++
		}
	}
	return total
}

// exitCodeFor мапит доменные ошибки в коды возврата (docs/COMMANDS.md §2).
func exitCodeFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		return 2
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return 1
	default:
		return 3
	}
}

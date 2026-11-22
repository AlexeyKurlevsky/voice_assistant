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
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres/repo"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/logger"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/worker"
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

// run собирает приложение и обрабатывает graceful shutdown.
// Возвращает код выхода (0/1/2/3).
func run(cfg *config.Config, log *slog.Logger) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DBDSN, cfg.DBMaxConns)
	if err != nil {
		log.Error("postgres pool", "err", err)
		return 3
	}
	defer pool.Close()

	if cfg.MigrateOnStart {
		if err := postgres.Migrate(ctx, cfg.DBDSN); err != nil {
			log.Error("migrate", "err", err)
			return 3
		}
	}

	userRepo := repo.NewUserRepo(pool)
	meetingRepo := repo.NewMeetingRepo(pool)
	taskRepo := repo.NewTaskRepo(pool)

	wp := application.NewWorkerPool(
		cfg.WorkerConcurrency,
		cfg.JobQueueCapacity,
		worker.NewProcessor(log),
		log,
	)
	wp.Start(ctx)

	userService := application.NewUserService(userRepo)
	meetingService := application.NewMeetingService(
		meetingRepo, taskRepo, wp, cfg.UploadDir, cfg.MaxUploadBytes, log,
	)

	deps := &cli.Deps{
		UserService:    userService,
		MeetingService: meetingService,
	}
	root := cli.NewRoot(deps)

	cliErr := root.ExecuteContext(ctx)

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

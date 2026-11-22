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
	userService := application.NewUserService(userRepo)

	deps := &cli.Deps{UserService: userService}
	root := cli.NewRoot(deps)

	if err := root.ExecuteContext(ctx); err != nil {
		log.Error("cli", "err", err)
		return exitCodeFor(err)
	}
	return 0
}

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

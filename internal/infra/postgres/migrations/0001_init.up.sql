package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/AlexeyKurlevsky/voice_assistant/application"
	"github.com/AlexeyKurlevsky/voice_assistant/cli"
	"github.com/AlexeyKurlevsky/voice_assistant/config"
	"github.com/AlexeyKurlevsky/voice_assistant/domain"
	"github.com/AlexeyKurlevsky/voice_assistant/infra/postgres"
	"github.com/AlexeyKurlevsky/voice_assistant/logger"
)

func main() {
	_ = godotenv.Load()

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

	userRepo := postgres.NewUserRepo(pool)
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
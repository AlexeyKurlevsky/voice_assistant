package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/infra/postgres/sqlcgen"
)

// UserRepo — реализация application.UserRepo.
type UserRepo struct {
	q *sqlcgen.Queries
}

// NewUserRepo создаёт репозиторий поверх пула соединений.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{q: sqlcgen.New(pool)}
}

// EnsureExists — идемпотентный get-or-create по external_id.
func (r *UserRepo) EnsureExists(ctx context.Context, externalID string) (*domain.User, error) {
	row, err := r.q.EnsureUser(ctx, externalID)
	if err != nil {
		return nil, fmt.Errorf("user_repo.EnsureExists: %w", err)
	}
	return toDomainUser(row), nil
}

// GetByExternalID возвращает domain.ErrNotFound, если пользователя нет.
func (r *UserRepo) GetByExternalID(ctx context.Context, externalID string) (*domain.User, error) {
	row, err := r.q.GetUserByExternalID(ctx, externalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("user_repo.GetByExternalID: %w", err)
	}
	return toDomainUser(row), nil
}

// toDomainUser маппит sqlcgen.User в domain.User.
func toDomainUser(row sqlcgen.User) *domain.User {
	return &domain.User{
		ID:         toUUID(row.ID),
		ExternalID: row.ExternalID,
		CreatedAt:  row.CreatedAt,
	}
}

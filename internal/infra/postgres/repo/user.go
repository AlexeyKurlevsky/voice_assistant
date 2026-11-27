package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// Create создаёт пользователя. ErrAlreadyExists, если external_id занят.
func (r *UserRepo) Create(ctx context.Context, externalID, tokenHash string) (*domain.User, error) {
	row, err := r.q.CreateUser(ctx, sqlcgen.CreateUserParams{
		ExternalID: externalID,
		TokenHash:  tokenHash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrAlreadyExists
		}
		return nil, fmt.Errorf("user_repo.Create: %w", err)
	}
	return &domain.User{
		ID:         toUUID(row.ID),
		ExternalID: row.ExternalID,
		CreatedAt:  row.CreatedAt,
	}, nil
}

// GetByExternalID — ErrNotFound, если нет.
func (r *UserRepo) GetByExternalID(ctx context.Context, externalID string) (*domain.User, error) {
	row, err := r.q.GetUserByExternalID(ctx, externalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("user_repo.GetByExternalID: %w", err)
	}
	return toDomainUserFromExt(row), nil
}

// GetByTokenHash — ErrNotFound, если нет.
func (r *UserRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.User, error) {
	row, err := r.q.GetUserByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("user_repo.GetByTokenHash: %w", err)
	}
	return toDomainUserFromToken(row), nil
}

// UpdateTokenHash — перевыпуск токена.
func (r *UserRepo) UpdateTokenHash(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	if err := r.q.UpdateUserTokenHash(ctx, sqlcgen.UpdateUserTokenHashParams{
		ID:        toPgUUID(userID),
		TokenHash: tokenHash,
	}); err != nil {
		return fmt.Errorf("user_repo.UpdateTokenHash: %w", err)
	}
	return nil
}

// toDomainUserFromExt маппит строку GetUserByExternalID.
func toDomainUserFromExt(row sqlcgen.GetUserByExternalIDRow) *domain.User {
	return &domain.User{
		ID:         toUUID(row.ID),
		ExternalID: row.ExternalID,
		CreatedAt:  row.CreatedAt,
	}
}

// toDomainUserFromToken маппит строку GetUserByTokenHash.
func toDomainUserFromToken(row sqlcgen.GetUserByTokenHashRow) *domain.User {
	return &domain.User{
		ID:         toUUID(row.ID),
		ExternalID: row.ExternalID,
		CreatedAt:  row.CreatedAt,
	}
}

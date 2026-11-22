package application

import (
	"context"
	"fmt"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// UserService — сценарии пользователей. Регистрация ленивая:
// первый вызов любой команды делает EnsureExists.
type UserService struct {
	repo UserRepo
}

func NewUserService(repo UserRepo) *UserService {
	return &UserService{repo: repo}
}

// EnsureExists валидирует external_id и возвращает/создаёт пользователя.
// Идемпотентен. ErrInvalidArgument при невалидном external_id.
func (s *UserService) EnsureExists(ctx context.Context, externalID string) (*domain.User, error) {
	if err := validateExternalID(externalID); err != nil {
		return nil, err
	}
	u, err := s.repo.EnsureExists(ctx, externalID)
	if err != nil {
		return nil, fmt.Errorf("user_service.EnsureExists: %w", err)
	}
	return u, nil
}

// validateExternalID — [a-zA-Z0-9_.-]{1,64}. Правило CLI-уровня,
// поэтому здесь, а не в domain.
func validateExternalID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: external_id is empty", domain.ErrInvalidArgument)
	}
	if len(id) > 64 {
		return fmt.Errorf("%w: external_id must be ≤ 64 chars", domain.ErrInvalidArgument)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			continue
		}
		return fmt.Errorf("%w: external_id must match [a-zA-Z0-9_.-]+", domain.ErrInvalidArgument)
	}
	return nil
}

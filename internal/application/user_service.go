package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// UserService — регистрация, аутентификация по токену, перевыпуск токена.
type UserService struct {
	repo UserRepo
}

// NewUserService создаёт сервис с указанным репозиторием.
func NewUserService(repo UserRepo) *UserService {
	return &UserService{repo: repo}
}

// Register создаёт пользователя и возвращает plaintext-токен.
// Токен показывается один раз — в БД хранится только SHA-256.
// ErrAlreadyExists, если external_id занят.
func (s *UserService) Register(ctx context.Context, externalID string) (*domain.User, string, error) {
	if err := validateExternalID(externalID); err != nil {
		return nil, "", err
	}
	token, err := generateToken()
	if err != nil {
		return nil, "", fmt.Errorf("user_service.Register: token: %w", err)
	}
	u, err := s.repo.Create(ctx, externalID, hashToken(token))
	if err != nil {
		return nil, "", fmt.Errorf("user_service.Register: %w", err)
	}
	return u, token, nil
}

// Authenticate проверяет токен и возвращает пользователя.
// Пустой токен — ErrInvalidArgument; несоответствие — ErrNotFound.
func (s *UserService) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	if token == "" {
		return nil, fmt.Errorf("%w: token is empty", domain.ErrInvalidArgument)
	}
	u, err := s.repo.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("user_service.Authenticate: %w", err)
	}
	return u, nil
}

// Rotate перевыпускает токен для аутентифицированного пользователя.
// Старый токен инвалидируется. Вызывается только после успешного
// Authenticate (в CLI — через middleware).
func (s *UserService) Rotate(ctx context.Context, userID uuid.UUID) (string, error) {
	newToken, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("user_service.Rotate: token: %w", err)
	}
	if err := s.repo.UpdateTokenHash(ctx, userID, hashToken(newToken)); err != nil {
		return "", fmt.Errorf("user_service.Rotate: %w", err)
	}
	return newToken, nil
}

// validateExternalID — [a-zA-Z0-9_.-]{1,64}. Правило CLI-уровня.
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

// generateToken — 32 случайных байта в hex (64 символа, 256 бит энтропии).
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken — SHA-256 (hex). Токен high-entropy, SHA-256 достаточно.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

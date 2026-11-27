package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// allowedExt — допустимые расширения. Аудио для реального Speech,
// .txt/.json — для mock (транскрипция берётся из содержимого).
var allowedExt = map[string]bool{
	".mp3": true, ".wav": true, ".m4a": true,
	".ogg": true, ".flac": true,
	".txt": true, ".json": true,
}

// MeetingService — загрузка и список разговоров.
// Обработки здесь нет. Она живет в worker/processor.
type MeetingService struct {
	meetingRepo MeetingRepo
	taskRepo    TaskRepo
	enqueuer    Enqueuer
	uploadDir   string
	maxBytes    int64
	log         *slog.Logger
}

func NewMeetingService(
	meetingRepo MeetingRepo,
	taskRepo TaskRepo,
	enqueuer Enqueuer,
	uploadDir string,
	maxBytes int64,
	log *slog.Logger,
) *MeetingService {
	return &MeetingService{
		meetingRepo: meetingRepo,
		taskRepo:    taskRepo,
		enqueuer:    enqueuer,
		uploadDir:   uploadDir,
		maxBytes:    maxBytes,
		log:         log,
	}
}

// Load: валидация → SHA-256 → дедупликация → копия → TX meeting+task → Enqueue.
//
// Возвращает (meeting, task, alreadyLoaded, error): alreadyLoaded=true, если
// файл с таким хэшем у пользователя уже есть — тогда новая встреча/задача
// не создаются, а возвращается существующая.
//
// При ErrQueueFull задача помечается failed (с отдельным контекстом,
// чтобы пережить уже отменённый ctx), а скопированный файл удаляется —
// задача не попадёт в обработку. При успешном запуске копию удаляет воркер
// после сохранения транскрипции в БД (см. worker/processor.go).
func (s *MeetingService) Load(ctx context.Context, userID uuid.UUID, srcPath string) (*domain.Meeting, *domain.Task, bool, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, false, fmt.Errorf("%w: file not found: %s", domain.ErrInvalidArgument, srcPath)
		}
		return nil, nil, false, fmt.Errorf("meeting_service.Load: stat: %w", err)
	}
	if info.IsDir() {
		return nil, nil, false, fmt.Errorf("%w: path is a directory: %s", domain.ErrInvalidArgument, srcPath)
	}
	if info.Size() > s.maxBytes {
		return nil, nil, false, fmt.Errorf("%w: file too large: %d > %d bytes",
			domain.ErrInvalidArgument, info.Size(), s.maxBytes)
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	if !allowedExt[ext] {
		return nil, nil, false, fmt.Errorf("%w: unsupported file type: %s", domain.ErrInvalidArgument, ext)
	}

	hash, err := fileSHA256(srcPath)
	if err != nil {
		return nil, nil, false, fmt.Errorf("meeting_service.Load: hash: %w", err)
	}

	// Дедупликация: этот файл у пользователя уже загружен.
	if existing, err := s.meetingRepo.GetByContentHashForUser(ctx, userID, hash); err == nil {
		task, terr := s.taskRepo.GetByMeetingIDForUser(ctx, existing.ID, userID)
		if terr != nil {
			return nil, nil, false, fmt.Errorf("meeting_service.Load: existing task: %w", terr)
		}
		s.log.Info("meeting_service.Load: duplicate file",
			"meeting_id", existing.ID, "content_hash", hash)
		return existing, task, true, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, nil, false, fmt.Errorf("meeting_service.Load: hash lookup: %w", err)
	}

	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, nil, false, fmt.Errorf("meeting_service.Load: mkdir upload: %w", err)
	}

	// Имя в uploadDir — UUID: нет коллизий между пользователями,
	// нет спецсимволов из имени исходного файла.
	relPath := uuid.NewString() + ext
	dstPath := filepath.Join(s.uploadDir, relPath)
	if err := copyFile(srcPath, dstPath); err != nil {
		return nil, nil, false, fmt.Errorf("meeting_service.Load: copy: %w", err)
	}

	title := strings.TrimSuffix(filepath.Base(srcPath), ext)
	m := &domain.Meeting{
		UserID:           userID,
		Title:            title,
		OriginalFilename: filepath.Base(srcPath),
		FilePath:         relPath,
		ContentHash:      hash,
	}
	task, err := s.meetingRepo.CreateWithTask(ctx, m)
	if err != nil {
		_ = os.Remove(dstPath) // записи в БД нет — файл «осиротел»
		return nil, nil, false, fmt.Errorf("meeting_service.Load: create: %w", err)
	}

	if err := s.enqueuer.Enqueue(ctx, task.ID); err != nil {
		_ = os.Remove(dstPath) // задача не попадёт в обработку — копия не нужна
		// Отдельный контекст: если ctx уже отменён (shutdown),
		// MarkFailed всё равно должен пройти.
		failCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if ferr := s.taskRepo.MarkFailed(failCtx, task.ID, "queue full"); ferr != nil {
			s.log.Warn("meeting_service.Load: mark failed after enqueue error",
				"task_id", task.ID, "err", ferr)
		}
		return nil, nil, false, fmt.Errorf("meeting_service.Load: enqueue: %w", err)
	}

	return m, task, false, nil
}

// List с нормализацией limit (≤0 или >500 → 50) и offset (<0 → 0).
// Репозиторий получает уже валидные значения.
func (s *MeetingService) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.MeetingListItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.meetingRepo.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("meeting_service.List: %w", err)
	}
	return items, nil
}

// copyFile — потоковое копирование + fsync, чтобы файл был на диске
// к моменту возврата из Load.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// fileSHA256 возвращает SHA-256 содержимого файла в hex (64 символа).
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Status возвращает задачу обработки встречи с проверкой прав.
// ErrNotFound, если встреча чужая или не существует.
func (s *MeetingService) Status(ctx context.Context, meetingID, userID uuid.UUID) (*domain.Task, error) {
	return s.taskRepo.GetByMeetingIDForUser(ctx, meetingID, userID)
}

// StatusHistory возвращает историю переходов задачи, по возрастанию времени.
func (s *MeetingService) StatusHistory(ctx context.Context, taskID uuid.UUID) ([]domain.StatusChange, error) {
	return s.taskRepo.ListStatusHistory(ctx, taskID)
}

// Transcript возвращает транскрипцию встречи с проверкой прав.
// ErrNotFound, если встреча чужая или транскрипции ещё нет.
func (s *MeetingService) Transcript(ctx context.Context, meetingID, userID uuid.UUID) (string, error) {
	return s.meetingRepo.GetTranscript(ctx, meetingID, userID)
}

// Search выполняет полнотекстовый поиск по встречам пользователя.
// Пустой запрос — ErrInvalidArgument.
func (s *MeetingService) Search(ctx context.Context, userID uuid.UUID, query string) ([]domain.SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: keyword is required", domain.ErrInvalidArgument)
	}
	hits, err := s.meetingRepo.Search(ctx, userID, query)
	if err != nil {
		return nil, fmt.Errorf("meeting_service.Search: %w", err)
	}
	return hits, nil
}

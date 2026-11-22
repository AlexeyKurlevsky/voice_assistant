package application

import (
	"context"
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

// Load: валидация → копия файла → TX создания meeting+task (T1) → Enqueue.
//
// При ErrQueueFull задача помечается failed (с отдельным контекстом,
// чтобы пережить уже отменённый ctx). Скопированный файл не удаляется —
// для аудита.
func (s *MeetingService) Load(ctx context.Context, userID uuid.UUID, srcPath string) (*domain.Meeting, *domain.Task, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("%w: file not found: %s", domain.ErrInvalidArgument, srcPath)
		}
		return nil, nil, fmt.Errorf("meeting_service.Load: stat: %w", err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("%w: path is a directory: %s", domain.ErrInvalidArgument, srcPath)
	}
	if info.Size() > s.maxBytes {
		return nil, nil, fmt.Errorf("%w: file too large: %d > %d bytes",
			domain.ErrInvalidArgument, info.Size(), s.maxBytes)
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	if !allowedExt[ext] {
		return nil, nil, fmt.Errorf("%w: unsupported file type: %s", domain.ErrInvalidArgument, ext)
	}

	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("meeting_service.Load: mkdir upload: %w", err)
	}

	// Имя в uploadDir — UUID: нет коллизий между пользователями,
	// нет спецсимволов из имени исходного файла.
	relPath := uuid.NewString() + ext
	dstPath := filepath.Join(s.uploadDir, relPath)
	if err := copyFile(srcPath, dstPath); err != nil {
		return nil, nil, fmt.Errorf("meeting_service.Load: copy: %w", err)
	}

	title := strings.TrimSuffix(filepath.Base(srcPath), ext)
	m := &domain.Meeting{
		UserID:           userID,
		Title:            title,
		OriginalFilename: filepath.Base(srcPath),
		FilePath:         relPath,
	}
	task, err := s.meetingRepo.CreateWithTask(ctx, m)
	if err != nil {
		_ = os.Remove(dstPath) // записи в БД нет — файл «осиротел»
		return nil, nil, fmt.Errorf("meeting_service.Load: create: %w", err)
	}

	if err := s.enqueuer.Enqueue(ctx, task.ID); err != nil {
		// Отдельный контекст: если ctx уже отменён (shutdown),
		// MarkFailed всё равно должен пройти.
		failCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if ferr := s.taskRepo.MarkFailed(failCtx, task.ID, "queue full"); ferr != nil {
			s.log.Warn("meeting_service.Load: mark failed after enqueue error",
				"task_id", task.ID, "err", ferr)
		}
		return nil, nil, fmt.Errorf("meeting_service.Load: enqueue: %w", err)
	}

	return m, task, nil
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

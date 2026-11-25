package application

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// Порты: контракт между application и infra. Правила:
//   - объявлены здесь, реализации — в infra (инверсия зависимостей);
//   - возвращают только доменные типы и ошибки (не pgx.Row, не HTTP);
//   - context.Context — первым аргументом;
//   - инфраструктурные ошибки маппятся в доменные на границе реализации.

// UserRepo — хранилище пользователей.
type UserRepo interface {
	// Create создаёт пользователя. ErrAlreadyExists, если external_id занят.
	Create(ctx context.Context, externalID, tokenHash string) (*domain.User, error)

	// GetByExternalID — ErrNotFound, если нет.
	GetByExternalID(ctx context.Context, externalID string) (*domain.User, error)

	// GetByTokenHash — ErrNotFound, если нет.
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.User, error)

	// UpdateTokenHash — перевыпуск токена.
	UpdateTokenHash(ctx context.Context, userID uuid.UUID, tokenHash string) error
}

// MeetingRepo — встречи + связанные транскрипция/выжимка.
// Все read-методы с userID фильтруют по m.user_id = $1 в SQL
// (docs/DATA_MODEL.md §5).
type MeetingRepo interface {
	// CreateWithTask — транзакция 1: встреча + задача + первая запись history.
	// Побочно заполняет m.ID и m.CreatedAt.
	CreateWithTask(ctx context.Context, m *domain.Meeting) (*domain.Task, error)

	// ListByUser — встречи пользователя, created_at DESC.
	// limit/offset уже нормализованы в MeetingService.
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.MeetingListItem, error)

	// GetByIDForUser — встреча с проверкой прав (ErrNotFound и для
	// «нет», и для «чужая»).
	GetByIDForUser(ctx context.Context, meetingID, userID uuid.UUID) (*domain.Meeting, error)

	// GetByTaskID — для фонового processor'а; без проверки прав.
	GetByTaskID(ctx context.Context, taskID uuid.UUID) (*domain.Meeting, error)

	// GetTranscript / GetSummary — с проверкой прав; ErrNotFound, если
	// встреча чужая или данных ещё нет.
	GetTranscript(ctx context.Context, meetingID, userID uuid.UUID) (string, error)
	GetSummary(ctx context.Context, meetingID, userID uuid.UUID) (string, error)

	// Search — полнотекстовый поиск по транскрипциям и выжимкам,
	// только по встречам указанного пользователя.
	Search(ctx context.Context, userID uuid.UUID, query string) ([]domain.SearchHit, error)
}

// TaskRepo — задачи обработки. Все переходы атомарны (транзакция 2–транзакция 6),
// включают запись в status_history.
type TaskRepo interface {
	MarkProcessing(ctx context.Context, taskID uuid.UUID) error                                   // транзакция 2
	SaveTranscriptionAndMarkTranscribed(ctx context.Context, taskID uuid.UUID, text string) error // транзакция 3
	SaveSummaryAndMarkSummarized(ctx context.Context, taskID uuid.UUID, text string) error        // транзакция 4
	MarkCompleted(ctx context.Context, taskID uuid.UUID) error                                    // транзакция 5
	MarkFailed(ctx context.Context, taskID uuid.UUID, errMsg string) error                        // транзакция 6

	GetByID(ctx context.Context, taskID uuid.UUID) (*domain.Task, error)
	GetByMeetingIDForUser(ctx context.Context, meetingID, userID uuid.UUID) (*domain.Task, error)
	ListStatusHistory(ctx context.Context, taskID uuid.UUID) ([]domain.StatusChange, error)

	// ResetStaleProcessing — на старте: processing → created, без записи
	// в history (это recovery, не бизнес-переход).
	ResetStaleProcessing(ctx context.Context) ([]uuid.UUID, error)

	ListPending(ctx context.Context) ([]uuid.UUID, error)
}

// Enqueuer — постановка задачи в очередь. Не блокируется:
// при переполнении возвращает domain.ErrQueueFull.
type Enqueuer interface {
	Enqueue(ctx context.Context, taskID uuid.UUID) error
}

// SpeechClient — абстракция распознавания речи.
// Отмена ctx должна прерывать вызов (context.Canceled).
type SpeechClient interface {
	Transcribe(ctx context.Context, filename string, r io.Reader) (string, error)
}

// LLMClient — абстракция LLM. Summarize — раз на встречу,
// Answer — на каждый chat.
type LLMClient interface {
	Summarize(ctx context.Context, transcript string) (string, error)
	Answer(ctx context.Context, question, context string) (string, error)
}

// ChatRepo — хранилище пар «вопрос-ответ».
type ChatRepo interface {
	// Save сохраняет пару и заполняет qa.ID, qa.CreatedAt.
	Save(ctx context.Context, qa *domain.QA) error

	// History — последние N пар по встрече, по убыванию времени.
	History(ctx context.Context, userID, meetingID uuid.UUID, limit int) ([]domain.QA, error)
}

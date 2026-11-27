package domain

import (
	"time"

	"github.com/google/uuid"
)

// Meeting — загруженная встреча.
// FilePath — путь относительно MS_UPLOAD_DIR (абсолютный не хранится:
// перенос каталога не ломает БД).
type Meeting struct {
	ID               uuid.UUID
	UserID           uuid.UUID // владелец; фильтрация в SQL, не в сервисах
	Title            string
	OriginalFilename string
	FilePath         string
	ContentHash      string // SHA-256 содержимого файла (дедупликация)
	CreatedAt        time.Time
}

// MeetingListItem — DTO для команды list.
// Отдельный тип, а не *Meeting: не тянет FilePath/OriginalFilename
// и фиксирует контракт list независимо от изменений Meeting.
type MeetingListItem struct {
	ID        uuid.UUID
	Title     string
	Status    Status
	Summary   string // "" если выжимки ещё нет
	CreatedAt time.Time
}

// SearchHit — строка для команды find.
type SearchHit struct {
	ID        uuid.UUID
	Title     string
	Status    Status
	Snippet   string
	CreatedAt time.Time
}

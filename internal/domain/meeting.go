package domain

import "time"

type Meeting struct {
	ID               int64
	UserID           int64
	Title            string
	OriginalFilename string
	FilePath         string
	CreatedAt        time.Time
}

// MeetingListItem — строка для команды list.
type MeetingListItem struct {
	ID        int64
	Title     string
	Status    Status
	Summary   string // "" если выжимки ещё нет
	CreatedAt time.Time
}

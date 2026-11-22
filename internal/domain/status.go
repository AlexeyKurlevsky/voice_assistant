package domain

type Status string

const (
	StatusCreated     Status = "created"
	StatusProcessing  Status = "processing"
	StatusTranscribed Status = "transcribed"
	StatusSummarized  Status = "summarized"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusCreated, StatusProcessing, StatusTranscribed,
		StatusSummarized, StatusCompleted, StatusFailed:
		return true
	}
	return false
}

func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

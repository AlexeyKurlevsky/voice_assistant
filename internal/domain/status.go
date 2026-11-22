package domain

// Status — статус задачи обработки.
//
//	created → processing → transcribed → summarized → completed
//	                 └─────────┴──────────────┴──→ failed
//
// summarized и completed разделены намеренно: summarized = LLM ответил
// и выжимка записана; completed = все материалы консистентны. При сбое
// между ними задачу можно добить без повторного вызова LLM.
// Подробнее — docs/STATUS_MODEL.md.
type Status string

const (
	StatusCreated     Status = "created"     // встреча и задача созданы (T1)
	StatusProcessing  Status = "processing"  // воркер взял задачу
	StatusTranscribed Status = "transcribed" // транскрипция сохранена (T3)
	StatusSummarized  Status = "summarized"  // выжимка сохранена (T4)
	StatusCompleted   Status = "completed"   // терминальный: всё готово (T5)
	StatusFailed      Status = "failed"      // терминальный: ошибка (T6)
)

// IsValid проверяет принадлежность множеству допустимых значений
// (совпадает с CHECK-констрейнтом processing_tasks.status).
func (s Status) IsValid() bool {
	switch s {
	case StatusCreated, StatusProcessing, StatusTranscribed,
		StatusSummarized, StatusCompleted, StatusFailed:
		return true
	}
	return false
}

// IsTerminal сообщает, что переходов из статуса больше нет.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

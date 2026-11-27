package domain

import "errors"

// Доменные ошибки для проверок через errors.Is в application и cli.
// Инфраструктурные ошибки (pgx.ErrNoRows, context.Canceled) маппятся
// в эти на границе: в repo/*.go и в фабриках клиентов.
var (
	// ErrNotFound — не найдено или недоступно пользователю.
	// Чужие встречи тоже дают ErrNotFound, а не ErrForbidden — чтобы
	// не подтверждать существование (docs/DECISIONS.md ADR-7).
	ErrNotFound = errors.New("not found")

	// ErrForbidden — доступ запрещён. Зарезервировано, пока не используется.
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidArgument — некорректный ввод (CLI: exit 2).
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrQueueFull — очередь обработки переполнена или пул остановлен.
	// Штатный backpressure (CLI: exit 3).
	ErrQueueFull = errors.New("processing queue is full, try again later")

	// ErrAlreadyExists — дубликат. Зарезервировано, пока не используется.
	ErrAlreadyExists = errors.New("already exists")
)

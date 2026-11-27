package application

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// TaskProcessor — обработчик одной задачи.
//
// Возвращённая ошибка идёт только в лог: MarkFailed вызывает сам
// processor (только он знает контекст ошибки).
type TaskProcessor func(ctx context.Context, taskID uuid.UUID) error

// WorkerPool — bounded pool: n воркеров, очередь capacity.
//
// Enqueue неблокирующий (ErrQueueFull при переполнении). Stop идемпотентен.
// Подробнее — docs/ARCHITECTURE.md §6.
type WorkerPool struct {
	n       int
	jobs    chan uuid.UUID
	process TaskProcessor
	log     *slog.Logger

	wg       sync.WaitGroup
	cancel   context.CancelFunc
	stopOnce sync.Once
	stopped  atomic.Bool
}

// NewWorkerPool: n — параллелизм (config.WorkerConcurrency),
// capacity — ёмкость очереди (config.JobQueueCapacity).
func NewWorkerPool(n, capacity int, process TaskProcessor, log *slog.Logger) *WorkerPool {
	return &WorkerPool{
		n:       n,
		jobs:    make(chan uuid.UUID, capacity),
		process: process,
		log:     log,
	}
}

// Start запускает n воркеров с контекстом, производным от parent.
// Отмена parent (например, signal.NotifyContext) останавливает их.
func (p *WorkerPool) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	for i := 0; i < p.n; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Enqueue неблокирующий. Возвращает ctx.Err(), если ctx отменён,
// или domain.ErrQueueFull, если пул остановлен / канал полон.
func (p *WorkerPool) Enqueue(ctx context.Context, taskID uuid.UUID) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if p.stopped.Load() {
		return domain.ErrQueueFull
	}
	select {
	case p.jobs <- taskID:
		return nil
	default:
		return domain.ErrQueueFull
	}
}

// Stop: stopped=true → close(jobs) → ждём завершения воркеров в пределах ctx.
// Контекст воркеров отменяется только после того, как все они завершились
// (нормальный сценарий) или истёк ctx (принудительный shutdown).
// Идемпотентен.
func (p *WorkerPool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		close(p.jobs)
	})

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Воркеры доработали сами — отменяем ctx только для очистки ресурсов.
		if p.cancel != nil {
			p.cancel()
		}
		return nil
	case <-ctx.Done():
		// Таймаут: отменяем ctx, чтобы воркеры прервали текущие операции.
		if p.cancel != nil {
			p.cancel()
		}
		return fmt.Errorf("worker_pool.Stop: %w", ctx.Err())
	}
}

// worker — цикл одной горутины. Ошибки process логируются и не
// прерывают воркер: одна плохая задача не блокирует остальные.
func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case taskID, ok := <-p.jobs:
			if !ok {
				return
			}
			if err := p.process(ctx, taskID); err != nil {
				p.log.Error("worker: process task", "task_id", taskID, "err", err)
			}
		}
	}
}

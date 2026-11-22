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

type TaskProcessor func(ctx context.Context, taskID uuid.UUID) error

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

func NewWorkerPool(n, capacity int, process TaskProcessor, log *slog.Logger) *WorkerPool {
	return &WorkerPool{
		n:       n,
		jobs:    make(chan uuid.UUID, capacity),
		process: process,
		log:     log,
	}
}

func (p *WorkerPool) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	for i := 0; i < p.n; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

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

func (p *WorkerPool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		if p.cancel != nil {
			p.cancel()
		}
		close(p.jobs)
	})

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker_pool.Stop: %w", ctx.Err())
	}
}

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

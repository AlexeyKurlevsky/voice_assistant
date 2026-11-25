package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

func noopProcess(_ context.Context, _ uuid.UUID) error { return nil }

func TestWorkerPool_Enqueue_Success(t *testing.T) {
	pool := NewWorkerPool(1, 10, noopProcess, discardLogger())
	ctx := context.Background()
	pool.Start(ctx)

	if err := pool.Enqueue(ctx, uuid.New()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := pool.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestWorkerPool_Enqueue_QueueFull(t *testing.T) {
	pool := NewWorkerPool(1, 0, noopProcess, discardLogger())
	ctx := context.Background()

	if err := pool.Enqueue(ctx, uuid.New()); !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
}

func TestWorkerPool_Enqueue_AfterStop(t *testing.T) {
	pool := NewWorkerPool(1, 1, noopProcess, discardLogger())
	ctx := context.Background()
	pool.Start(ctx)
	if err := pool.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := pool.Enqueue(ctx, uuid.New()); !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
}

func TestWorkerPool_Stop_Idempotent(t *testing.T) {
	pool := NewWorkerPool(1, 1, noopProcess, discardLogger())
	ctx := context.Background()
	pool.Start(ctx)

	if err := pool.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := pool.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestWorkerPool_LimitConcurrency(t *testing.T) {
	const limit = 3
	var inFlight, maxSeen atomic.Int32

	process := func(_ context.Context, _ uuid.UUID) error {
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return nil
	}

	pool := NewWorkerPool(limit, 100, process, discardLogger())
	ctx := context.Background()
	pool.Start(ctx)

	for i := 0; i < 10; i++ {
		if err := pool.Enqueue(ctx, uuid.New()); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	if err := pool.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if maxSeen.Load() > limit {
		t.Errorf("max concurrency = %d, want <= %d", maxSeen.Load(), limit)
	}
	if maxSeen.Load() <= 1 {
		t.Errorf("expected parallel execution, max = %d", maxSeen.Load())
	}
}

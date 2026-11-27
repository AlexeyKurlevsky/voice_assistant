package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func writeBenchFile(b *testing.B, size int64) string {
	b.Helper()
	path := filepath.Join(b.TempDir(), "meeting.txt")
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		b.Fatal(err)
	}
	return path
}

func BenchmarkFileSHA256(b *testing.B) {
	path := writeBenchFile(b, 1<<20) // 1 MiB
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := fileSHA256(path); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMeetingService_Load(b *testing.B) {
	svc := NewMeetingService(
		&fakeMeetingRepo{}, &fakeTaskRepo{}, &fakeEnqueuer{},
		b.TempDir(), 100<<20, discardLogger(),
	)
	path := writeBenchFile(b, 1<<20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := svc.Load(context.Background(), uuid.New(), path); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWorkerPool_EnqueueProcess(b *testing.B) {
	pool := NewWorkerPool(4, 4096, noopProcess, discardLogger())
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Stop(ctx)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pool.Enqueue(ctx, uuid.New()); err != nil {
			b.Fatal(err)
		}
	}
}

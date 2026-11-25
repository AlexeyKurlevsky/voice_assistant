package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type fakeMeetingRepo struct {
	createErr  error
	created    *domain.Meeting
	list       []domain.MeetingListItem
	listLimit  int
	listOffset int
	byID       *domain.Meeting
	byTaskID   *domain.Meeting
	transcript string
	summary    string
	search     []domain.SearchHit
}

func (f *fakeMeetingRepo) CreateWithTask(_ context.Context, m *domain.Meeting) (*domain.Task, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	m.ID = uuid.New()
	m.CreatedAt = time.Now()
	f.created = m
	return &domain.Task{ID: uuid.New(), MeetingID: m.ID, Status: domain.StatusCreated}, nil
}

func (f *fakeMeetingRepo) ListByUser(_ context.Context, _ uuid.UUID, limit, offset int) ([]domain.MeetingListItem, error) {
	f.listLimit = limit
	f.listOffset = offset
	return f.list, nil
}

func (f *fakeMeetingRepo) GetByIDForUser(_ context.Context, _, _ uuid.UUID) (*domain.Meeting, error) {
	if f.byID == nil {
		return nil, domain.ErrNotFound
	}
	return f.byID, nil
}

func (f *fakeMeetingRepo) GetByTaskID(_ context.Context, _ uuid.UUID) (*domain.Meeting, error) {
	if f.byTaskID == nil {
		return nil, domain.ErrNotFound
	}
	return f.byTaskID, nil
}

func (f *fakeMeetingRepo) GetTranscript(_ context.Context, _, _ uuid.UUID) (string, error) {
	if f.transcript == "" {
		return "", domain.ErrNotFound
	}
	return f.transcript, nil
}

func (f *fakeMeetingRepo) GetSummary(_ context.Context, _, _ uuid.UUID) (string, error) {
	if f.summary == "" {
		return "", domain.ErrNotFound
	}
	return f.summary, nil
}

func (f *fakeMeetingRepo) Search(_ context.Context, _ uuid.UUID, _ string) ([]domain.SearchHit, error) {
	return f.search, nil
}

type fakeTaskRepo struct {
	task             *domain.Task
	markFailedCalled bool
	markFailedMsg    string
}

func (f *fakeTaskRepo) MarkProcessing(_ context.Context, _ uuid.UUID) error { return nil }
func (f *fakeTaskRepo) SaveTranscriptionAndMarkTranscribed(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeTaskRepo) SaveSummaryAndMarkSummarized(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (f *fakeTaskRepo) MarkCompleted(_ context.Context, _ uuid.UUID) error { return nil }
func (f *fakeTaskRepo) MarkFailed(_ context.Context, _ uuid.UUID, msg string) error {
	f.markFailedCalled = true
	f.markFailedMsg = msg
	return nil
}
func (f *fakeTaskRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Task, error) {
	if f.task == nil {
		return nil, domain.ErrNotFound
	}
	return f.task, nil
}
func (f *fakeTaskRepo) GetByMeetingIDForUser(_ context.Context, _, _ uuid.UUID) (*domain.Task, error) {
	if f.task == nil {
		return nil, domain.ErrNotFound
	}
	return f.task, nil
}
func (f *fakeTaskRepo) ListStatusHistory(_ context.Context, _ uuid.UUID) ([]domain.StatusChange, error) {
	return nil, nil
}
func (f *fakeTaskRepo) ResetStaleProcessing(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}
func (f *fakeTaskRepo) ListPending(_ context.Context) ([]uuid.UUID, error) { return nil, nil }

type fakeEnqueuer struct {
	err      error
	enqueued uuid.UUID
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.enqueued = id
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestMeetingService_Load_Success(t *testing.T) {
	meetingRepo := &fakeMeetingRepo{}
	taskRepo := &fakeTaskRepo{}
	enq := &fakeEnqueuer{}
	svc := NewMeetingService(meetingRepo, taskRepo, enq, t.TempDir(), 1024*1024, discardLogger())

	path := writeTempFile(t, "meeting.txt", "текст встречи")
	m, task, err := svc.Load(context.Background(), uuid.New(), path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.ID == uuid.Nil {
		t.Error("meeting ID must be set")
	}
	if task.Status != domain.StatusCreated {
		t.Errorf("task status = %q", task.Status)
	}
	if enq.enqueued != task.ID {
		t.Errorf("enqueued = %s, want %s", enq.enqueued, task.ID)
	}
	if _, err := os.Stat(filepath.Join(svc.uploadDir, m.FilePath)); err != nil {
		t.Errorf("uploaded copy not found: %v", err)
	}
}

func TestMeetingService_Load_FileNotFound(t *testing.T) {
	svc := NewMeetingService(&fakeMeetingRepo{}, &fakeTaskRepo{}, &fakeEnqueuer{}, t.TempDir(), 1024, discardLogger())
	_, _, err := svc.Load(context.Background(), uuid.New(), filepath.Join(t.TempDir(), "nope.txt"))
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestMeetingService_Load_UnsupportedExt(t *testing.T) {
	svc := NewMeetingService(&fakeMeetingRepo{}, &fakeTaskRepo{}, &fakeEnqueuer{}, t.TempDir(), 1024, discardLogger())
	path := writeTempFile(t, "doc.pdf", "x")
	_, _, err := svc.Load(context.Background(), uuid.New(), path)
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestMeetingService_Load_QueueFull(t *testing.T) {
	meetingRepo := &fakeMeetingRepo{}
	taskRepo := &fakeTaskRepo{}
	svc := NewMeetingService(meetingRepo, taskRepo,
		&fakeEnqueuer{err: domain.ErrQueueFull}, t.TempDir(), 1024, discardLogger())

	path := writeTempFile(t, "meeting.txt", "x")
	_, _, err := svc.Load(context.Background(), uuid.New(), path)
	if !errors.Is(err, domain.ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
	if !taskRepo.markFailedCalled {
		t.Error("task must be marked failed on queue overflow")
	}
}

func TestMeetingService_List_NormalizesPagination(t *testing.T) {
	repo := &fakeMeetingRepo{}
	svc := NewMeetingService(repo, &fakeTaskRepo{}, &fakeEnqueuer{}, t.TempDir(), 1024, discardLogger())

	if _, err := svc.List(context.Background(), uuid.New(), 0, -1); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.listLimit != 50 || repo.listOffset != 0 {
		t.Errorf("limit/offset = %d/%d, want 50/0", repo.listLimit, repo.listOffset)
	}

	if _, err := svc.List(context.Background(), uuid.New(), 1000, 5); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.listLimit != 50 || repo.listOffset != 5 {
		t.Errorf("limit/offset = %d/%d, want 50/5", repo.listLimit, repo.listOffset)
	}
}

func TestMeetingService_Search_EmptyQuery(t *testing.T) {
	svc := NewMeetingService(&fakeMeetingRepo{}, &fakeTaskRepo{}, &fakeEnqueuer{}, t.TempDir(), 1024, discardLogger())
	_, err := svc.Search(context.Background(), uuid.New(), "   ")
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestMeetingService_Transcript(t *testing.T) {
	repo := &fakeMeetingRepo{transcript: "полный текст"}
	svc := NewMeetingService(repo, &fakeTaskRepo{}, &fakeEnqueuer{}, t.TempDir(), 1024, discardLogger())
	got, err := svc.Transcript(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	if got != "полный текст" {
		t.Errorf("got %q", got)
	}
}

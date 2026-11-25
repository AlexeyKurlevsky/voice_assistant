package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeTaskRepo struct {
	task               *domain.Task
	processingCalled   bool
	transcribedSaved   string
	summarizedSaved    string
	completedCalled    bool
	failedCalled       bool
	failedMsg          string
	markProcessingErr  error
	markFailedErr      error
	markTranscribedErr error
	markSummarizedErr  error
	markCompletedErr   error
}

func (f *fakeTaskRepo) MarkProcessing(_ context.Context, _ uuid.UUID) error {
	f.processingCalled = true
	if f.markProcessingErr != nil {
		return f.markProcessingErr
	}
	f.task.Status = domain.StatusProcessing
	return nil
}

func (f *fakeTaskRepo) SaveTranscriptionAndMarkTranscribed(_ context.Context, _ uuid.UUID, text string) error {
	if f.markTranscribedErr != nil {
		return f.markTranscribedErr
	}
	f.transcribedSaved = text
	f.task.Status = domain.StatusTranscribed
	return nil
}

func (f *fakeTaskRepo) SaveSummaryAndMarkSummarized(_ context.Context, _ uuid.UUID, text string) error {
	if f.markSummarizedErr != nil {
		return f.markSummarizedErr
	}
	f.summarizedSaved = text
	f.task.Status = domain.StatusSummarized
	return nil
}

func (f *fakeTaskRepo) MarkCompleted(_ context.Context, _ uuid.UUID) error {
	if f.markCompletedErr != nil {
		return f.markCompletedErr
	}
	f.completedCalled = true
	f.task.Status = domain.StatusCompleted
	return nil
}

func (f *fakeTaskRepo) MarkFailed(_ context.Context, _ uuid.UUID, msg string) error {
	if f.markFailedErr != nil {
		return f.markFailedErr
	}
	f.failedCalled = true
	f.failedMsg = msg
	f.task.Status = domain.StatusFailed
	return nil
}

func (f *fakeTaskRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Task, error) {
	if f.task == nil {
		return nil, domain.ErrNotFound
	}
	return f.task, nil
}

func (f *fakeTaskRepo) GetByMeetingIDForUser(_ context.Context, _, _ uuid.UUID) (*domain.Task, error) {
	return f.GetByID(context.Background(), uuid.Nil)
}

func (f *fakeTaskRepo) ListStatusHistory(_ context.Context, _ uuid.UUID) ([]domain.StatusChange, error) {
	return nil, nil
}

func (f *fakeTaskRepo) ResetStaleProcessing(_ context.Context) ([]uuid.UUID, error) {
	return nil, nil
}

func (f *fakeTaskRepo) ListPending(_ context.Context) ([]uuid.UUID, error) { return nil, nil }

type fakeMeetingRepo struct {
	meeting    *domain.Meeting
	transcript string
}

func (f *fakeMeetingRepo) CreateWithTask(_ context.Context, _ *domain.Meeting) (*domain.Task, error) {
	return nil, nil
}
func (f *fakeMeetingRepo) ListByUser(_ context.Context, _ uuid.UUID, _, _ int) ([]domain.MeetingListItem, error) {
	return nil, nil
}
func (f *fakeMeetingRepo) GetByIDForUser(_ context.Context, _, _ uuid.UUID) (*domain.Meeting, error) {
	return f.meeting, nil
}
func (f *fakeMeetingRepo) GetByTaskID(_ context.Context, _ uuid.UUID) (*domain.Meeting, error) {
	if f.meeting == nil {
		return nil, domain.ErrNotFound
	}
	return f.meeting, nil
}
func (f *fakeMeetingRepo) GetByContentHashForUser(_ context.Context, _ uuid.UUID, _ string) (*domain.Meeting, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeMeetingRepo) GetTranscript(_ context.Context, _, _ uuid.UUID) (string, error) {
	if f.transcript == "" {
		return "", domain.ErrNotFound
	}
	return f.transcript, nil
}
func (f *fakeMeetingRepo) GetSummary(_ context.Context, _, _ uuid.UUID) (string, error) {
	return "", domain.ErrNotFound
}
func (f *fakeMeetingRepo) Search(_ context.Context, _ uuid.UUID, _ string) ([]domain.SearchHit, error) {
	return nil, nil
}

type fakeSpeech struct {
	text   string
	err    error
	called int
}

func (f *fakeSpeech) Transcribe(_ context.Context, _ string, _ io.Reader) (string, error) {
	f.called++
	return f.text, f.err
}

type fakeLLM struct {
	summary string
	err     error
	called  int
}

func (f *fakeLLM) Summarize(_ context.Context, _ string) (string, error) {
	f.called++
	return f.summary, f.err
}

func (f *fakeLLM) Answer(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func setupProcessor(t *testing.T, taskRepo *fakeTaskRepo, meetingRepo *fakeMeetingRepo, speech application.SpeechClient, llm application.LLMClient) (application.TaskProcessor, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "meeting.txt"), []byte("audio stub"), 0o600); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	proc := NewProcessor(taskRepo, meetingRepo, speech, llm, Config{
		UploadDir:     dir,
		SpeechTimeout: time.Second,
		LLMTimeout:    time.Second,
	}, discardLogger())
	return proc, dir
}

func TestProcessor_HappyPath(t *testing.T) {
	taskID := uuid.New()
	taskRepo := &fakeTaskRepo{task: &domain.Task{ID: taskID, Status: domain.StatusCreated}}
	meetingRepo := &fakeMeetingRepo{
		meeting: &domain.Meeting{ID: uuid.New(), UserID: uuid.New(), FilePath: "meeting.txt", OriginalFilename: "meeting.txt"},
	}
	speech := &fakeSpeech{text: "транскрипция"}
	llm := &fakeLLM{summary: "выжимка"}

	proc, _ := setupProcessor(t, taskRepo, meetingRepo, speech, llm)
	if err := proc(context.Background(), taskID); err != nil {
		t.Fatalf("process: %v", err)
	}

	if !taskRepo.processingCalled {
		t.Error("MarkProcessing must be called")
	}
	if taskRepo.transcribedSaved != "транскрипция" {
		t.Errorf("transcribedSaved = %q", taskRepo.transcribedSaved)
	}
	if taskRepo.summarizedSaved != "выжимка" {
		t.Errorf("summarizedSaved = %q", taskRepo.summarizedSaved)
	}
	if !taskRepo.completedCalled {
		t.Error("MarkCompleted must be called")
	}
	if speech.called != 1 || llm.called != 1 {
		t.Errorf("calls = speech:%d llm:%d", speech.called, llm.called)
	}
}

func TestProcessor_SpeechErrorMarksFailed(t *testing.T) {
	taskID := uuid.New()
	taskRepo := &fakeTaskRepo{task: &domain.Task{ID: taskID, Status: domain.StatusCreated}}
	meetingRepo := &fakeMeetingRepo{
		meeting: &domain.Meeting{ID: uuid.New(), UserID: uuid.New(), FilePath: "meeting.txt", OriginalFilename: "meeting.txt"},
	}
	speech := &fakeSpeech{err: errors.New("speech unavailable")}

	proc, _ := setupProcessor(t, taskRepo, meetingRepo, speech, &fakeLLM{})
	if err := proc(context.Background(), taskID); err == nil {
		t.Fatal("expected error from process")
	}
	if !taskRepo.failedCalled || !strings.Contains(taskRepo.failedMsg, "speech unavailable") {
		t.Errorf("failedCalled=%v msg=%q", taskRepo.failedCalled, taskRepo.failedMsg)
	}
	if taskRepo.completedCalled {
		t.Error("task must not complete after speech error")
	}
}

func TestProcessor_RecoverySummarized(t *testing.T) {
	taskID := uuid.New()
	taskRepo := &fakeTaskRepo{task: &domain.Task{ID: taskID, Status: domain.StatusSummarized}}
	meetingRepo := &fakeMeetingRepo{
		meeting: &domain.Meeting{ID: uuid.New(), UserID: uuid.New(), FilePath: "meeting.txt", OriginalFilename: "meeting.txt"},
	}
	speech := &fakeSpeech{}
	llm := &fakeLLM{}

	proc, _ := setupProcessor(t, taskRepo, meetingRepo, speech, llm)
	if err := proc(context.Background(), taskID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if !taskRepo.completedCalled {
		t.Error("summarized task must be finalized")
	}
	if speech.called != 0 || llm.called != 0 {
		t.Errorf("no external calls expected, speech=%d llm=%d", speech.called, llm.called)
	}
}

func TestProcessor_RecoveryTranscribed(t *testing.T) {
	taskID := uuid.New()
	taskRepo := &fakeTaskRepo{task: &domain.Task{ID: taskID, Status: domain.StatusTranscribed}}
	meetingRepo := &fakeMeetingRepo{
		meeting:    &domain.Meeting{ID: uuid.New(), UserID: uuid.New(), FilePath: "meeting.txt", OriginalFilename: "meeting.txt"},
		transcript: "уже сохранено",
	}
	speech := &fakeSpeech{}
	llm := &fakeLLM{summary: "выжимка"}

	proc, _ := setupProcessor(t, taskRepo, meetingRepo, speech, llm)
	if err := proc(context.Background(), taskID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if speech.called != 0 {
		t.Errorf("speech must not be called again, called=%d", speech.called)
	}
	if llm.called != 1 {
		t.Errorf("llm must be called once, called=%d", llm.called)
	}
	if taskRepo.summarizedSaved != "выжимка" || !taskRepo.completedCalled {
		t.Errorf("summarized=%q completed=%v", taskRepo.summarizedSaved, taskRepo.completedCalled)
	}
}

func TestProcessor_TerminalSkips(t *testing.T) {
	taskID := uuid.New()
	taskRepo := &fakeTaskRepo{task: &domain.Task{ID: taskID, Status: domain.StatusCompleted}}
	meetingRepo := &fakeMeetingRepo{}

	proc, _ := setupProcessor(t, taskRepo, meetingRepo, &fakeSpeech{}, &fakeLLM{})
	if err := proc(context.Background(), taskID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if taskRepo.processingCalled || taskRepo.completedCalled {
		t.Error("terminal task must be skipped")
	}
}

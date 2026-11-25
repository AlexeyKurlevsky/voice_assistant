package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

type fakeChatRepo struct {
	saved *domain.QA
}

func (f *fakeChatRepo) Save(_ context.Context, qa *domain.QA) error {
	qa.ID = uuid.New()
	qa.CreatedAt = time.Now()
	f.saved = qa
	return nil
}

func (f *fakeChatRepo) History(_ context.Context, _, _ uuid.UUID, _ int) ([]domain.QA, error) {
	return nil, nil
}

type fakeLLM struct {
	answer    string
	summarize string
	err       error
}

func (f *fakeLLM) Summarize(_ context.Context, _ string) (string, error) {
	return f.summarize, f.err
}

func (f *fakeLLM) Answer(_ context.Context, _, _ string) (string, error) {
	return f.answer, f.err
}

func TestChatService_Ask_Success(t *testing.T) {
	meetingRepo := &fakeMeetingRepo{transcript: "транскрипция"}
	taskRepo := &fakeTaskRepo{task: &domain.Task{Status: domain.StatusCompleted}}
	chatRepo := &fakeChatRepo{}
	llm := &fakeLLM{answer: "ответ"}

	svc := NewChatService(meetingRepo, taskRepo, chatRepo, llm, time.Second, discardLogger())
	qa, err := svc.Ask(context.Background(), uuid.New(), uuid.New(), "какие сроки?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if qa.Question != "какие сроки?" || qa.Answer != "ответ" {
		t.Errorf("qa = %q / %q", qa.Question, qa.Answer)
	}
	if chatRepo.saved == nil {
		t.Error("QA must be saved")
	}
}

func TestChatService_Ask_NotCompleted(t *testing.T) {
	taskRepo := &fakeTaskRepo{task: &domain.Task{Status: domain.StatusProcessing}}
	svc := NewChatService(&fakeMeetingRepo{}, taskRepo, &fakeChatRepo{}, &fakeLLM{}, time.Second, discardLogger())

	_, err := svc.Ask(context.Background(), uuid.New(), uuid.New(), "вопрос")
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestChatService_Ask_EmptyQuestion(t *testing.T) {
	taskRepo := &fakeTaskRepo{task: &domain.Task{Status: domain.StatusCompleted}}
	svc := NewChatService(&fakeMeetingRepo{}, taskRepo, &fakeChatRepo{}, &fakeLLM{}, time.Second, discardLogger())

	_, err := svc.Ask(context.Background(), uuid.New(), uuid.New(), "   ")
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestChatService_Ask_MeetingNotFound(t *testing.T) {
	taskRepo := &fakeTaskRepo{}
	svc := NewChatService(&fakeMeetingRepo{}, taskRepo, &fakeChatRepo{}, &fakeLLM{}, time.Second, discardLogger())

	_, err := svc.Ask(context.Background(), uuid.New(), uuid.New(), "вопрос")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestChatService_Ask_LLMError(t *testing.T) {
	taskRepo := &fakeTaskRepo{task: &domain.Task{Status: domain.StatusCompleted}}
	meetingRepo := &fakeMeetingRepo{transcript: "t"}
	svc := NewChatService(meetingRepo, taskRepo, &fakeChatRepo{}, &fakeLLM{err: errors.New("llm down")}, time.Second, discardLogger())

	if _, err := svc.Ask(context.Background(), uuid.New(), uuid.New(), "вопрос"); err == nil {
		t.Fatal("expected LLM error")
	}
}

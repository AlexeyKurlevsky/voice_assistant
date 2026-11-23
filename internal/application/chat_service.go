package application

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/domain"
)

// ChatService — сценарии вопросов по материалам встречи.
// Работает только с встречами в статусе completed: до этого нет
// полного контекста для ответа.
type ChatService struct {
	meetingRepo MeetingRepo
	taskRepo    TaskRepo
	chatRepo    ChatRepo
	llm         LLMClient
	llmTimeout  time.Duration
	log         *slog.Logger
}

// NewChatService создаёт сервис с указанными зависимостями.
func NewChatService(
	meetingRepo MeetingRepo,
	taskRepo TaskRepo,
	chatRepo ChatRepo,
	llm LLMClient,
	llmTimeout time.Duration,
	log *slog.Logger,
) *ChatService {
	return &ChatService{
		meetingRepo: meetingRepo,
		taskRepo:    taskRepo,
		chatRepo:    chatRepo,
		llm:         llm,
		llmTimeout:  llmTimeout,
		log:         log,
	}
}

// Ask задаёт вопрос по материалам встречи.
//
// Проверки:
//   - встреча принадлежит пользователю (ErrNotFound, если чужая);
//   - статус встречи — completed (ErrInvalidArgument, если ещё не готова);
//   - вопрос не пустой (ErrInvalidArgument).
//
// Контекст для LLM: выжимка (если есть) + транскрипция.
// Пара сохраняется в chat_messages. Ошибка сохранения не блокирует
// ответ пользователю — она логируется.
func (s *ChatService) Ask(ctx context.Context, userID, meetingID uuid.UUID, question string) (*domain.QA, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("%w: question is empty", domain.ErrInvalidArgument)
	}

	task, err := s.taskRepo.GetByMeetingIDForUser(ctx, meetingID, userID)
	if err != nil {
		return nil, err
	}
	if task.Status != domain.StatusCompleted {
		return nil, fmt.Errorf("%w: meeting not ready for questions: status=%s",
			domain.ErrInvalidArgument, task.Status)
	}

	transcript, err := s.meetingRepo.GetTranscript(ctx, meetingID, userID)
	if err != nil {
		return nil, err
	}

	contextText := transcript
	if summary, err := s.meetingRepo.GetSummary(ctx, meetingID, userID); err == nil {
		contextText = "[Краткая выжимка]\n" + summary + "\n\n[Транскрипция]\n" + transcript
	}

	llmCtx, cancel := context.WithTimeout(ctx, s.llmTimeout)
	answer, err := s.llm.Answer(llmCtx, question, contextText)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("chat_service.Ask: llm: %w", err)
	}

	qa := &domain.QA{
		UserID:    userID,
		MeetingID: meetingID,
		Question:  question,
		Answer:    answer,
	}
	if err := s.chatRepo.Save(ctx, qa); err != nil {
		s.log.Warn("chat_service.Ask: save qa", "err", err)
	}
	return qa, nil
}

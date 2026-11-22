// Package llm содержит реализации application.LLMClient.
// mock — для тестов и локальной разработки; реальные провайдеры
// (openrouter) выбираются через factory.New по env MS_LLM_PROVIDER.
package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Mock — LLMClient с детерминированными ответами.
// Summarize возвращает первые ~200 символов транскрипции с префиксом;
// Answer — шаблонный ответ с эхом вопроса. Задержка эмулирует реальный
// вызов, чтобы status <id> успевал показать промежуточные состояния.
type Mock struct {
	delay time.Duration
}

// NewMock создаёт mock с задержкой delay (MS_MOCK_LLM_DELAY).
func NewMock(delay time.Duration) *Mock {
	return &Mock{delay: delay}
}

// Summarize возвращает краткую выжимку из transcript.
// Отмена ctx прерывает задержку и возвращает context.Canceled.
func (m *Mock) Summarize(ctx context.Context, transcript string) (string, error) {
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	clean := strings.TrimSpace(transcript)
	if clean == "" {
		return "", fmt.Errorf("mock llm: empty transcript")
	}

	snippet := clean
	if len(snippet) > 200 {
		if idx := strings.LastIndex(snippet[:200], " "); idx > 0 {
			snippet = snippet[:idx]
		} else {
			snippet = snippet[:200]
		}
		snippet += "..."
	}
	return "Краткая выжимка: " + snippet, nil
}

// Answer возвращает шаблонный ответ на question по contextText.
func (m *Mock) Answer(ctx context.Context, question, contextText string) (string, error) {
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	q := strings.TrimSpace(question)
	if q == "" {
		return "", fmt.Errorf("mock llm: empty question")
	}
	return fmt.Sprintf("Ответ на вопрос «%s»: на основе материалов встречи — детали в транскрипции (mock).", q), nil
}

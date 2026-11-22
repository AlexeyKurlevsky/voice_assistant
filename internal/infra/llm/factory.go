package llm

import (
	"fmt"
	"time"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

// Options — параметры фабрики LLMClient.
type Options struct {
	// MockDelay — задержка mock-клиента (MS_MOCK_LLM_DELAY).
	MockDelay time.Duration
}

// New выбирает реализацию application.LLMClient по имени провайдера.
//
// Допустимые значения: "mock" (реализовано), "openrouter" (зарезервировано).
// Неизвестный провайдер или нереализованный — ошибка.
func New(provider string, opts Options) (application.LLMClient, error) {
	switch provider {
	case "mock":
		return NewMock(opts.MockDelay), nil
	case "openrouter":
		return nil, fmt.Errorf("llm factory: openrouter provider is not implemented yet")
	default:
		return nil, fmt.Errorf("llm factory: unknown provider %q", provider)
	}
}

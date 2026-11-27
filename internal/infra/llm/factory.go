package llm

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

// Options — параметры фабрики LLMClient.
type Options struct {
	// MockDelay — задержка mock-клиента (MS_MOCK_LLM_DELAY).
	MockDelay time.Duration

	// BaseURL, APIKey, Model — параметры OpenAI-совместимого провайдера.
	BaseURL string
	APIKey  string
	Model   string

	// HTTPClient — инъектируемый клиент (в тестах указывает на httptest).
	HTTPClient *http.Client
}

// New выбирает реализацию application.LLMClient по имени провайдера.
//
// Допустимые значения: "mock" (детерминированная заглушка), "openrouter"
// (OpenAI-совместимый API, модель задаётся через opts.Model).
// Неизвестный провайдер — ошибка.
func New(provider string, opts Options) (application.LLMClient, error) {
	switch provider {
	case "mock":
		return NewMock(opts.MockDelay), nil
	case "openrouter":
		return NewOpenAICompat(opts.BaseURL, opts.APIKey, opts.Model, opts.HTTPClient)
	default:
		return nil, fmt.Errorf("llm factory: unknown provider %q", provider)
	}
}

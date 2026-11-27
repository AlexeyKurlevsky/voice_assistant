package speech

import (
	"fmt"
	"time"

	"github.com/AlexeyKurlevsky/voice_assistant/internal/application"
)

// Options — параметры фабрики SpeechClient.
type Options struct {
	// MockDelay — задержка mock-клиента (MS_MOCK_SPEECH_DELAY).
	MockDelay time.Duration
}

// New выбирает реализацию application.SpeechClient по имени провайдера.
//
// Допустимые значения: "mock" (реализовано), "whisper" (зарезервировано).
// Неизвестный провайдер или нереализованный — ошибка.
func New(provider string, opts Options) (application.SpeechClient, error) {
	switch provider {
	case "mock":
		return NewMock(opts.MockDelay), nil
	case "whisper":
		return nil, fmt.Errorf("speech factory: whisper provider is not implemented yet")
	default:
		return nil, fmt.Errorf("speech factory: unknown provider %q", provider)
	}
}

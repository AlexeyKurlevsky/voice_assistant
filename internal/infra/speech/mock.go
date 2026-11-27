// Package speech содержит реализации application.SpeechClient.
// mock — для тестов и локальной разработки без внешних API;
// реальные провайдеры (whisper) добавляются как отдельные типы
// и выбираются через factory.New по env MS_SPEECH_PROVIDER.
package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// Mock — SpeechClient с фиксированной транскрипцией.
//
// Поведение по расширению:
//   - .txt — содержимое файла как транскрипция;
//   - .json — поле "transcription" (или ошибка, если "fail": true);
//   - остальное (аудио) — фиксированная заглушка.
//
// Задержка эмулирует реальный вызов, чтобы status <id> успевал
// показать промежуточные состояния.
type Mock struct {
	delay time.Duration
}

// NewMock создаёт mock с задержкой delay (MS_MOCK_SPEECH_DELAY).
func NewMock(delay time.Duration) *Mock {
	return &Mock{delay: delay}
}

// Transcribe возвращает транскрипцию по правилам Mock (см. тип).
// Отмена ctx прерывает задержку и возвращает context.Canceled.
func (m *Mock) Transcribe(ctx context.Context, filename string, r io.Reader) (string, error) {
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt":
		data, err := io.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("mock speech: read txt: %w", err)
		}
		return string(data), nil

	case ".json":
		data, err := io.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("mock speech: read json: %w", err)
		}
		var payload struct {
			Transcription string `json:"transcription"`
			Fail          bool   `json:"fail"`
			FailMessage   string `json:"fail_message"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return "", fmt.Errorf("mock speech: invalid json: %w", err)
		}
		if payload.Fail {
			msg := payload.FailMessage
			if msg == "" {
				msg = "mock failure (as requested)"
			}
			return "", fmt.Errorf("mock speech: %s", msg)
		}
		if payload.Transcription == "" {
			return "", fmt.Errorf("mock speech: empty transcription in json")
		}
		return payload.Transcription, nil

	default:
		return fmt.Sprintf("Тестовая транскрипция для файла %q. Обсудили бюджет Q1, сроки релиза и план найма.", filename), nil
	}
}

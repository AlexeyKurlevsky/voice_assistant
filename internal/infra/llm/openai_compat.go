package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxBodyBytes ограничивает размер читаемого тела ответа API, чтобы
// не аллоцировать неограниченно при сбое на стороне провайдера.
const maxBodyBytes = 4 << 20 // 4 MiB

// defaultOpenRouterBaseURL — базовый URL OpenRouter.
// Endpoint строится как baseURL + "/chat/completions".
const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenAICompat — универсальный LLM-клиент поверх OpenAI-совместимого
// /chat/completions API. Работает с OpenRouter, DeepSeek, Groq и любым
// другим провайдером, который понимает формат OpenAI: конкретный сервис
// и модель задаются через baseURL, apiKey и model.
//
// Таймауты и отмена управляются исключительно переданным context.Context:
// вызывающий (worker/processor) уже оборачивает вызов в
// context.WithTimeout. Клиент не навешивает собственный deadline.
type OpenAICompat struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAICompat создаёт OpenAI-совместимый клиент.
// apiKey и model обязательны; baseURL имеет дефолт OpenRouter.
// client можно передать nil — тогда используется &http.Client{}.
func NewOpenAICompat(baseURL, apiKey, model string, client *http.Client) (*OpenAICompat, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai compat: API key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("openai compat: model is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOpenRouterBaseURL
	}
	if client == nil {
		client = &http.Client{}
	}
	return &OpenAICompat{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: client,
	}, nil
}

// chatMessage — сообщение диалога в формате OpenAI.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest — тело запроса /chat/completions.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatCompletionResponse — тело успешного ответа.
type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// apiErrorResponse — тело ошибки провайдера (поле error).
type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Summarize формирует краткую выжимку по транскрипции встречи.
func (c *OpenAICompat) Summarize(ctx context.Context, transcript string) (string, error) {
	if strings.TrimSpace(transcript) == "" {
		return "", fmt.Errorf("openai compat: empty transcript")
	}
	return c.complete(ctx, []chatMessage{
		{
			Role: "system",
			Content: "Ты — ассистент для конспектирования встреч. " +
				"По переданной транскрипции составь краткую выжимку: " +
				"ключевые темы, договорённости, сроки и ответственных. " +
				"Отвечай только по тексту, без домыслов. Пиши по-русски.",
		},
		{Role: "user", Content: transcript},
	})
}

// Answer отвечает на вопрос по переданному контексту материалов встречи.
func (c *OpenAICompat) Answer(ctx context.Context, question, contextText string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", fmt.Errorf("openai compat: empty question")
	}
	return c.complete(ctx, []chatMessage{
		{
			Role: "system",
			Content: "Ты — ассистент, отвечающий на вопросы по материалам встречи. " +
				"Отвечай строго на основе переданного контекста. " +
				"Если в контексте нет ответа — прямо скажи об этом. Пиши по-русски.",
		},
		{
			Role:    "user",
			Content: "Контекст:\n\n" + contextText + "\n\nВопрос: " + question,
		},
	})
}

// complete — один вызов /chat/completions и разбор ответа.
func (c *OpenAICompat) complete(ctx context.Context, messages []chatMessage) (string, error) {
	payload := chatCompletionRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("openai compat: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai compat: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai compat: do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("openai compat: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai compat: api error %d: %s", resp.StatusCode, errorMessage(data))
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("openai compat: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openai compat: empty choices in response")
	}
	content := strings.TrimSpace(out.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("openai compat: empty completion content")
	}
	return content, nil
}

// endpoint возвращает полный URL метода /chat/completions.
func (c *OpenAICompat) endpoint() string {
	return c.baseURL + "/chat/completions"
}

// errorMessage достаёт человекочитаемое сообщение из тела ошибки.
// Если JSON не содержит error.message — возвращает сырое тело.
func errorMessage(data []byte) string {
	var e apiErrorResponse
	if err := json.Unmarshal(data, &e); err == nil && e.Error.Message != "" {
		return strings.TrimSpace(e.Error.Message)
	}
	return strings.TrimSpace(string(data))
}

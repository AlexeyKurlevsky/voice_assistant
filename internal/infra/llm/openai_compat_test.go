package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompat_Summarize(t *testing.T) {
	var gotPath, gotAuth, gotModel, gotUser string
	var gotStream bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotModel = req.Model
		gotStream = req.Stream
		if len(req.Messages) > 0 {
			gotUser = req.Messages[len(req.Messages)-1].Content
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Краткая выжимка"}},
			},
		})
	}))
	defer srv.Close()

	client, err := NewOpenAICompat(srv.URL, "test-key", "deepseek/deepseek-chat", srv.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompat: %v", err)
	}

	got, err := client.Summarize(context.Background(), "Обсудили бюджет и сроки.")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got != "Краткая выжимка" {
		t.Fatalf("unexpected content: %q", got)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotModel != "deepseek/deepseek-chat" {
		t.Errorf("model = %q", gotModel)
	}
	if gotStream {
		t.Errorf("stream = true, want false")
	}
	if gotUser != "Обсудили бюджет и сроки." {
		t.Errorf("user content = %q", gotUser)
	}
}

func TestOpenAICompat_Answer(t *testing.T) {
	var gotUser string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 {
			gotUser = req.Messages[len(req.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Сроки: до конца месяца."}}]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAICompat(srv.URL, "k", "m", srv.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompat: %v", err)
	}

	got, err := client.Answer(context.Background(), "какие сроки?", "Транскрипция встречи...")
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if got != "Сроки: до конца месяца." {
		t.Fatalf("unexpected answer: %q", got)
	}
	if !strings.Contains(gotUser, "какие сроки?") {
		t.Errorf("user content missing question: %q", gotUser)
	}
	if !strings.Contains(gotUser, "Транскрипция встречи...") {
		t.Errorf("user content missing context: %q", gotUser)
	}
}

func TestOpenAICompat_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit exceeded","type":"rate_limit"}}`))
	}))
	defer srv.Close()

	client, err := NewOpenAICompat(srv.URL, "k", "m", srv.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompat: %v", err)
	}

	_, err = client.Summarize(context.Background(), "текст")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAICompat_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	client, err := NewOpenAICompat(srv.URL, "k", "m", srv.Client())
	if err != nil {
		t.Fatalf("NewOpenAICompat: %v", err)
	}

	if _, err := client.Summarize(context.Background(), "текст"); err == nil {
		t.Fatal("expected empty choices error, got nil")
	}
}

func TestOpenAICompat_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отмена до запроса — Do обязан вернуть context.Canceled сразу

	c, err := NewOpenAICompat("https://example.invalid", "k", "m", nil)
	if err != nil {
		t.Fatalf("NewOpenAICompat: %v", err)
	}

	_, err = c.Summarize(ctx, "текст")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestOpenAICompat_ContextCanceledMidFlight(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Обработчик всегда завершается быстро: отмена на клиенте проверяется
		// отдельно, а сервер лишь должен не блокировать Close().
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	// DisableKeepAlives — клиент сразу закрывает соединение после отмены,
	// иначе httptest.Server.Close() ждёт «живое» соединение ~10 секунд.
	// Timeout — страховка, чтобы тест не завис, даже если отмена не сработает.
	client, err := NewOpenAICompat(srv.URL, "k", "m", &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	})
	if err != nil {
		t.Fatalf("NewOpenAICompat: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = client.Summarize(ctx, "текст")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestNewOpenAICompat_Validation(t *testing.T) {
	if _, err := NewOpenAICompat("", "", "m", nil); err == nil {
		t.Error("expected error for empty key")
	}
	if _, err := NewOpenAICompat("", "k", "", nil); err == nil {
		t.Error("expected error for empty model")
	}

	c, err := NewOpenAICompat("", "k", "m", nil)
	if err != nil {
		t.Fatalf("NewOpenAICompat with defaults: %v", err)
	}
	if c.baseURL != defaultOpenRouterBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultOpenRouterBaseURL)
	}
}

func TestNew_ProviderSelection(t *testing.T) {
	if _, err := New("mock", Options{}); err != nil {
		t.Fatalf("mock provider: %v", err)
	}
	if _, err := New("openrouter", Options{APIKey: "k", Model: "m"}); err != nil {
		t.Fatalf("openrouter provider: %v", err)
	}
	if _, err := New("unknown", Options{}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

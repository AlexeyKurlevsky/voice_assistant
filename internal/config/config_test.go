package config

import (
	"strings"
	"testing"
	"time"
)

func loadTestConfig(t *testing.T, extra map[string]string) (*Config, error) {
	t.Helper()
	t.Setenv("MS_DB_DSN", "postgres://u:p@localhost:5432/db?sslmode=disable")
	for k, v := range extra {
		t.Setenv(k, v)
	}
	return LoadWithOptions(LoadOptions{EnvFile: ""})
}

func TestLoad_Defaults(t *testing.T) {
	c, err := loadTestConfig(t, nil)
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if c.WorkerConcurrency != 3 {
		t.Errorf("WorkerConcurrency = %d, want 3", c.WorkerConcurrency)
	}
	if c.JobQueueCapacity != 100 {
		t.Errorf("JobQueueCapacity = %d, want 100", c.JobQueueCapacity)
	}
	if c.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %s", c.ShutdownTimeout)
	}
	if c.SpeechProvider != "mock" || c.LLMProvider != "mock" {
		t.Errorf("providers = %q/%q", c.SpeechProvider, c.LLMProvider)
	}
	if c.LLMBaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("LLMBaseURL = %q", c.LLMBaseURL)
	}
}

func TestLoad_RequiresDSN(t *testing.T) {
	t.Setenv("MS_DB_DSN", "")
	_, err := LoadWithOptions(LoadOptions{EnvFile: ""})
	if err == nil || !strings.Contains(err.Error(), "MS_DB_DSN") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_UnknownProvider(t *testing.T) {
	_, err := loadTestConfig(t, map[string]string{"MS_LLM_PROVIDER": "watson"})
	if err == nil || !strings.Contains(err.Error(), "MS_LLM_PROVIDER") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_OpenRouterRequiresKeyAndModel(t *testing.T) {
	_, err := loadTestConfig(t, map[string]string{"MS_LLM_PROVIDER": "openrouter"})
	if err == nil || !strings.Contains(err.Error(), "MS_LLM_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = loadTestConfig(t, map[string]string{
		"MS_LLM_PROVIDER": "openrouter",
		"MS_LLM_API_KEY":  "sk-test",
	})
	if err == nil || !strings.Contains(err.Error(), "MS_LLM_MODEL") {
		t.Fatalf("unexpected error: %v", err)
	}

	c, err := loadTestConfig(t, map[string]string{
		"MS_LLM_PROVIDER": "openrouter",
		"MS_LLM_API_KEY":  "sk-test",
		"MS_LLM_MODEL":    "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if c.LLMAPIKey != "sk-test" || c.LLMModel != "deepseek-chat" {
		t.Errorf("LLM config = %q/%q", c.LLMAPIKey, c.LLMModel)
	}
}

func TestLoad_InvalidConcurrency(t *testing.T) {
	_, err := loadTestConfig(t, map[string]string{"MS_WORKER_CONCURRENCY": "0"})
	if err == nil || !strings.Contains(err.Error(), "MS_WORKER_CONCURRENCY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

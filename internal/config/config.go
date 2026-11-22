// Package config загружает конфигурацию из .env и переменных окружения.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config — все настройки приложения.
type Config struct {
	DBDSN          string
	DBMaxConns     int32
	MigrateOnStart bool

	UploadDir      string
	MaxUploadBytes int64

	WorkerConcurrency int
	JobQueueCapacity  int
	ShutdownTimeout   time.Duration

	SpeechTimeout time.Duration
	LLMTimeout    time.Duration

	SpeechProvider string
	LLMProvider    string

	MockSpeechDelay time.Duration
	MockLLMDelay    time.Duration

	LogLevel  string
	LogFormat string
}

// LoadOptions управляет источниками конфигурации.
// EnvFile == "" отключает чтение .env (для тестов).
type LoadOptions struct {
	EnvFile string
}

// Load — сахар для типичного случая: .env в текущей директории.
func Load() (*Config, error) {
	return LoadWithOptions(LoadOptions{EnvFile: ".env"})
}

// LoadWithOptions загружает конфиг: defaults → .env → env процесса.
// Возвращает ошибку при невалидных или отсутствующих обязательных значениях.
func LoadWithOptions(opts LoadOptions) (*Config, error) {
	if opts.EnvFile != "" {
		_ = godotenv.Load(opts.EnvFile)
	}

	c := &Config{
		DBDSN:             os.Getenv("MS_DB_DSN"),
		DBMaxConns:        int32(getEnvInt("MS_DB_MAX_CONNS", 10)),
		MigrateOnStart:    getEnvBool("MS_MIGRATE_ON_START", true),
		UploadDir:         getEnv("MS_UPLOAD_DIR", "./data/uploads"),
		MaxUploadBytes:    int64(getEnvInt("MS_MAX_UPLOAD_BYTES", 100*1024*1024)),
		WorkerConcurrency: getEnvInt("MS_WORKER_CONCURRENCY", 3),
		JobQueueCapacity:  getEnvInt("MS_JOB_QUEUE_CAPACITY", 100),
		ShutdownTimeout:   getEnvDuration("MS_SHUTDOWN_TIMEOUT", 10*time.Second),
		SpeechTimeout:     getEnvDuration("MS_SPEECH_TIMEOUT", 120*time.Second),
		LLMTimeout:        getEnvDuration("MS_LLM_TIMEOUT", 60*time.Second),
		SpeechProvider:    getEnv("MS_SPEECH_PROVIDER", "mock"),
		LLMProvider:       getEnv("MS_LLM_PROVIDER", "mock"),
		MockSpeechDelay:   getEnvDuration("MS_MOCK_SPEECH_DELAY", 2*time.Second),
		MockLLMDelay:      getEnvDuration("MS_MOCK_LLM_DELAY", 1*time.Second),
		LogLevel:          getEnv("MS_LOG_LEVEL", "info"),
		LogFormat:         getEnv("MS_LOG_FORMAT", "text"),
	}

	if c.DBDSN == "" {
		return nil, fmt.Errorf("config: MS_DB_DSN is required")
	}
	if c.WorkerConcurrency <= 0 {
		return nil, fmt.Errorf("config: MS_WORKER_CONCURRENCY must be > 0")
	}
	if c.JobQueueCapacity <= 0 {
		return nil, fmt.Errorf("config: MS_JOB_QUEUE_CAPACITY must be > 0")
	}
	if c.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("config: MS_SHUTDOWN_TIMEOUT must be > 0")
	}
	if c.MaxUploadBytes <= 0 {
		return nil, fmt.Errorf("config: MS_MAX_UPLOAD_BYTES must be > 0")
	}
	if c.SpeechTimeout <= 0 || c.LLMTimeout <= 0 {
		return nil, fmt.Errorf("config: MS_SPEECH_TIMEOUT and MS_LLM_TIMEOUT must be > 0")
	}
	switch c.SpeechProvider {
	case "mock", "whisper":
	default:
		return nil, fmt.Errorf("config: unknown MS_SPEECH_PROVIDER=%q", c.SpeechProvider)
	}
	switch c.LLMProvider {
	case "mock", "openrouter":
	default:
		return nil, fmt.Errorf("config: unknown MS_LLM_PROVIDER=%q", c.LLMProvider)
	}
	return c, nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

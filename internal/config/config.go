package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// БД
	DBDSN          string
	DBMaxConns     int32
	MigrateOnStart bool

	// Обработка
	WorkerConcurrency int
	JobQueueCapacity  int
	ShutdownTimeout   time.Duration

	// Провайдеры (используются начиная со следующего батча)
	SpeechProvider string
	LLMProvider    string

	// Логи
	LogLevel  string
	LogFormat string
}

// LoadOptions управляет источниками конфигурации.
// Нулевое значение = загрузить ".env" (типичное поведение для CLI).
// В тестах передавайте LoadOptions{EnvFile: ""} — читать будет только os.Getenv.
type LoadOptions struct {
	EnvFile string
}

// Load — сахар для типичного случая: .env в текущей директории.
func Load() (*Config, error) {
	return LoadWithOptions(LoadOptions{EnvFile: ".env"})
}

func LoadWithOptions(opts LoadOptions) (*Config, error) {
	if opts.EnvFile != "" {
		// Файла может не быть — это нормально. godotenv не перезаписывает
		// уже установленные переменные окружения, приоритет сохраняется:
		// defaults → .env → env процесса.
		_ = godotenv.Load(opts.EnvFile)
	}

	c := &Config{
		DBDSN:             os.Getenv("MS_DB_DSN"),
		DBMaxConns:        int32(getEnvInt("MS_DB_MAX_CONNS", 10)),
		MigrateOnStart:    getEnvBool("MS_MIGRATE_ON_START", true),
		WorkerConcurrency: getEnvInt("MS_WORKER_CONCURRENCY", 3),
		JobQueueCapacity:  getEnvInt("MS_JOB_QUEUE_CAPACITY", 100),
		ShutdownTimeout:   getEnvDuration("MS_SHUTDOWN_TIMEOUT", 10*time.Second),
		SpeechProvider:    getEnv("MS_SPEECH_PROVIDER", "mock"),
		LLMProvider:       getEnv("MS_LLM_PROVIDER", "mock"),
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

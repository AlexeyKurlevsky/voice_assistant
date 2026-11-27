# Тестирование

Документ описывает текущее состояние тестов: что уже реализовано, как
мокается, как проверяется конкурентность, и что запланировано.

**Статус:** unit-тесты реализованы и проходят (`go test ./...`,
`go test -race ./...` зелёные). Интеграционные тесты с PostgreSQL —
**план**, ещё не написаны.

---

## 1. Пирамида

```
        ┌─────────────────────────────┐
        │   integration tests         │  ПЛАН: обёртки repo + PostgreSQL
        │   (TEST_DATABASE_URL)       │
        └────────────┬────────────────┘
                     │
        ┌────────────▼────────────────┐
        │     unit tests              │  РЕАЛИЗОВАНО: application-сервисы,
        │     mock/fake, без БД/сети  │  domain, config, speech, llm, worker, cli
        └────────────┬────────────────┘
                     │
        ┌────────────▼────────────────┐
        │   race / timeout /          │  РЕАЛИЗОВАНО: go test -race ./...
        │   cancellation tests        │
        └─────────────────────────────┘
```

**Ключевое правило:** `sqlcgen` и обёртки `repo/*.go` не мокаются в
unit-тестах — их проверяют интеграционные тесты против реальной PostgreSQL.
Unit-тесты работают с интерфейсами `application.*Repo` и ручными фейками.

---

## 2. Реализованные unit-тесты

| Пакет | Что проверяется |
|---|---|
| `internal/domain` | `Status.IsValid`, `Status.IsTerminal` |
| `internal/application` | `UserService` (Register, валидация external_id, Auth, Rotate); `MeetingService` (Load: успех/файл не найден/расширение/переполнение очереди, нормализация пагинации, пустой find, transcript); `ChatService` (completed-гейт, пустой вопрос, not found, ошибка LLM); `WorkerPool` (enqueue успех/полный/после Stop, идемпотентность Stop, лимит параллелизма) |
| `internal/config` | дефолты, обязательный DSN, невалидный провайдер, обязательные `MS_LLM_API_KEY`/`MS_LLM_MODEL`, `MS_WORKER_CONCURRENCY` |
| `internal/infra/speech` | mock: `.txt`, `.json`, `fail`, аудио-заглушка, отмена контекста |
| `internal/infra/llm` | mock (`Summarize`/`Answer`, пустые входы, отмена); `OpenAICompat` (`httptest`: тело запроса, Bearer, ошибка API, пустые choices, отмена до/во время запроса, валидация, выбор провайдера) |
| `internal/worker` | полный pipeline, ошибка speech → failed, recovery из `summarized`/`transcribed`, пропуск терминальных |
| `internal/cli` | `start` (успех и без `--user`), `load` без токена, неизвестная команда |

Запуск:

```bash
go test ./...
go test -race ./...
```

---

## 3. Как мокаем

Без `mockery`/`gomock` — ручные фейки в `_test.go` рядом с тестом.
Интерфейсы узкие (3–7 методов), поэтому фейки занимают ~15–30 строк.

- Репозитории и клиенты: структуры-фейки, реализующие интерфейсы из
  `internal/application/ports.go`.
- Файловые операции: `t.TempDir()` + `os.WriteFile`.
- HTTP-клиент `OpenAICompat`: `httptest.NewServer`, тело запроса
  проверяется декодированием JSON, ответы — фиксированные JSON-строки.
- Отмена/таймауты: `context.WithCancel` / `context.WithTimeout`.

Пример фейка:

```go
type fakeMeetingRepo struct {
    createErr error
    created   *domain.Meeting
}

func (f *fakeMeetingRepo) CreateWithTask(_ context.Context, m *domain.Meeting) (*domain.Task, error) {
    if f.createErr != nil {
        return nil, f.createErr
    }
    m.ID = uuid.New()
    f.created = m
    return &domain.Task{ID: uuid.New(), MeetingID: m.ID, Status: domain.StatusCreated}, nil
}
```

---

## 4. Конкурентность: race, timeout, cancellation, лимит

Обязательная проверка перед коммитом:

```bash
go test -race ./...
```

Реализованы и проходят:

- `TestWorkerPool_LimitConcurrency` — атомарный счётчик подтверждает,
  что одновременно работает не больше `WorkerConcurrency` задач.
- отмена контекста: `TestMock_RespectsContext` (speech/llm),
  `TestOpenAICompat_ContextCanceled` и `TestOpenAICompat_ContextCanceledMidFlight`.
- `TestWorkerPool_Stop_Idempotent`, `TestWorkerPool_Enqueue_AfterStop`.

Тесты на утечку горутин и полный graceful shutdown — опционально,
описаны ниже в плане.

---

## 5. Интеграционные тесты (план)

### 5.1. Требование к окружению

- PostgreSQL 14+ запущен локально.
- `TEST_DATABASE_URL` указывает на **отдельную** БД (не `MS_DB_DSN`).
- Если переменная не задана — тесты делают `t.Skip`.

### 5.2. Подготовка БД

`tests/integration/main_test.go` с `TestMain`:
подключиться через `pgxpool`, прогнать миграции из `embed.FS`, перед каждым
тестом `TRUNCATE ... RESTART IDENTITY CASCADE`. Без Docker.

### 5.3. Покрываемые сценарии

| # | Сценарий |
|---|---|
| I1 | `CreateWithTask` — встреча + задача + история в одной TX |
| I2 | Откат `CreateWithTask` при ошибке `INSERT processing_tasks` |
| I3–I7 | Каждый переход статуса + `status_history` |
| I8 | `ListByUser` — только встречи пользователя, сортировка |
| I9–I10 | `GetByIDForUser` — чужая/несуществующая → `ErrNotFound` |
| I11–I12 | `Search` — поиск по транскрипциям/выжимкам, сниппет, пустой результат |
| I13 | `GetTranscript` чужая встреча → `ErrNotFound` |
| I14 | `ChatRepo.Save` + `History` |
| I15 | `ResetStaleProcessing` — `processing → created` без записи в history |
| I16 | Откат `SaveTranscriptionAndMarkTranscribed` при дубликате `meeting_id` |

Планируемая структура:

```
tests/integration/
  main_test.go
  helpers_test.go
  meeting_repo_test.go
  task_repo_test.go
  search_test.go
  chat_repo_test.go
```

После реализации добавить Makefile-таргет:

```makefile
test-integration:
	TEST_DATABASE_URL=$${TEST_DATABASE_URL} go test -tags=integration -race ./tests/integration/...
```

---

## 6. Чего в тестах нет и почему

| Не тестируется | Причина |
|---|---|
| Реальные внешние API (OpenRouter, Whisper) | Не воспроизводимы в CI, зависят от ключей/квот. `OpenAICompat` проверяется через `httptest`. |
| `sqlcgen` изолированно | Генерируется из SQL; проверяется интеграционными тестами. |
| `main.go` | Собирается `go build`; логика вынесена в `application`/`worker`. |
| `logger` | Тонкая обёртка над `slog`, покрытия не требует. |

---

## 7. Профилирование (pprof)

Профили снимаются через бенчмарки и `go test` — это стандартный способ для
CLI-проекта (HTTP-`/debug/pprof` тут не к чему привязать). Покрыты
CPU-нагруженные пути без внешних зависимостей:

- `internal/application` — `BenchmarkFileSHA256`,
  `BenchmarkMeetingService_Load`, `BenchmarkWorkerPool_EnqueueProcess`;
- `internal/infra/llm` — `BenchmarkOpenAICompat_MarshalRequest`,
  `BenchmarkOpenAICompat_ParseResponse`.

Снять профиль:

```bash
make profile-cpu    # go test -bench . -cpuprofile → go tool pprof -top
make profile-mem    # то же для -memprofile
```

Профили пишутся в `./cpu.prof` и `./mem.prof` (в `.gitignore`). Для
интерактивного анализа — веб-интерфейс:

```bash
go tool pprof -http=:6060 cpu.prof
```

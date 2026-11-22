# Тестирование

Документ описывает стратегию тестирования: что покрывается unit-тестами,
что — интеграционными, как проверяется конкурентное поведение, и что
намеренно не тестируется.

---

## 1. Пирамида

```
        ┌─────────────────────────┐
        │   integration tests     │  обёртки *_repo.go + реальный PostgreSQL
        │   ~15–20 сценариев      │  (TEST_DATABASE_URL)
        └────────────┬────────────┘
                     │
        ┌────────────▼────────────┐
        │     unit tests          │  application-сервисы + mock-клиенты
        │     ~40–60 тестов       │  без БД, без сети
        └────────────┬────────────┘
                     │
        ┌────────────▼────────────┐
        │   race / timeout /      │  go test -race ./...
        │   cancellation tests    │  отдельные тесты на конкурентность
        └─────────────────────────┘
```

**Ключевое правило:** `sqlcgen` **не мокается**. Это тонкая обёртка над
SQL, её поведение проверяется интеграционными тестами против реальной
PostgreSQL. Unit-тесты работают с `application.*Repo`-интерфейсами,
реализации которых — ручные фейки, а не `sqlcgen`.

---

## 2. Unit-тесты

### 2.1. Что покрывается

| Пакет | Что тестируется | Моки |
|---|---|---|
| `internal/application` | `UserService.EnsureExists` | `UserRepo` (fake) |
| `internal/application` | `MeetingService.Load` — валидация файла, создание встречи + задачи, `Enqueue` | `MeetingRepo`, `WorkerPool.Enqueuer` |
| `internal/application` | `MeetingService.List`, `Get`, `Status`, `Find` | `MeetingRepo`, `TaskRepo` |
| `internal/application` | `ChatService.Ask` — проверка `completed`-статуса, вызов LLM, сохранение Q/A | `MeetingRepo`, `ChatRepo`, `LLMClient` |
| `internal/application` | `WorkerPool.Enqueue` — успех, переполнение (`ErrQueueFull`), отмена контекста | канал-заглушка |
| `internal/infra/speech/mock` | `Transcribe` для `.txt`, `.json`, аудио; задержка; отмена контекста | — |
| `internal/infra/llm/mock` | `Summarize`, `Answer`; задержка; отмена контекста | — |
| `internal/cli` | Парсинг аргументов, `--limit`/`--offset`, коды возврата | сервисы (fake) |

### 2.2. Как мокать

Без `mockery`/`gomock` — ручные фейки в `_test.go` рядом с тестом. Причина:
интерфейсы узкие (3–7 методов), фейки пишутся за 15 строк и читаются проще,
чем сгенерированные.

Пример:

```go
type fakeMeetingRepo struct {
    created  *domain.Meeting
    createErr error
    listRes  []domain.MeetingListItem
}

func (f *fakeMeetingRepo) CreateWithTask(ctx context.Context, m *domain.Meeting) (*domain.Task, error) {
    if f.createErr != nil {
        return nil, f.createErr
    }
    f.created = m
    return &domain.Task{ID: 1, MeetingID: 1, Status: domain.StatusCreated}, nil
}
// ... остальные методы
```

Тест:

```go
func TestMeetingService_Load_QueueFull(t *testing.T) {
    repo := &fakeMeetingRepo{}
    pool := &fakePool{enqueueErr: domain.ErrQueueFull}
    repo.markFailedCalled = false

    svc := application.NewMeetingService(repo, pool, t.TempDir(), 1024*1024)

    _, err := svc.Load(ctx, userID, pathToTempFile(t, "audio.mp3"))
    require.ErrorIs(t, err, domain.ErrQueueFull)
    require.True(t, repo.markFailedCalled)
}
```

### 2.3. Что **не** покрывается unit-тестами

- `sqlcgen` — генерируется, не пишется руками.
- `postgres.*_repo.go` — обёртки, их поведение — интеграционные тесты.
- `main.go` — сборка приложения, проверяется `go build` и сквозным сценарием.
- HTTP-клиенты реальных провайдеров (`whisper.go`, `openrouter.go`) —
  либо `httptest.Server` в unit-тесте клиента (если будем реализовывать),
  либо откладываются до момента реальной интеграции.

### 2.4. Запуск

```bash
go test ./internal/application/... ./internal/infra/speech/...
go test ./...
```

Быстро, без зависимостей. `t.TempDir()` для файловых операций.

---

## 3. Интеграционные тесты

### 3.1. Требование к окружению

- PostgreSQL 14+ запущен локально.
- Переменная `TEST_DATABASE_URL` указывает на **отдельную** БД
  (не `MS_DB_DSN`!), например:
  ```bash
  export TEST_DATABASE_URL="postgres://ms:ms@localhost:5432/ms_test?sslmode=disable"
  ```
- Если переменная не задана — все тесты в `tests/integration/` делают
  `t.Skip("TEST_DATABASE_URL not set")`. Это позволяет `go test ./...`
  работать без подготовки БД.

### 3.2. Подготовка БД

`tests/integration/main_test.go` с `TestMain`:

1. `pgxpool.New(TEST_DATABASE_URL)`.
2. Прогнать миграции из `embed.FS` (`migrate.Up`).
3. Для каждого теста — `TRUNCATE users, meetings, processing_tasks,
   status_history, transcriptions, summaries, chat_messages RESTART IDENTITY CASCADE`
   в `TestMain`/подготовке.

**Без Docker.** Миграции те же, что и в проде (`0001_init.up.sql`).

### 3.3. Что покрывается

| # | Сценарий | Что проверяет |
|---|---|---|
| I1 | `CreateWithTask` | `INSERT meetings` + `INSERT processing_tasks` + `INSERT status_history` в одной TX; task.status = `created` |
| I2 | `CreateWithTask` при ошибке `INSERT processing_tasks` | `meetings` не остаётся «осиротевшей» (откат TX) |
| I3 | `MarkProcessing` | `status='processing'`, `attempts=1`, в `status_history` запись `created → processing` |
| I4 | `SaveTranscriptionAndMarkTranscribed` | `transcriptions` содержит текст, `status='transcribed'`, история `processing → transcribed` |
| I5 | `SaveSummaryAndMarkSummarized` | `summaries` содержит текст, `status='summarized'`, история `transcribed → summarized` |
| I6 | `MarkCompleted` | `status='completed'`, история `summarized → completed` |
| I7 | `MarkFailed` | `status='failed'`, `error_message` заполнен, история |
| I8 | `ListByUser` | возвращает только встречи указанного `userID`, сортировка `created_at DESC`, `summary` пустой, если выжимки нет |
| I9 | `GetByIDForUser` — чужая встреча | `domain.ErrNotFound`, не `ErrForbidden` |
| I10 | `GetByIDForUser` — несуществующая | `domain.ErrNotFound` |
| I11 | `SearchMeetings` | находит по слову в `transcriptions` и в `summaries`; не находит по чужим встречам; `ts_headline` возвращает сниппет |
| I12 | `SearchMeetings` — пустой результат | пустой slice, без ошибки |
| I13 | `GetTranscriptForUser` — чужая встреча | `domain.ErrNotFound` |
| I14 | `ChatRepo.Save` + `HistoryChatMessages` | сохраняет и читает обратно; `user_id` согласован с `meeting_id` |
| I15 | `ResetStaleProcessing` | `processing → created`, возвращает `meeting_id`; `status_history` **не** пишется (это восстановление, не бизнес-переход) |
| I16 | Транзакция `SaveTranscriptionAndMarkTranscribed` при ошибке `INSERT transcriptions` (например, дубликат `meeting_id`) | `status` не меняется, `status_history` не растёт |

### 3.4. Структура

```
tests/integration/
  main_test.go                TestMain: pool, миграции, TRUNCATE
  helpers_test.go             setup/cleanup, фабрики сущностей
  meeting_repo_test.go        I1–I2, I8–I10
  task_repo_test.go           I3–I7, I15–I16
  search_test.go              I11–I12
  chat_repo_test.go           I13–I14
```

Пакет `integration` с тегом `//go:build integration` — чтобы `go test ./...`
без `-tags=integration` не подхватывал их (дополнительная защита,
поверх `TEST_DATABASE_URL`).

Запуск:

```bash
make test-integration
# = TEST_DATABASE_URL=... go test -tags=integration -race ./tests/integration/...
```

---

## 4. Конкурентность: race, timeout, cancellation, лимит

### 4.1. Race detector

```bash
go test -race ./...
make test-race
```

`make test-race` — обязательный шаг перед коммитом.

### 4.2. Тест на лимит параллелизма

`internal/application/worker_pool_test.go`:

```go
func TestWorkerPool_LimitConcurrency(t *testing.T) {
    const limit = 3
    var inFlight atomic.Int32
    var maxSeen atomic.Int32

    process := func(ctx context.Context, taskID int64) error {
        cur := inFlight.Add(1)
        defer inFlight.Add(-1)
        for {
            old := maxSeen.Load()
            if cur <= old || maxSeen.CompareAndSwap(old, cur) {
                break
            }
        }
        time.Sleep(20 * time.Millisecond)
        return nil
    }

    pool := application.NewWorkerPool(limit, 100, process, slog.Default())
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    pool.Start(ctx)

    for i := int64(0); i < 10; i++ {
        require.NoError(t, pool.Enqueue(ctx, i))
    }
    time.Sleep(500 * time.Millisecond)
    pool.Stop(ctx)

    require.LessOrEqual(t, maxSeen.Load(), int32(limit))
    require.Greater(t, maxSeen.Load(), int32(1), "должны были работать параллельно")
}
```

### 4.3. Тест на отмену контекста

`internal/infra/speech/mock_test.go`:

```go
func TestMockSpeech_RespectsContext(t *testing.T) {
    c := speech.NewMock(5*time.Second, nil) // задержка 5 с
    ctx, cancel := context.WithCancel(context.Background())
    go func() { time.Sleep(50*time.Millisecond); cancel() }()

    _, err := c.Transcribe(ctx, "audio.mp3", bytes.NewReader(nil))
    require.ErrorIs(t, err, context.Canceled)
}
```

### 4.4. Тест на таймаут

Аналогично, с `context.WithTimeout(ctx, 50*time.Millisecond)` и проверкой
`errors.Is(err, context.DeadlineExceeded)`. Дополнительно — что
`MarkFailed` был вызван с сообщением про timeout (через fake `TaskRepo`).

### 4.5. Тест на отсутствие утечек горутин

`worker_pool_test.go`:

```go
func TestWorkerPool_NoGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()
    pool := application.NewWorkerPool(3, 10, noopProcess, slog.Default())
    ctx := context.Background()
    pool.Start(ctx)
    for i := int64(0); i < 5; i++ {
        _ = pool.Enqueue(ctx, i)
    }
    pool.Stop(ctx)
    time.Sleep(50 * time.Millisecond) // время на завершение
    after := runtime.NumGoroutine()
    require.LessOrEqual(t, after, before+1) // допуск на фоновые горутины рантайма
}
```

`pool.Stop(ctx)` должен дождаться `wg.Wait()`; тест ловит случай, когда
воркеры «потерялись» на `select` без `ctx.Done()`.

### 4.6. Graceful shutdown

`cmd/meeting-assistant/main_test.go` (или `tests/integration`):

- Запустить приложение с `signal.NotifyContext`.
- Отправить `Enqueue` 2 задач с задержкой mock-Speech 5 с.
- Послать `SIGTERM` через `cmd.Process.Signal`.
- Проверить: процесс завершился в пределах `ShutdownTimeout + ε`,
  задачи не остались в `processing` (должны стать `failed` с `"shutdown"`),
  `pgxpool` закрыт (по логам).

Тест не обязателен для CI (требует запуска процесса), но полезен как
«доказательство» поведения. Можно сделать `go test -tags=shutdown`.

---

## 5. Чего в тестах **нет** и почему

| Не тестируется | Причина |
|---|---|
| Реальные внешние API (Whisper, OpenRouter) | Не воспроизводимы в CI, зависят от ключей и квот. Проверяются вручную при переключении провайдера. |
| `sqlcgen` изолированно | Генерируется из SQL, тестировать его — тестировать `pgx`. |
| Полный сквозной сценарий `load → list → get` через `os/exec` | Один тест такого типа есть (graceful shutdown), но не как основной способ. Основное — unit + integration. |
| `main.go` | Собирается `go build`; логика вынесена в `application`. |

---

## 6. Makefile-таргеты

```makefile
test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	TEST_DATABASE_URL=$${TEST_DATABASE_URL} go test -tags=integration -race ./tests/integration/...

test-all: test-race test-integration
```

---

## 7. CI (если появится)

Минимальный набор шагов:

1. `go build ./...`
2. `go vet ./...`
3. `sqlc diff` — `sqlcgen` синхронизирован с `schema.sql` + `queries/`.
4. `go test -race ./...`
5. `go test -tags=integration -race ./tests/integration/...` — с сервис-контейнером
   PostgreSQL или с уже поднятой БД в окружении раннера.

Пункты 1–4 — обязательные, 5 — желательно.
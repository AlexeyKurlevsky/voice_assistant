# Архитектура

Документ описывает слои приложения, направление зависимостей и ключевые
сценарии взаимодействия. Цель — отделить бизнес-логику от транспорта (CLI),
БД (PostgreSQL) и внешних API (Speech/LLM).

---

## 1. Слои

```
┌─────────────────────────────────────────────────────────────┐
│  presentation:  cmd/voice-assistant, internal/cli           │
│  Принимает команды. Никакой бизнес-логики.                  │
└──────────────────────────┬──────────────────────────────────┘
                           │ вызывает use cases
┌──────────────────────────▼──────────────────────────────────┐
│  application: internal/application                          │
│  Use cases + порты (интерфейсы). Знает domain, не знает     │
│  infra. Никаких pgx, HTTP-деталей, io.Reader в публичных    │
│  сигнатурах сервисов.                                       │
└──────────┬──────────────────────────────────┬───────────────┘
           │                                  │
┌──────────▼──────────────┐      ┌────────────▼───────────────┐
│  domain: internal/domain │      │  ports (интерфейсы)        │
│  Сущности, статусы,      │      │  SpeechClient, LLMClient,  │
│  доменные ошибки.        │      │  UserRepo, MeetingRepo,    │
│  Без зависимостей.       │      │  TaskRepo, ChatRepo        │
└──────────────────────────┘      └────────────┬───────────────┘
                                               │ реализуется в
┌──────────────────────────────────────────────▼──────────────┐
│  infrastructure: internal/infra                             │
│  postgres — sqlcgen + обёртки-репозитории (pgx/v5);         │
│  speech   — mock, whisper;                                  │
│  llm      — mock, openrouter;                               │
│  worker   — фоновый pipeline обработки задач.               │
└─────────────────────────────────────────────────────────────┘
```

**Правило зависимостей:** только внутрь. `domain` не импортирует ничего
из проекта. `application` импортирует `domain`. `infra` и `cli`
импортируют `application` и `domain`. Обратного импорта нет.

**`sqlcgen` — деталь инфраструктуры.** Сгенерированный код не «протекает»
наружу: `domain` и `application` про него не знают. Обёртки `repo/*.go`
маппят `sqlcgen.*` в доменные типы и обратно.

---

## 2. Структура пакетов

```
cmd/voice-assistant/main.go        сборка приложения, DI, graceful shutdown
internal/
  config/                          загрузка env, валидация
  domain/                          сущности, статусы, доменные ошибки
  application/                     use cases + порты
    ports.go
    user_service.go
    meeting_service.go
    worker_pool.go
  infra/
    postgres/
      schema.sql                   конечное состояние схемы (вход для sqlc)
      sqlc.yaml                    конфиг генератора
      queries/                     исходники SQL-запросов
        users.sql
        meetings.sql
        tasks.sql
        transcriptions.sql
        summaries.sql
      sqlcgen/                     сгенерированный код (коммитится)
      migrations/                  golang-migrate (0001_init.up/down.sql)
      db.go                        package postgres — pgxpool + запуск миграций
      repo/
        pgconv.go                  package repo — toUUID / toPgUUID
        user.go                    package repo — обёртка: sqlcgen → domain
        meeting.go
        task.go
    speech/                        mock.go, factory.go (+ whisper в будущем)
    llm/                           mock.go, openai_compat.go, factory.go
                                   (OpenAI-совместимый клиент, провайдер openrouter)
  worker/
    processor.go                   pipeline: transcribe → summarize → finalize
  cli/
    root.go + по одному файлу на команду
tests/
  integration/                     TEST_DATABASE_URL (без docker)
```

---

## 3. Диаграмма зависимостей

```mermaid
graph TD
    CLI[cli] --> APP[application]
    MAIN[cmd/main] --> CLI
    MAIN --> APP
    MAIN --> INFRA[infra]
    MAIN --> CONFIG[config]
    APP --> DOMAIN[domain]
    INFRA --> APP
    INFRA --> DOMAIN
    APP -. инверсия зависимостей .-> INFRA

    style DOMAIN fill:#e6f3ff
    style APP fill:#fff3e6
    style INFRA fill:#e6ffe6
    style CLI fill:#f3e6ff
```

Стрелка `application -.-> infra` — инверсия: `application` объявляет
интерфейсы, `infra` их реализует, `main.go` связывает их.

---

## 4. Потоки

### 4.1. Регистрация

```mermaid
sequenceDiagram
    actor U as Пользователь
    participant CLI as cli.start
    participant SVC as UserService
    participant R as UserRepo

    U->>CLI: voice-assistant --user alice start
    CLI->>SVC: Register(ctx, "alice")
    SVC->>R: Create(ctx, externalID, tokenHash)
    R-->>SVC: *User
    SVC-->>CLI: *User + token
    CLI-->>U: user_id=<uuid>, token=<hex>
```

`UserRepo.Create` — обычный `INSERT` с `UNIQUE (external_id)` и
`UNIQUE (token_hash)`. Дубликат ловится как `pg`-ошибка `23505` и
маппится в `domain.ErrAlreadyExists` (см. `repo/user.go`).

### 4.2. Загрузка и постановка в очередь

```mermaid
sequenceDiagram
    actor U as Пользователь
    participant CLI as cli.load
    participant MS as MeetingService
    participant FS as UploadDir
    participant MR as MeetingRepo
    participant Q as WorkerPool

    U->>CLI: load ./audio.mp3
    CLI->>MS: Load(ctx, userID, path)
    MS->>MS: validateFile(path)
    MS->>FS: saveCopy(uploadDir/uuid.mp3)
    MS->>MR: CreateWithTask(ctx, meeting) — транзакция
    MR-->>MS: *Task (status=created)
    MS->>Q: Enqueue(ctx, task.ID)
    alt очередь переполнена
        Q-->>MS: ErrQueueFull
        MS->>MR: MarkFailed(ctx, task.ID, "queue full")
        MS-->>CLI: ошибка
    else успех
        Q-->>MS: nil
        MS-->>CLI: meeting_id, status=created
    end
```

### 4.3. Асинхронная обработка

```mermaid
flowchart TD
    Start([Worker pool: N горутин]) --> Take[Take task ID из jobs]
    Take --> GetTask[TaskRepo.GetByID]
    GetTask --> Terminal{IsTerminal?}
    Terminal -->|да| Skip[skip, log]
    Terminal -->|нет| Recover{status == summarized?}
    Recover -->|да| FastComplete[MarkCompleted — recovery]
    Recover -->|нет| Mark[MarkProcessing: created→processing — транзакция]
    Mark --> ST1["ctx, cancel = context.WithTimeout(parent, SpeechTimeout)"]
    ST1 --> Speech[SpeechClient.Transcribe]
    Speech -->|ok| SaveT[SaveTranscriptionAndMarkTranscribed — транзакция]
    Speech -->|err / timeout| Fail[MarkFailed — транзакция]
    SaveT --> ST2["ctx, cancel = context.WithTimeout(parent, LLMTimeout)"]
    ST2 --> LLM[LLMClient.Summarize]
    LLM -->|ok| SaveS[SaveSummaryAndMarkSummarized — транзакция]
    LLM -->|err / timeout| Fail
    SaveS --> Final[MarkCompleted — транзакция]
    Final --> Done([completed])
    Fail --> DoneF([failed])
```

Каждый переход статуса — в транзакции; каждая транзакция включает запись
в `status_history`. Подробности — в [`STATUS_MODEL.md`](STATUS_MODEL.md).

Отмена родительского контекста (SIGTERM/SIGINT) прерывает текущий шаг;
для текущей задачи выполняется `MarkFailed(..., "shutdown")` с отдельным
контекстом (см. `worker.processor.fail`), чтобы пережить уже отменённый
родительский контекст.

### 4.4. Чтение результатов

```mermaid
sequenceDiagram
    actor U as Пользователь
    participant CLI as cli
    participant SVC as Service
    participant R as Repo

    U->>CLI: get 42 / list / status / find / chat
    CLI->>SVC: метод(ctx, userID, ...)
    SVC->>R: запрос с WHERE user_id = $1
    Note over R: репозиторий сам гарантирует<br/>разграничение прав
    R-->>SVC: данные или ErrNotFound
    SVC-->>CLI: DTO
    CLI-->>U: таблица или JSON
```

**Ключевое решение:** проверка прав встроена в репозиторий. Все методы
`MeetingRepo`, принимающие `userID`, добавляют `WHERE m.user_id = $1`.
Даже если сервис забудет проверить — БД не вернёт чужие данные.

---

## 5. Обработка ошибок

```
domain.ErrNotFound        → "meeting 42 not found"        (exit 1)
domain.ErrForbidden       → "meeting 42 not yours"        (exit 1)
domain.ErrInvalidArgument → "invalid argument: ..."       (exit 2)
domain.ErrQueueFull       → "server busy, try later"      (exit 3)
```

- `fmt.Errorf("meeting_service.get: %w", err)` — оборачивание;
- проверки через `errors.Is` / `errors.As`;
- инфраструктурные ошибки → доменные: `pgx.ErrNoRows → domain.ErrNotFound`.

**Маппинг `pgx → domain` — на обёртке `repo/*.go`.** `sqlcgen` возвращает
`pgx.ErrNoRows`; обёртка переводит его в `domain.ErrNotFound`. `application`
и `domain` про `pgx` не знают.

Логи — `log/slog`, JSON. Поля: `op`, `user_id`, `meeting_id`, `task_id`,
`err`, `duration_ms`. **Никогда** не логируются `api_key`, тела ответов API
целиком, содержимое транскрипций.

---

## 6. Конкурентность

- **Worker pool** — `chan uuid.UUID` с буфером `JobQueueCapacity`, `N` горутин-воркеров.
- **Enqueue** — неблокирующий `select` с `default`; если канал полон —
  `ErrQueueFull` (без создания горутин).
- **Backpressure** — через переполнение очереди, явная ошибка пользователю.
- **Per-task context** — `context.WithTimeout` на каждый шаг (`SpeechTimeout`, `LLMTimeout`).
- **Graceful shutdown** — `signal.NotifyContext` → отмена корневого →
  воркеры дочитывают канал → `close(jobs)` → `wg.Wait()` с дедлайном
  `ShutdownTimeout`. Контекст воркеров отменяется **после** `wg.Wait`
  (нормальный сценарий) либо принудительно, если `ShutdownTimeout` истёк.
  Затем — `pgxpool.Close()`.
- **Recovery** — `ResetStaleProcessing` + `ListPending` на старте команды
  `load` (единственной, которая инициирует новую работу). Read-only команды
  (`start`, `list`, `status`, `get`) recovery не запускают.

Проверки качества:
- `go test -race ./...`;
- тест на `context.WithCancel` во время `Transcribe`;
- тест на лимит параллелизма (атомик-счётчик в mock-Speech).

---

## 7. Транзакции

| Операция | Где | Атомарно |
|---|---|---|
| `CreateWithTask` | `repo/meeting.go` | `INSERT meetings` + `INSERT processing_tasks(status=created)` + `INSERT status_history(status=created)` |
| `MarkProcessing` | `repo/task.go` | `UPDATE status='processing', attempts+1` + `INSERT status_history(status=processing)` |
| `SaveTranscriptionAndMarkTranscribed` | `repo/task.go` | `INSERT transcriptions` + `UPDATE status='transcribed'` + `INSERT status_history(status=transcribed)` |
| `SaveSummaryAndMarkSummarized` | `repo/task.go` | `INSERT summaries` + `UPDATE status='summarized'` + `INSERT status_history(status=summarized)` |
| `MarkCompleted` | `repo/task.go` | `UPDATE status='completed'` + `INSERT status_history(status=completed)` |
| `MarkFailed` | `repo/task.go` | `UPDATE status='failed', error_message=$1` + `INSERT status_history(status=failed)` |

Реализация — `pgxpool.Begin` → `queries.WithTx(tx)` → `tx.Commit` /
`defer tx.Rollback(ctx)`. Все запросы внутри транзакции идут через
`sqlcgen.Queries`, привязанный к транзакции.

---

## 8. Фабрика клиентов по конфигу

```go
speechClient, err := speech.New(cfg.SpeechProvider, speech.Options{
    MockDelay: cfg.MockSpeechDelay,
})
llmClient, err := llm.New(cfg.LLMProvider, llm.Options{
    MockDelay: cfg.MockLLMDelay,
    BaseURL:   cfg.LLMBaseURL,
    APIKey:    cfg.LLMAPIKey,
    Model:     cfg.LLMModel,
})
```

Фабрики в `infra/*/factory.go`. Замена реализаций — только env:
`MS_SPEECH_PROVIDER=whisper` (план) или `MS_LLM_PROVIDER=openrouter`
(реализовано, модель через `MS_OPENROUTER_MODEL`). Бизнес-логика не меняется.

---

## 9. Генерация кода БД через sqlc

Слой доступа к данным (`internal/infra/postgres`) использует `sqlc` как
**dev-time генератор**: SQL-запросы пишутся вручную в `queries/*.sql`,
`sqlc generate` компилирует их в типобезопасный Go-код в `sqlcgen/`.
В рантайме новых зависимостей нет — `sqlcgen` вызывает тот же `pgx/v5`.

### 9.1. Что где живёт

| Файл / каталог | Роль | Руками править? |
|---|---|---|
| `schema.sql` | Конечное состояние схемы (парсится sqlc) | Да, синхронизируется с `migrations/` |
| `queries/*.sql` | Источник истины для запросов | Да |
| `sqlc.yaml` | Конфиг генератора | Да |
| `sqlcgen/*.go` | Сгенерированный код | **Нет** (перезаписывается) |
| `repo/*.go` | Обёртки: маппинг `sqlcgen` → `domain`, ошибки | Да |

### 9.2. Правила

1. **`domain` и `application` не видят `sqlcgen`.** Обёртки в `postgres/repo/*.go`
   (пакет `repo`) принимают/возвращают доменные типы; `sqlcgen` — деталь
   реализации инфраструктуры. Инверсия зависимостей сохраняется.
2. **Маппинг ошибок — на обёртке.** `pgx.ErrNoRows → domain.ErrNotFound`,
   остальные — оборачиваются через `fmt.Errorf("...: %w", err)`.
3. **Транзакции — через `queries.WithTx(tx)`.** Один `pgxpool.Begin`
   на бизнес-операцию, внутри — все `INSERT`/`UPDATE` через `qtx`.
4. **`sqlcgen/` коммитится.** Это позволяет собирать проект без установки
   `sqlc`; CI проверяет синхронизацию через `sqlc diff`.
5. **`schema.sql` обновляется вместе с миграцией.** При добавлении
   миграции `0002_*.sql` — дописать соответствующие `CREATE`/`ALTER`
   в `schema.sql`, иначе `sqlc` сгенерирует код под устаревшую схему.

### 9.3. Конфигурация `sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "schema.sql"
    queries: "queries"
    gen:
      go:
        package: "sqlcgen"
        out: "sqlcgen"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
        emit_empty_slices: true
        overrides:
          - db_type: "pg_catalog.timestamptz"
            go_type:
              import: "time"
              type: "Time"
          - db_type: "timestamptz"
            go_type:
              import: "time"
              type: "Time"
          - db_type: "pg_catalog.timestamptz"
            nullable: true
            go_type:
              import: "time"
              type: "Time"
              pointer: true
          - db_type: "timestamptz"
            nullable: true
            go_type:
              import: "time"
              type: "Time"
              pointer: true
```

- `sql_package: "pgx/v5"` — генерируется код поверх `pgx`, а не `database/sql`.
- `emit_pointers_for_null_types: true` — `NULL`-колонки становятся `*T`.
- `overrides` для `timestamptz` — `NOT NULL` колонки дают `time.Time`,
  nullable — `*time.Time`. Без overrides `sqlc` генерирует `pgtype.Timestamptz`,
  что заставляет делать `.Time` в маппинге.
- `emit_empty_slices: true` — `:many` возвращает `[]T{}`, а не `nil`.
- `emit_interface` — **не включаем**: `Querier` не нужен, unit-тесты мокают
  `application.*Repo`, а не `sqlcgen`.

### 9.4. Что остаётся ручным `pgxpool`

- Запрос `find` с `ts_headline` — гибридно: если `sqlc` справится с типами
  без overrides, оставляем в `queries/meetings.sql`; если нет — метод
  `Search` в `repo/meeting.go` пишется напрямую через `pgxpool.Query`.
- Любые будущие динамические фильтры (`list --status=...`) — `sqlc`
  не собирает SQL на лету.

# Meeting Assistant

Backend-сервис для загрузки, асинхронной обработки и поиска материалов встреч.
Принимает аудио-/тестовый файл, получает транскрипцию через speech-клиент,
краткую выжимку через LLM-клиент, сохраняет результаты в PostgreSQL и позволяет
искать по ним и задавать уточняющие вопросы.

Демонстрирует инженерные практики уровня Middle Go-разработчика:
чистое разделение слоёв, заменяемость внешних интеграций, конкурентную
обработку с ограничением, транзакции, context-aware таймауты, graceful
shutdown, структурированное логирование, unit- и интеграционные тесты.

---

## Возможности

| Команда | Назначение |
|---|---|
| `start` | Регистрация пользователя по `--user` |
| `load <path>` | Загрузка файла встречи и постановка в очередь |
| `list` | Список встреч пользователя со статусом и выжимкой |
| `status <id>` | Детальный статус обработки (даты, ошибка) |
| `get <id>` | Полная транскрипция встречи |
| `find <keyword>` | Полнотекстовый поиск по встречам пользователя |
| `chat <id> <text>` | Вопрос к LLM по материалам одной встречи |

Дополнительно:
- ограниченная очередь с `N` воркерами;
- таймауты на каждый шаг обработки;
- graceful shutdown с корректной остановкой фоновых задач;
- разграничение данных: пользователь видит только свои встречи;
- смена провайдера speech/LLM через env без изменения бизнес-логики.

---

## Модель безопасности

Аутентификация — по session-токену:

```bash
# первый раз: получить токен (показывается один раз)
$ voice-assistant start --user alice
user_id: ...
token: 7c3aed...

# сохранить в файл (рекомендуется chmod 600)
$ echo "7c3aed..." > ~/.voice-assistant-token
$ chmod 600 ~/.voice-assistant-token

# дальше — без флагов, токен подхватится автоматически
$ voice-assistant list

# или явно
$ voice-assistant --token 7c3aed... list
```

## Статусы обработки

```
created ──► processing ──► transcribed ──► summarized ──► completed
                │
                └──► failed
```

| Статус | Что означает | Кто переводит |
|---|---|---|
| `created` | Встреча и задача созданы в одной транзакции | `MeetingService.Load` |
| `processing` | Воркер взял задачу | `WorkerPool` |
| `transcribed` | Транскрипция сохранена в БД | `WorkerPool` после `SpeechClient.Transcribe` |
| `summarized` | Краткая выжимка сохранена в БД | `WorkerPool` после `LLMClient.Summarize` |
| `completed` | Все материалы консистентны, встреча доступна для `get`/`find`/`chat` | `WorkerPool` финальным `UPDATE` |
| `failed` | Ошибка на любом шаге; текст в `error_message` | `WorkerPool` при `err` |

Полное описание модели — в [`docs/STATUS_MODEL.md`](docs/STATUS_MODEL.md).

---

## Быстрый старт

Требования: Go 1.26+ (см. `go.mod`), PostgreSQL 14+.

```bash
git clone <repo> && cd voice-assistant
cp .env.example .env                       # отредактировать MS_DB_DSN
make build

createdb voice_assistant
make migrate-up

# регистрация: токен показывается один раз
./bin/voice-assistant --user alice start
# сохраните token из вывода
./bin/voice-assistant --token "$TOKEN" load ./testdata/sample.txt
./bin/voice-assistant --token "$TOKEN" list
./bin/voice-assistant --token "$TOKEN" status <meeting_id>
./bin/voice-assistant --token "$TOKEN" get <meeting_id>
./bin/voice-assistant --token "$TOKEN" find "бюджет"
./bin/voice-assistant --token "$TOKEN" chat <meeting_id> "какие сроки обсудили?"
```

### Пример вывода

```bash
$ voice-assistant --token "$TOKEN" list
ID          CREATED              STATUS      SUMMARY
3a2b5c7d   2026-11-25 10:12:03  completed   Обсудили бюджет Q1, сроки релиза...
5f8d9e1a   2026-11-25 10:14:41  processing  —
```

```bash
$ voice-assistant --token "$TOKEN" status 5f8d9e1a
meeting_id:       5f8d9e1a
status:           failed
created_at:       2026-11-25 10:15:02
updated_at:       2026-11-25 10:15:09
error:            speech: mock failure (as requested)
```

```bash
$ voice-assistant --token "$TOKEN" find "бюджет"
ID          CREATED              STATUS      SNIPPET
3a2b5c7d   2026-11-25 10:12:03  completed   ...обсудили <бюджет> Q1 и сроки...
```

---

## Документация

| Документ | Содержание |
|---|---|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Слои, зависимости, потоки, ошибки, конкурентность, sqlc |
| [`docs/STATUS_MODEL.md`](docs/STATUS_MODEL.md) | Модель статусов, гарантии переходов |
| [`docs/DATA_MODEL.md`](docs/DATA_MODEL.md) | ER-схема, индексы, транзакции, связь с sqlc |
| [`docs/COMMANDS.md`](docs/COMMANDS.md) | Контракт CLI, коды возврата |
| [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md) | Переменные окружения |
| [`docs/TESTING.md`](docs/TESTING.md) | Unit / integration / race |
| [`docs/DECISIONS.md`](docs/DECISIONS.md) | ADR по ключевым решениям |
| [`docs/DEMO.md`](docs/DEMO.md) | Сценарий демонстрации |

---

## Технологический стек

| Слой | Технология |
|---|---|
| Язык | Go 1.23+ |
| CLI | `spf13/cobra` |
| База | PostgreSQL 14+ |
| Драйвер | `jackc/pgx/v5` + `pgxpool` |
| Генерация кода БД | `sqlc` (dev-time, поверх `pgx/v5`) |
| Миграции | `golang-migrate/migrate` + `embed.FS` |
| Логи | `log/slog` (stdlib) |
| Тесты | `stretchr/testify` |
| Speech | `mock` (по умолчанию), `whisper` (OpenAI-совместимый, план) |
| LLM | `mock` (по умолчанию), `openrouter` (OpenAI-совместимый, реализован) |

---

## Makefile

```
make build              # сборка ./bin/voice-assistant
make run                # go run ./cmd/voice-assistant
make test               # unit-тесты
make test-race          # go test -race ./...
make migrate-up         # golang-migrate up
make migrate-down       # golang-migrate down
make sqlc               # sqlc generate
make sqlc-check         # sqlc diff (падает, если sqlcgen устарел)
make lint               # go vet
```

---

## Лицензия

MIT.

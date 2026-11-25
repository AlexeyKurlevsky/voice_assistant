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

Требования: Go 1.23+, PostgreSQL 14+.

```bash
git clone <repo> && cd meeting-assistant
cp .env.example .env                       # отредактировать MS_DB_DSN
make build

createdb ms
createdb ms_test                           # для интеграционных тестов
make migrate-up

./bin/meeting-assistant --user alice start
./bin/meeting-assistant --user alice load ./testdata/sample.mp3
./bin/meeting-assistant --user alice list
./bin/meeting-assistant --user alice status 1
./bin/meeting-assistant --user alice get 1
./bin/meeting-assistant --user alice find "бюджет"
./bin/meeting-assistant --user alice chat 1 "какие сроки обсудили?"
```

### Пример вывода

```bash
$ meeting-assistant --user alice list
ID   CREATED              STATUS       SUMMARY
1    2026-11-19 10:12:03  completed    Обсудили бюджет Q1, сроки релиза...
2    2026-11-19 10:14:41  processing   —
3    2026-11-19 10:15:02  failed       —
```

```bash
$ meeting-assistant --user alice status 3
meeting_id:       3
status:           failed
created_at:       2026-11-19 10:15:02
updated_at:       2026-11-19 10:15:09
error:            speech client: upstream 503
```

```bash
$ meeting-assistant --user alice find "бюджет"
ID   CREATED              STATUS       SNIPPET
1    2026-11-19 10:12:03  completed    ...обсудили <бюджет> Q1 и сроки...
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
| Speech | `mock` (по умолчанию), `whisper` (OpenAI-совместимый) |
| LLM | `mock` (по умолчанию), `openrouter` (опционально) |

---

## Makefile

```
make build              # сборка ./bin/meeting-assistant
make run                # go run ./cmd/meeting-assistant
make test               # unit-тесты
make test-race          # go test -race ./...
make test-integration   # с TEST_DATABASE_URL
make migrate-up         # golang-migrate up
make migrate-down       # golang-migrate down
make sqlc               # sqlc generate
make sqlc-check         # sqlc diff (падает, если sqlcgen устарел)
make lint               # go vet + staticcheck
```

---

## Лицензия

MIT.
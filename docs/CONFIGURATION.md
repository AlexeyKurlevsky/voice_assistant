# Конфигурация

Документ описывает переменные окружения, их дефолты, валидацию и порядок
загрузки.

---

## 1. Порядок загрузки

1. Значения по умолчанию (в коде `config.Load`).
2. Файл `.env` (или путь из `--config`), если существует.
3. Переменные окружения процесса — **перекрывают** `.env`.

Это стандартный приоритет: удобно держать дефолты в `.env` для разработки,
а в CI/проде передавать значения через env без изменения файла.

Для загрузки `.env` используется `github.com/joho/godotenv` (не перезаписывает
уже установленные переменные). Логика загрузки инкапсулирована в `config`:
`config.Load()` читает `.env` из текущей директории; тесты используют
`config.LoadWithOptions(LoadOptions{EnvFile: ""})` — только `os.Getenv`,
без файловой системы.
---

## 2. Переменные

### 2.1. База данных

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `MS_DB_DSN` | **да** | — | DSN PostgreSQL, например `postgres://ms:ms@localhost:5432/ms?sslmode=disable`. |
| `MS_DB_MAX_CONNS` | нет | `10` | Максимум соединений в `pgxpool`. |
| `MS_MIGRATE_ON_START` | нет | `true` | Применять миграции при старте `main.go`. В проде — `false`. |

Если `MS_DB_DSN` пуст — `config.Load` возвращает ошибку, приложение
не стартует.

### 2.2. Файлы

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `MS_UPLOAD_DIR` | нет | `./data/uploads` | Каталог для сохранённых копий файлов. Создаётся при старте (`MkdirAll`). |
| `MS_MAX_UPLOAD_BYTES` | нет | `104857600` (100 MiB) | Максимальный размер загружаемого файла. |

### 2.3. Обработка

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `MS_WORKER_CONCURRENCY` | нет | `3` | Число воркеров-горутин. Должно быть > 0. |
| `MS_JOB_QUEUE_CAPACITY` | нет | `100` | Ёмкость канала `jobs`. Должно быть > 0. |
| `MS_SHUTDOWN_TIMEOUT` | нет | `10s` | Максимальное время на graceful shutdown. |

**Связь `WorkerConcurrency` и `JobQueueCapacity`.** Общее число «в полёте»
задач — `WorkerConcurrency + JobQueueCapacity`. При дефолтах — 103.
Если `load` вызывается чаще, пользователь получает `ErrQueueFull`
(exit 3), а задача помечается `failed` с `error_message="queue full"`.

### 2.4. Таймауты

| Переменная | По умолчанию | Описание |
|---|---|---|
| `MS_HTTP_TIMEOUT` | `60s` | Общий таймаут HTTP-клиента (dial + TLS + чтение тела). |
| `MS_SPEECH_TIMEOUT` | `120s` | Таймаут на один вызов `SpeechClient.Transcribe`. |
| `MS_LLM_TIMEOUT` | `60s` | Таймаут на один вызов `LLMClient.Summarize` / `Answer`. |

Внешние таймауты оборачивают внутренние: `MS_HTTP_TIMEOUT < MS_SPEECH_TIMEOUT`.
При нарушении этого правила `config.Load` пишет предупреждение в лог,
но не падает (HTTP-таймаут всё равно сработает первым).

### 2.5. Провайдеры

| Переменная | Обязательно | По умолчанию | Допустимые значения |
|---|---|---|---|
| `MS_SPEECH_PROVIDER` | нет | `mock` | `mock`, `whisper` |
| `MS_LLM_PROVIDER` | нет | `mock` | `mock`, `openrouter` |

#### `MS_SPEECH_PROVIDER=mock`

Не требует ключей. Возвращает заранее подготовленную транскрипцию:
- для `.txt` — содержимое файла как есть;
- для `.json` — поле `"transcription"` из JSON;
- для аудио — фиксированная заглушка с именем файла.

Дополнительная задержка эмулирует реальный вызов:
`MS_MOCK_SPEECH_DELAY` (по умолчанию `2s`). Нужна, чтобы `status` успевал
показать промежуточные состояния.

#### `MS_SPEECH_PROVIDER=whisper`

| Переменная | Обязательно при `whisper` | Описание |
|---|---|---|
| `MS_WHISPER_BASE_URL` | да | Базовый URL OpenAI-совместимого API. Пример: `https://api.openai.com/v1`, `https://api.groq.com/openai/v1`, `http://localhost:8080/v1`. |
| `MS_WHISPER_API_KEY` | да | Bearer-токен. |
| `MS_WHISPER_MODEL` | нет (по умолчанию `whisper-1`) | Имя модели. |

Если `MS_SPEECH_PROVIDER=whisper`, а `MS_WHISPER_BASE_URL` или
`MS_WHISPER_API_KEY` пусты — `config.Load` возвращает ошибку.

#### `MS_LLM_PROVIDER=mock`

Не требует ключей. Для `Summarize` возвращает фиксированную выжимку
на основе первых 200 символов транскрипции. Для `Answer` — фиксированный
ответ с эхом вопроса. Задержка — `MS_MOCK_LLM_DELAY` (по умолчанию `1s`).

#### `MS_LLM_PROVIDER=openrouter`

| Переменная | Обязательно при `openrouter` | Описание |
|---|---|---|
| `MS_OPENROUTER_BASE_URL` | да | По умолчанию `https://openrouter.ai/api/v1`. |
| `MS_OPENROUTER_API_KEY` | да | Ключ. |
| `MS_OPENROUTER_MODEL` | да | Например `anthropic/claude-3.5-sonnet`. |

Если ключ или модель пусты — `config.Load` возвращает ошибку.

### 2.6. Логирование

| Переменная | По умолчанию | Допустимые значения |
|---|---|---|
| `MS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `MS_LOG_FORMAT` | `json` | `json`, `text` |

`text` удобен для локальной разработки, `json` — для сбора логов.

**Что не логируется никогда:** `MS_WHISPER_API_KEY`, `MS_OPENROUTER_API_KEY`,
содержимое транскрипций, тела ответов внешних API.

---

### 2.7. Аутентификация

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `MS_TOKEN` | нет | — | Session-токен. Если не задан, CLI читает `~/.voice-assistant-token` |
| `--token` (флаг) | — | из `MS_TOKEN` / файла | Переопределяет все источники |

Порядок источников: `--token` → `MS_TOKEN` → `~/.voice-assistant-token` → пусто.

`start --user alice` не требует токена. `rotate` требует (для подтверждения).
Все остальные команды — обязателен.

## 3. Валидация

`config.Load` проверяет:

| Условие | Результат |
|---|---|
| `MS_DB_DSN` пуст | ошибка |
| `MS_WORKER_CONCURRENCY <= 0` | ошибка |
| `MS_JOB_QUEUE_CAPACITY <= 0` | ошибка |
| `MS_SPEECH_PROVIDER` ∉ {mock, whisper} | ошибка |
| `MS_LLM_PROVIDER` ∉ {mock, openrouter} | ошибка |
| `speech=whisper`, но ключ/URL пусты | ошибка |
| `llm=openrouter`, но ключ/модель пусты | ошибка |
| `MS_LOG_LEVEL` ∉ {debug, info, warn, error} | ошибка |
| `MS_HTTP_TIMEOUT >= MS_SPEECH_TIMEOUT` | предупреждение в лог |

При любой ошибке — сообщение `config: <detail>` и выход с кодом 3.

---

## 4. `.env.example`

```dotenv
# --- База данных ---
MS_DB_DSN=postgres://ms:ms@localhost:5432/ms?sslmode=disable
MS_DB_MAX_CONNS=10
MS_MIGRATE_ON_START=true

# --- Файлы ---
MS_UPLOAD_DIR=./data/uploads
MS_MAX_UPLOAD_BYTES=104857600

# --- Обработка ---
MS_WORKER_CONCURRENCY=3
MS_JOB_QUEUE_CAPACITY=100
MS_SHUTDOWN_TIMEOUT=10s

# --- Таймауты ---
MS_HTTP_TIMEOUT=60s
MS_SPEECH_TIMEOUT=120s
MS_LLM_TIMEOUT=60s

# --- Speech ---
MS_SPEECH_PROVIDER=mock
MS_MOCK_SPEECH_DELAY=2s
# MS_WHISPER_BASE_URL=https://api.groq.com/openai/v1
# MS_WHISPER_API_KEY=...
# MS_WHISPER_MODEL=whisper-large-v3

# --- LLM ---
MS_LLM_PROVIDER=mock
MS_MOCK_LLM_DELAY=1s
# MS_OPENROUTER_BASE_URL=https://openrouter.ai/api/v1
# MS_OPENROUTER_API_KEY=...
# MS_OPENROUTER_MODEL=anthropic/claude-3.5-sonnet

# --- Логи ---
MS_LOG_LEVEL=info
MS_LOG_FORMAT=text
```

Закомментированные блоки — примеры реальных провайдеров. Для переключения
нужно раскомментировать три строки и поменять `MS_*_PROVIDER`.

---

## 5. Конфигурация в тестах

Unit-тесты не читают env: `config` передаётся в сервисы как структура,
тесты создают её вручную.

Интеграционные тесты читают только `TEST_DATABASE_URL`:

```bash
export TEST_DATABASE_URL="postgres://ms:ms@localhost:5432/ms_test?sslmode=disable"
make test-integration
```

Если переменная не установлена — интеграционные тесты делают `t.Skip`.
Это позволяет гонять `go test ./...` без подготовки БД.

---

## 6. Что заменить для перехода на реальные провайдеры

1. В `.env` установить `MS_SPEECH_PROVIDER=whisper` и три `MS_WHISPER_*`.
2. Установить `MS_LLM_PROVIDER=openrouter` и три `MS_OPENROUTER_*`.
3. Перезапустить приложение.

Бизнес-логика, миграции, CLI и тесты не меняются — это прямое следствие
инверсии зависимостей (`application/ports.go`).
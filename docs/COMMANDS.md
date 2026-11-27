# CLI-команды

Документ фиксирует контракт CLI: синтаксис, флаги, формат вывода, коды
возврата и поведение при ошибках. Бинарник называется `voice-assistant`.

Конфигурация загружается из `.env` и переменных окружения (см.
[`CONFIGURATION.md`](CONFIGURATION.md)); отдельного флага `--config` нет.

---

## 1. Аутентификация

`start` создаёт пользователя и **один раз** показывает session-токен
(64 hex-символа). В базе хранится только SHA-256 токена.

Все остальные команды требуют токен. Источники токена (по убыванию
приоритета):

1. флаг `--token <hex>`;
2. переменная окружения `MS_TOKEN`;
3. файл `~/.voice-assistant-token` (рекомендуется `chmod 600`).

Если токен пуст — ошибка `--token is required` (exit 2).

Флаг `--user <external_id>` используется **только** командой `start`.
`external_id` валидируется: непустой, длина ≤ 64, символы `[a-zA-Z0-9_.-]`.

---

## 2. Общие флаги и коды возврата

| Флаг | Команды | Назначение |
|---|---|---|
| `--user <id>` | `start` | `external_id` пользователя. |
| `--token <hex>` | все, кроме `start` | Session-токен. |
| `--json` | все | Вывод в JSON. |

Коды возврата:

| Код | Значение |
|---|---|
| `0` | Успех. |
| `1` | Бизнес-ошибка: не найдено / недоступно (`not found`, `forbidden`). |
| `2` | Ошибка аргументов: невалидный `--user`, пустой `--token`, невалидный `meeting_id`, файл не найден, неподдерживаемое расширение. |
| `3` | Инфраструктурная ошибка и всё остальное: недоступна БД, очередь переполнена, таймаут, неизвестная команда, повторный `start` с занятым именем. |

---

## 3. Команды

### 3.1. `start`

Регистрация + выдача токена. Токен показывается **один раз**.

```bash
voice-assistant --user <external_id> start
```

**Вывод (text):**

```
user_id: 3a2b5c7d-...
external_id: alice
created_at: 2026-11-25T15:00:00+03:00

token: 7c3aed...

Сохраните токен — он показывается один раз.
Дальше: --token <token> или MS_TOKEN=<token> или ~/.voice-assistant-token.
```

**Вывод (--json):**

```json
{"user_id":"3a2b5c7d-...","external_id":"alice","token":"7c3aed...","created_at":"2026-11-25T15:00:00+03:00"}
```

**Поведение:** при первом вызове создаёт пользователя. Повторный `start`
с тем же `--user` возвращает ошибку `user "alice" already exists` (exit 3).
Для перевыпуска токена используйте `rotate`; сброс при утере — только вручную
через `psql` или пересоздание БД.

### 3.2. `rotate`

Перевыпуск токена. Требует действующий `--token` (старый инвалидируется).

```bash
voice-assistant --token <old-token> rotate
```

### 3.3. `load <path>`

Загрузка файла и постановка задачи в очередь.

```bash
voice-assistant --token <token> load <path>
```

Поддерживаемые расширения: `.mp3`, `.wav`, `.m4a`, `.ogg`, `.flac`,
`.txt`, `.json`. `.txt` и `.json` — для тестового режима `mock`-Speech.

**Что делает:**

1. Валидирует файл: существует, не каталог, размер ≤ `MS_MAX_UPLOAD_BYTES`
   (по умолчанию 100 MiB), расширение в списке.
2. Копирует в `MS_UPLOAD_DIR/<uuid>.<ext>`.
3. В транзакции: `INSERT meetings` + `INSERT processing_tasks(status='created')`
   + `INSERT status_history`.
4. `Enqueue` — если очередь полна, помечает задачу `failed` и возвращает ошибку.
5. Печатает `meeting_id` (UUID) и текущий статус.

Если файл с тем же содержимым (SHA-256) уже загружен этим пользователем —
новая встреча не создаётся, выводится существующая с пометкой
`already loaded`.

**Вывод:**

```
meeting_id: 3a2b5c7d-...
status: created
```

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Файл не найден | 2 | `invalid argument: file not found: ./nope.mp3` |
| Расширение не поддерживается | 2 | `invalid argument: unsupported file type: .pdf` |
| Файл слишком большой | 2 | `invalid argument: file too large: ...` |
| Очередь переполнена | 3 | `processing queue is full, try again later` |
| БД недоступна | 3 | обёрнутая ошибка БД |

### 3.4. `list`

Список встреч текущего пользователя.

```bash
voice-assistant --token <token> list [--limit N] [--offset M]
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--limit` | `50` | Максимум записей (нормализуется к 50 при `<=0` или `>500`). |
| `--offset` | `0` | Смещение (отрицательное нормализуется к 0). |

**Вывод (text):**

```
ID         CREATED              STATUS     SUMMARY
3a2b5c7d   2026-11-25 15:00:00  completed  Краткая выжимка...
```

`SUMMARY` — первые 60 символов выжимки; если её нет — `—`.

### 3.5. `status <id>`

Детальный статус обработки встречи.

```bash
voice-assistant --token <token> status <meeting_id> [--verbose] [--watch [--interval D]]
```

**Вывод (text):**

```
meeting_id: 3a2b5c7d-...
status: failed
created_at: 2026-11-25 15:00:02
updated_at: 2026-11-25 15:00:09
error: speech: mock failure (as requested)
```

`--verbose` добавляет историю переходов `status_history`;
`--watch` опрашивает статус до терминального состояния (интервал по
умолчанию 2s).

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Встреча не найдена / чужая | 1 | `meeting <id> not found` |
| Невалидный `meeting_id` | 2 | `invalid argument: invalid meeting id` |

### 3.6. `get <id>`

Полная транскрипция встречи.

```bash
voice-assistant --token <token> get <meeting_id>
```

**Вывод (text):** транскрипция целиком. **(--json):**

```json
{"meeting_id":"3a2b5c7d-...","transcription":"...полный текст..."}
```

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Встреча не найдена / чужая | 1 | `meeting <id> not found` |
| Транскрипции ещё нет | 1 | `meeting <id> not found (or transcription not ready)` |

### 3.7. `find <keyword>`

Полнотекстовый поиск по встречам текущего пользователя.

```bash
voice-assistant --token <token> find <keyword>
```

`<keyword>` — одна или несколько фраз (объединяются в один запрос).
Синтаксис `websearch_to_tsquery`: `OR`, `-слово`, `"точная фраза"`.

**Вывод (text):**

```
ID         CREATED              STATUS     SNIPPET
3a2b5c7d   2026-11-25 15:00:02  completed  ...обсудили <бюджет> Q1...
```

Пустой keyword → exit 2. Нет результатов — пустая таблица, exit 0.

### 3.8. `chat <id> <text>`

Вопрос к LLM по материалам одной встречи.

```bash
voice-assistant --token <token> chat <meeting_id> <text>
```

Требует статус встречи `completed`. Контекст для LLM — выжимка (если есть)
+ транскрипция; пара вопрос-ответ сохраняется в `chat_messages`.

**Вывод (text):**

```
Q: какие сроки обсудили?
A: На встрече обсудили сроки релиза — конец Q1.
```

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Встреча не найдена / чужая | 1 | `meeting <id> not found` |
| Статус не `completed` | 2 | `invalid argument: meeting not ready for questions: status=...` |
| LLM недоступен / таймаут | 3 | обёрнутая ошибка LLM |

---

## 4. Сводная таблица

| Команда | Аргументы | Флаги |
|---|---|---|
| `start` | — | `--user`, `--json` |
| `rotate` | — | `--json` |
| `load` | `<path>` | `--json` |
| `list` | — | `--limit`, `--offset`, `--json` |
| `status` | `<id>` | `--verbose`, `--watch`, `--interval`, `--json` |
| `get` | `<id>` | `--json` |
| `find` | `<keyword>` | `--json` |
| `chat` | `<id> <text>` | `--json` |

---

## 5. Пример полного сценария

```bash
export MS_DB_DSN="postgres://ms:ms@localhost:5432/voice_assistant?sslmode=disable"

# регистрация: токен показывается один раз
voice-assistant --user alice start
# token: 7c3aed...

TOKEN=7c3aed...

voice-assistant --token "$TOKEN" load ./testdata/sample.txt
# meeting_id: 3a2b5c7d-...
# status: created

voice-assistant --token "$TOKEN" status 3a2b5c7d-...
# status: processing → ... → completed

voice-assistant --token "$TOKEN" list
voice-assistant --token "$TOKEN" get 3a2b5c7d-...
voice-assistant --token "$TOKEN" find "бюджет"
voice-assistant --token "$TOKEN" chat 3a2b5c7d-... "какие сроки обсудили?"

# чужой пользователь не видит встречу
voice-assistant --user bob start
voice-assistant --token "$BOB_TOKEN" status 3a2b5c7d-...
# meeting 3a2b5c7d-... not found
# exit 1
```

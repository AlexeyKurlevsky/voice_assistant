# CLI-команды

Документ фиксирует контракт CLI: синтаксис, флаги, формат вывода, коды
возврата и поведение при ошибках.

---

## 1. Общие флаги

Флаги корневой команды — общие для всех подкоманд:

| Флаг | Обязательный | По умолчанию | Назначение |
|---|---|---|---|
| `--user <id>` | да | — | `external_id` пользователя. При первом использовании пользователь создаётся автоматически. |
| `--json` | нет | `false` | Вывод в JSON вместо таблицы. |
| `--verbose` | нет | `false` | Дополнительный вывод (для `status` — история переходов). |
| `--config <path>` | нет | `./.env` | Путь к файлу конфигурации. |
| `--help` / `-h` | — | — | Справка. |

`--user` валидируется: непустая строка, длина ≤ 64, `[a-zA-Z0-9_.-]`.
Нарушение → exit 2 с сообщением `invalid --user`.

---

## 2. Коды возврата

| Код | Значение |
|---|---|
| `0` | Успех. |
| `1` | Бизнес-ошибка: `not found`, `forbidden`, `failed`-статус обработки. |
| `2` | Ошибка аргументов: неизвестная команда, отсутствует `--user`, невалидный аргумент. |
| `3` | Инфраструктурная ошибка: недоступна БД, очередь переполнена, таймаут. |

Коды стабильны и используются в скриптах и тестах.

---

## 3. Команды

### 3.1. `start`

Регистрация пользователя. Идемпотентна: повторный вызов не создаёт дубликат.

```
meeting-assistant --user <id> start
```

**Вывод (text):**

```
user_id: 1
external_id: alice
created_at: 2026-11-19 10:00:00
```

**Вывод (--json):**

```json
{"user_id":1,"external_id":"alice","created_at":"2026-11-19T10:00:00Z"}
```

**Поведение:** если пользователь уже существует — возвращает его же данные,
exit 0. Реальная регистрация выполняется и при первом вызове любой другой
команды (lazy `EnsureExists`), но `start` — явный способ.

---

### 3.2. `load <path>`

Загрузка файла и постановка задачи в очередь.

```
meeting-assistant --user <id> load <path>
```

| Аргумент | Обязательный | Описание |
|---|---|---|
| `<path>` | да | Путь к локальному файлу. Поддерживаемые расширения: `.mp3`, `.wav`, `.m4a`, `.ogg`, `.flac`, `.txt`, `.json`. |

**Что делает:**
1. Проверяет `--user` (создаёт при необходимости).
2. Валидирует файл: существует, читается, размер ≤ `MS_MAX_UPLOAD_BYTES` (по умолчанию 100 MiB), расширение в списке.
3. Копирует в `MS_UPLOAD_DIR/<uuid>.<ext>`.
4. В транзакции: `INSERT meetings` + `INSERT processing_tasks(status='created')` + `INSERT status_history`.
5. `Enqueue` — если очередь полна, помечает задачу `failed` и возвращает ошибку.
6. Печатает `meeting_id` и текущий статус.

**Вывод:**

```
meeting_id: 42
status: created
```

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Файл не найден | 2 | `file not found: ./nope.mp3` |
| Расширение не поддерживается | 2 | `unsupported file type: .pdf` |
| Файл слишком большой | 2 | `file too large: 250 MiB > 100 MiB` |
| Очередь переполнена | 3 | `server busy: processing queue is full, try again later` |
| БД недоступна | 3 | `database unavailable: ...` |

`txt` и `json` — для тестового режима: mock-Speech возвращает заранее
подготовленную транскрипцию на основе содержимого файла (см. `CONFIGURATION.md`,
`MS_SPEECH_PROVIDER=mock`).

---

### 3.3. `list`

Список встреч текущего пользователя.

```
meeting-assistant --user <id> list [--limit N] [--offset M]
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--limit` | `50` | Максимум записей. Max `500`. |
| `--offset` | `0` | Смещение. |

**Вывод (text):**

```
ID   CREATED              STATUS       SUMMARY
1    2026-11-19 10:12:03  completed    Обсудили бюджет Q1, сроки релиза...
2    2026-11-19 10:14:41  processing   —
3    2026-11-19 10:15:02  failed       —
```

`SUMMARY` — первые 60 символов краткой выжимки. Если её ещё нет — `—`.

**Вывод (--json):**

```json
[
  {"id":1,"title":"sample","status":"completed","summary":"Обсудили бюджет Q1...","created_at":"2026-11-19T10:12:03Z"},
  {"id":2,"title":"daily","status":"processing","summary":"","created_at":"2026-11-19T10:14:41Z"}
]
```

Если встреч нет — пустая таблица (только заголовок) и exit 0.

---

### 3.4. `status <id>`

Детальный статус обработки встречи.

```
meeting-assistant --user <id> status <id> [--verbose]
```

**Вывод (text):**

```
meeting_id: 3
status: failed
created_at: 2026-11-19 10:15:02
updated_at: 2026-11-19 10:15:09
error: speech client: upstream 503
```

При `--verbose` дополнительно:

```
history:
  2026-11-19 10:15:02  (null) -> created
  2026-11-19 10:15:03  created -> processing
  2026-11-19 10:15:09  processing -> failed
```

Поле `error` выводится только при `failed`. Для остальных статусов строка
опускается.

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Встреча не найдена | 1 | `meeting 999 not found` |
| Встреча принадлежит другому | 1 | `meeting 42 not found` (не раскрываем) |

---

### 3.5. `get <id>`

Полная транскрипция встречи.

```
meeting-assistant --user <id> get <id>
```

**Вывод (text):** транскрипция целиком, без обёртки.

**Вывод (--json):**

```json
{"meeting_id":1,"title":"sample","status":"completed","transcription":"...полный текст..."}
```

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Встреча не найдена / чужая | 1 | `meeting 42 not found` |
| Транскрипции ещё нет (статус `created`/`processing`) | 1 | `transcription not ready: status=processing` |

---

### 3.6. `find <keyword>`

Полнотекстовый поиск по встречам текущего пользователя.

```
meeting-assistant --user <id> find <keyword>
```

`<keyword>` — одна или несколько фраз через пробел (объединяются в один
поисковый запрос). Синтаксис `websearch_to_tsquery`: поддерживает
`OR`, `-слово`, `"точная фраза"`.

**Вывод (text):**

```
ID   CREATED              STATUS       SNIPPET
1    2026-11-19 10:12:03  completed    ...обсудили <бюджет> Q1 и сроки...
```

**Вывод (--json):**

```json
[{"id":1,"title":"sample","status":"completed","snippet":"...обсудили <бюджет> Q1 и сроки...","created_at":"2026-11-19T10:12:03Z"}]
```

Если ничего не найдено — пустая таблица, exit 0 (не ошибка).

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Пустой keyword | 2 | `keyword is required` |

---

### 3.7. `chat <id> <text>`

Вопрос к LLM по материалам одной встречи.

```
meeting-assistant --user <id> chat <id> <text>
```

| Аргумент | Обязательный | Описание |
|---|---|---|
| `<id>` | да | `meeting_id`. |
| `<text>` | да | Вопрос. Может содержать пробелы (один аргумент в кавычках). |

**Что делает:**
1. Проверяет, что встреча принадлежит пользователю и её статус — `completed`
   (только по полностью обработанной встрече есть контекст).
2. Берёт транскрипцию (и выжимку, если есть) как контекст.
3. `LLMClient.Answer(ctx, question, contextText)`.
4. `ChatRepo.Save(userID, meetingID, question, answer)`.

**Вывод (text):**

```
Q: какие сроки обсудили?
A: На встрече обсудили сроки релиза — конец Q1, с контрольной точкой в середине февраля.
```

**Вывод (--json):**

```json
{"meeting_id":1,"question":"какие сроки обсудили?","answer":"На встрече обсудили..."}
```

**Ошибки:**

| Ситуация | exit | Сообщение |
|---|---|---|
| Встреча не найдена / чужая | 1 | `meeting 42 not found` |
| Статус не `completed` | 1 | `meeting 42 not ready for questions: status=processing` |
| LLM недоступен | 3 | `llm unavailable: ...` |
| Таймаут LLM | 3 | `llm timeout after 60s` |

---

## 4. Сводная таблица

| Команда | Аргументы | Флаги | Exit 0 | Exit 1 | Exit 2 | Exit 3 |
|---|---|---|---|---|---|---|
| `start` | — | — | ✓ | | | |
| `load` | `<path>` | — | ✓ | | ✓ | ✓ |
| `list` | — | `--limit`, `--offset` | ✓ | | ✓ | ✓ |
| `status` | `<id>` | `--verbose` | ✓ | ✓ | ✓ | ✓ |
| `get` | `<id>` | — | ✓ | ✓ | ✓ | ✓ |
| `find` | `<keyword>` | — | ✓ | | ✓ | ✓ |
| `chat` | `<id> <text>` | — | ✓ | ✓ | ✓ | ✓ |

---

## 5. Пример полного сценария

```bash
export MS_DB_DSN="postgres://ms:ms@localhost:5432/ms?sslmode=disable"

meeting-assistant --user alice start
# user_id: 1
# external_id: alice
# created_at: 2026-11-19 10:00:00

meeting-assistant --user alice load ./testdata/sample.mp3
# meeting_id: 1
# status: created

meeting-assistant --user alice status 1
# status: processing

# через несколько секунд
meeting-assistant --user alice status 1
# status: completed

meeting-assistant --user alice get 1
# <полный текст транскрипции>

meeting-assistant --user alice find "бюджет"
# ID   CREATED              STATUS       SNIPPET
# 1    2026-11-19 10:12:03  completed    ...обсудили <бюджет> Q1...

meeting-assistant --user alice chat 1 "какие сроки обсудили?"
# Q: какие сроки обсудили?
# A: ...

# чужой пользователь не видит встречу
meeting-assistant --user bob status 1
# meeting 1 not found
# exit 1
```
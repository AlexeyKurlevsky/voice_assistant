# Модель статусов обработки

Документ фиксирует модель статусов задачи обработки встречи и гарантии,
которые даёт каждый переход.

---

## 1. Диаграмма

```
                  ┌──────────────┐
                  │   created    │  (встреча + задача созданы в одной транзакции)
                  └──────┬───────┘
                         │ MarkProcessing
                  ┌──────▼───────┐
                  │  processing  │  (воркер взял задачу)
                  └──────┬───────┘
                         │ SaveTranscriptionAndMarkTranscribed (транзакция)
                  ┌──────▼───────┐
                  │ transcribed  │  (транскрипция в БД)
                  └──────┬───────┘
                         │ SaveSummaryAndMarkSummarized (транзакция)
                  ┌──────▼───────┐
                  │ summarized   │  (выжимка в БД)
                  └──────┬───────┘
                         │ MarkCompleted (транзакция)
                  ┌──────▼───────┐
                  │  completed   │  (всё консистентно, готово к чтению)
                  └──────────────┘

  из processing / transcribed / summarized при ошибке:
                         │ MarkFailed (транзакция)
                  ┌──────▼───────┐
                  │    failed    │  (error_message заполнен)
                  └──────────────┘
```

---

## 2. Семантика статусов

| Статус | Что гарантирует |
|---|---|
| `created` | Встреча и задача созданы атомарно. Файл сохранён на диск. Обработка ещё не начата. |
| `processing` | Воркер взял задачу в работу. Число задач в этом статусе ≤ `WorkerConcurrency`. |
| `transcribed` | Транскрипция сохранена в `transcriptions` и зафиксирована вместе со сменой статуса. |
| `summarized` | Краткая выжимка сохранена в `summaries` и зафиксирована вместе со сменой статуса. LLM-этап успешно завершён. |
| `completed` | Все материалы консистентны: транскрипция и выжимка в БД, задача в терминальном статусе. Встреча безопасно читается командами `get`, `find`, `chat`. |
| `failed` | Обработка прервана ошибкой. `error_message` заполнен. Из терминального статуса переходов нет. |

Разделение `summarized` и `completed` не формальное: `summarized` фиксирует
**успешный ответ LLM**, а `completed` — **финальную консистентность всех данных
в БД**. Если между этими шагами произойдёт сбой БД, задача останется в
`summarized` — это позволит на рестарте завершить финализацию без повторного
вызова LLM (что важно для платных провайдеров).

---

## 3. Гарантии переходов

Каждый переход сопровождается записью в `status_history` (столбец `status` —
новый статус; предыдущий восстанавливается по соседней записи).

| Переход | Транзакция | Атомарные операции |
|---|---|---|
| `created → processing` | **да** | `UPDATE processing_tasks SET status='processing', attempts=attempts+1, updated_at=now()` + `INSERT status_history(status='processing')` |
| `processing → transcribed` | **да** | `INSERT transcriptions` + `UPDATE processing_tasks SET status='transcribed'` + `INSERT status_history(status='transcribed')` |
| `transcribed → summarized` | **да** | `INSERT summaries` + `UPDATE processing_tasks SET status='summarized'` + `INSERT status_history(status='summarized')` |
| `summarized → completed` | **да** | `UPDATE processing_tasks SET status='completed'` + `INSERT status_history(status='completed')` |
| `* → failed` | **да** | `UPDATE processing_tasks SET status='failed', error_message=$1` + `INSERT status_history(status='failed')` |

**Терминальные статусы:** `completed`, `failed`. Из них переходов нет.

**Правила валидации.** Сервис проверяет допустимость перехода перед вызовом
репозитория: например, `processing → completed` (пропуск шагов) недопустим.
На уровне БД проверяется только множество значений (`CHECK`), потому что
допустимость переходов — бизнес-правило, а не инвариант хранилища.

---

## 4. Восстановление после сбоя

При запуске сервиса (только для команды `load` — см. `cmd/voice-assistant/main.go`):

1. `TaskRepo.ResetStaleProcessing(ctx)` — переводит «зависшие» `processing` обратно в `created`,
   возвращает их `meeting_id`.
2. `TaskRepo.ListPending(ctx)` — добирает задачи в статусах `created`, `transcribed`,
   `summarized`, оставшиеся с прошлого запуска.
3. Оба набора ставятся в очередь через `WorkerPool.Enqueue`. Пользователю
   выводится предупреждение в stderr: `[recovery] восстановлено N задач`.

Задачи в статусе `summarized` при рестарте попадают в очередь наравне с `created`.
Воркер видит статус `summarized`, пропускает Speech и LLM, выполняет только
`MarkCompleted`. Повторных вызовов внешних сервисов не происходит.
Аналогично для `transcribed`: транскрипция читается из БД, LLM вызывается
заново (только этап выжимки).

---

## 5. Что показывает `status <id>`

```json
{
  "meeting_id": "3a2b...",
  "status": "failed",
  "created_at": "2026-11-19T10:15:02Z",
  "updated_at": "2026-11-19T10:15:09Z",
  "error_message": "speech client: upstream 503"
}
```

- `status` — текущий статус из `processing_tasks.status`;
- `created_at` — `processing_tasks.created_at`;
- `updated_at` — `processing_tasks.updated_at` (обновляется на каждом переходе);
- `error_message` — заполнен только при `failed`;
- при `--verbose` дополнительно выводится `status_history` (последовательность
  записей, отрисованная стрелками: `created -> processing -> ...`);
- при `--watch` — polling до терминального статуса с обновлением в одной строке
  (text) или NDJSON (json).

---

## 6. Соответствие требованиям ТЗ

| Требование | Где выполнено |
|---|---|
| Минимальный набор статусов из ТЗ | Все 6 присутствуют |
| Прослеживаемый жизненный цикл | `status_history` фиксирует каждый переход |
| Ошибки доступны через `status` | `processing_tasks.error_message` |
| Терминальные статусы | `completed`, `failed` |
| Восстановление после сбоя | `ResetStaleProcessing` + `ListPending` на старте `load` |
| Гарантия консистентности | Каждый переход — в транзакции |
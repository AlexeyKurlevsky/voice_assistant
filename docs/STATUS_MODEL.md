# Модель статусов обработки

Документ фиксирует модель статусов задачи обработки встречи и гарантии,
которые даёт каждый переход.

---

## 1. Диаграмма

```
                  ┌──────────────┐
                  │   created    │  (встреча + задача созданы в одной TX)
                  └──────┬───────┘
                         │ MarkProcessing
                  ┌──────▼───────┐
                  │  processing  │  (воркер взял задачу)
                  └──────┬───────┘
                         │ SaveTranscriptionAndMarkTranscribed (TX)
                  ┌──────▼───────┐
                  │ transcribed  │  (транскрипция в БД)
                  └──────┬───────┘
                         │ SaveSummaryAndMarkSummarized (TX)
                  ┌──────▼───────┐
                  │ summarized   │  (выжимка в БД)
                  └──────┬───────┘
                         │ MarkCompleted (TX)
                  ┌──────▼───────┐
                  │  completed   │  (всё консистентно, готово к чтению)
                  └──────────────┘

  из processing / transcribed / summarized при ошибке:
                         │ MarkFailed (TX)
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

Каждый переход сопровождается записью в `status_history`.

| Переход | Транзакция | Атомарные операции |
|---|---|---|
| `created → processing` | **TX** | `UPDATE processing_tasks SET status='processing', attempts=attempts+1, updated_at=now()` + `INSERT status_history` |
| `processing → transcribed` | **TX** | `INSERT transcriptions` + `UPDATE processing_tasks SET status='transcribed'` + `INSERT status_history` |
| `transcribed → summarized` | **TX** | `INSERT summaries` + `UPDATE processing_tasks SET status='summarized'` + `INSERT status_history` |
| `summarized → completed` | **TX** | `UPDATE processing_tasks SET status='completed'` + `INSERT status_history` |
| `* → failed` | **TX** | `UPDATE processing_tasks SET status='failed', error_message=$1` + `INSERT status_history` |

**Терминальные статусы:** `completed`, `failed`. Из них переходов нет.

**Правила валидации.** Сервис проверяет допустимость перехода перед вызовом
репозитория: например, `processing → completed` (пропуск шагов) недопустим.
На уровне БД проверяется только множество значений (`CHECK`), потому что
допустимость переходов — бизнес-правило, а не инвариант хранилища.

---

## 4. Восстановление после сбоя

При запуске сервиса:

1. `TaskRepo.ResetStaleProcessing(ctx)` — переводит «зависшие» `processing` обратно в `created`,
   возвращает их `meeting_id`.
2. `TaskRepo.ListPending(ctx)` — добирает `created`, оставшиеся с прошлого запуска.
3. Оба набора ставятся в очередь через `WorkerPool.Enqueue`.

Задачи в статусе `summarized` при рестарте попадают в очередь наравне с `created`.
Воркер видит статус `summarized`, пропускает Speech и LLM, выполняет только
`MarkCompleted`. Повторных вызовов внешних сервисов не происходит.

---

## 5. Что показывает `status <id>`

```json
{
  "meeting_id": 3,
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
- при `--verbose` дополнительно выводится `status_history`.

---
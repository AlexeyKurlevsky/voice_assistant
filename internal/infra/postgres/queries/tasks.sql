-- name: CreateTask :one
INSERT INTO processing_tasks (meeting_id, status)
VALUES ($1, $2)
RETURNING id, meeting_id, status, attempts, created_at, updated_at;

-- name: GetTaskByID :one
SELECT id, meeting_id, status, error_message, attempts, created_at, updated_at
FROM processing_tasks
WHERE id = $1;

-- name: GetTaskForMeetingAndUser :one
SELECT t.id, t.meeting_id, t.status, t.error_message, t.attempts, t.created_at, t.updated_at
FROM processing_tasks t
JOIN meetings m ON m.id = t.meeting_id
WHERE t.meeting_id = $1 AND m.user_id = $2;

-- name: MarkTaskProcessing :exec
UPDATE processing_tasks
SET status = 'processing', attempts = attempts + 1, updated_at = now()
WHERE id = $1;

-- name: MarkTaskTranscribed :exec
UPDATE processing_tasks SET status = 'transcribed', updated_at = now() WHERE id = $1;

-- name: MarkTaskSummarized :exec
UPDATE processing_tasks SET status = 'summarized', updated_at = now() WHERE id = $1;

-- name: MarkTaskCompleted :exec
UPDATE processing_tasks SET status = 'completed', updated_at = now() WHERE id = $1;

-- name: MarkTaskFailed :exec
UPDATE processing_tasks
SET status = 'failed', error_message = $2, updated_at = now()
WHERE id = $1;

-- name: InsertStatusHistory :exec
INSERT INTO status_history (task_id, status)
VALUES ($1, $2);

-- name: ListStatusHistory :many
SELECT status, created_at
FROM status_history
WHERE task_id = $1
ORDER BY created_at ASC;

-- name: ResetStaleProcessing :many
UPDATE processing_tasks
SET status = 'created', updated_at = now()
WHERE status = 'processing'
RETURNING id;

-- name: ListPendingTasks :many
SELECT id FROM processing_tasks WHERE status IN ('created', 'transcribed', 'summarized');
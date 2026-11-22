-- name: CreateTask :one
INSERT INTO processing_tasks (meeting_id, status)
VALUES ($1, $2)
RETURNING id, meeting_id, status, attempts, created_at, updated_at;

-- name: GetTaskStatus :one
SELECT status FROM processing_tasks WHERE id = $1;

-- name: MarkTaskFailed :exec
UPDATE processing_tasks
SET status = 'failed',
    error_message = $2,
    updated_at = now()
WHERE id = $1;

-- name: InsertStatusHistory :exec
INSERT INTO status_history (task_id, old_status, new_status)
VALUES ($1, $2, $3);
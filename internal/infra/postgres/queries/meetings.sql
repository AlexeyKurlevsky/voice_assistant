-- name: CreateMeeting :one
INSERT INTO meetings (user_id, title, original_filename, file_path)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, title, original_filename, file_path, created_at;

-- name: ListMeetingsByUser :many
SELECT m.id, m.title, m.created_at,
       t.status,
       COALESCE(s.text, '') AS summary
FROM meetings m
JOIN processing_tasks t ON t.meeting_id = m.id
LEFT JOIN summaries s   ON s.meeting_id = m.id
WHERE m.user_id = $1
ORDER BY m.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetMeetingForUser :one
SELECT id, user_id, title, original_filename, file_path, created_at
FROM meetings
WHERE id = $1 AND user_id = $2;

-- name: GetMeetingByTaskID :one
SELECT m.id, m.user_id, m.title, m.original_filename, m.file_path, m.created_at
FROM meetings m
JOIN processing_tasks t ON t.meeting_id = m.id
WHERE t.id = $1;
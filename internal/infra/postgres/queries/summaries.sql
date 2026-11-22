-- name: InsertSummary :exec
INSERT INTO summaries (meeting_id, text)
VALUES ($1, $2);

-- name: GetSummaryForUser :one
SELECT s.text
FROM summaries s
JOIN meetings m ON m.id = s.meeting_id
WHERE s.meeting_id = $1 AND m.user_id = $2;
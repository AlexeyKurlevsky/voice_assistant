-- name: InsertTranscription :exec
INSERT INTO transcriptions (meeting_id, text)
VALUES ($1, $2);

-- name: GetTranscriptForUser :one
SELECT tr.text
FROM transcriptions tr
JOIN meetings m ON m.id = tr.meeting_id
WHERE tr.meeting_id = $1 AND m.user_id = $2;
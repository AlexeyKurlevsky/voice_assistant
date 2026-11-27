-- name: SaveChatMessage :one
INSERT INTO chat_messages (user_id, meeting_id, question, answer)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, meeting_id, question, answer, created_at;

-- name: HistoryChatMessages :many
SELECT id, user_id, meeting_id, question, answer, created_at
FROM chat_messages
WHERE user_id = $1 AND meeting_id = $2
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count)::int;
-- name: CreateUser :one
INSERT INTO users (external_id, token_hash)
VALUES ($1, $2)
RETURNING id, external_id, created_at;

-- name: GetUserByExternalID :one
SELECT id, external_id, created_at
FROM users
WHERE external_id = $1;

-- name: GetUserByTokenHash :one
SELECT id, external_id, created_at
FROM users
WHERE token_hash = $1;

-- name: UpdateUserTokenHash :exec
UPDATE users SET token_hash = $2 WHERE id = $1;
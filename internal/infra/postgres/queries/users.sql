-- name: EnsureUser :one
INSERT INTO users (external_id)
VALUES ($1)
ON CONFLICT (external_id) DO UPDATE SET external_id = EXCLUDED.external_id
RETURNING id, external_id, created_at;

-- name: GetUserByExternalID :one
SELECT id, external_id, created_at
FROM users
WHERE external_id = $1;
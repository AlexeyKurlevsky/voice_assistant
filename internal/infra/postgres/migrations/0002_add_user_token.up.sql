ALTER TABLE users ADD COLUMN token_hash TEXT;

-- Существующим пользователям ставим «фантомные» токены, которые никто
-- не знает. Для них нужен будет ручной reset через psql или пересоздание БД.
UPDATE users
SET token_hash = encode(gen_random_bytes(32), 'hex')
WHERE token_hash IS NULL;

ALTER TABLE users ALTER COLUMN token_hash SET NOT NULL;
CREATE UNIQUE INDEX idx_users_token_hash ON users(token_hash);
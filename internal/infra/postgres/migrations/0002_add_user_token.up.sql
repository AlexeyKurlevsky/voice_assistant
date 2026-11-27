ALTER TABLE users ADD COLUMN token_hash TEXT;

-- Существующим пользователям ставим «фантомные» токены, которые никто
-- не знает. Для них нужен будет ручной reset через psql или пересоздание БД.
-- Используем gen_random_uuid() (ядро PostgreSQL 13+): расширение pgcrypto
-- (в нём gen_random_bytes()) может быть не установлено.
UPDATE users
SET token_hash = replace(gen_random_uuid()::text, '-', '')
               || replace(gen_random_uuid()::text, '-', '')
WHERE token_hash IS NULL;

ALTER TABLE users ALTER COLUMN token_hash SET NOT NULL;
CREATE UNIQUE INDEX idx_users_token_hash ON users(token_hash);

ALTER TABLE meetings ADD COLUMN content_hash TEXT;
CREATE INDEX idx_meetings_user_content_hash ON meetings (user_id, content_hash);

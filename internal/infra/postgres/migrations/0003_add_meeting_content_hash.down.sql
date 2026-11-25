DROP INDEX IF EXISTS idx_meetings_user_content_hash;
ALTER TABLE meetings DROP COLUMN IF EXISTS content_hash;

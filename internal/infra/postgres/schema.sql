CREATE TABLE users (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT        NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE meetings (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title             TEXT        NOT NULL,
    original_filename TEXT        NOT NULL,
    file_path         TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_meetings_user_created ON meetings (user_id, created_at DESC);

CREATE TABLE processing_tasks (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id    UUID        NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    status        TEXT        NOT NULL
                  CHECK (status IN ('created','processing','transcribed',
                                    'summarized','completed','failed')),
    error_message TEXT,
    attempts      INT         NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (meeting_id)
);
CREATE INDEX idx_tasks_status ON processing_tasks (status);

CREATE TABLE status_history (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID        NOT NULL REFERENCES processing_tasks(id) ON DELETE CASCADE,
    old_status TEXT,
    new_status TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_status_history_task ON status_history (task_id, created_at);

CREATE TABLE transcriptions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id UUID        NOT NULL UNIQUE REFERENCES meetings(id) ON DELETE CASCADE,
    text       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_transcriptions_fts
    ON transcriptions USING GIN (to_tsvector('simple', text));

CREATE TABLE summaries (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id UUID        NOT NULL UNIQUE REFERENCES meetings(id) ON DELETE CASCADE,
    text       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_summaries_fts
    ON summaries USING GIN (to_tsvector('simple', text));

CREATE TABLE chat_messages (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    meeting_id UUID        NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    question   TEXT        NOT NULL,
    answer     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_chat_user_meeting ON chat_messages (user_id, meeting_id, created_at DESC);
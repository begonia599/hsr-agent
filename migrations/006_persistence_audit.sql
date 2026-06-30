CREATE TABLE IF NOT EXISTS conversations (
    id          BIGSERIAL PRIMARY KEY,
    session_id  TEXT,
    user_id     TEXT,
    title       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta        JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_conversations_session_updated
    ON conversations (session_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversations_updated
    ON conversations (updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_turns (
    id                BIGSERIAL PRIMARY KEY,
    conversation_id   BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    trace_id          TEXT NOT NULL UNIQUE,
    model             TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'error', 'aborted', 'max_steps')),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ,
    latency_ms        INT,
    tool_call_count   INT NOT NULL DEFAULT 0,
    prompt_tokens     INT,
    completion_tokens INT,
    total_tokens      INT,
    error_code        TEXT,
    error_message     TEXT,
    final_answer      TEXT,
    meta              JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_agent_turns_conversation_started
    ON agent_turns (conversation_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_turns_status_started
    ON agent_turns (status, started_at DESC);

CREATE TABLE IF NOT EXISTS messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content         TEXT NOT NULL,
    turn_id         BIGINT REFERENCES agent_turns(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created
    ON messages (conversation_id, created_at, id);

CREATE INDEX IF NOT EXISTS idx_messages_turn
    ON messages (turn_id);

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    id            BIGSERIAL PRIMARY KEY,
    turn_id       BIGINT NOT NULL REFERENCES agent_turns(id) ON DELETE CASCADE,
    seq           INT NOT NULL,
    tool_call_id  TEXT,
    tool_name     TEXT NOT NULL,
    arguments     JSONB NOT NULL DEFAULT '{}'::jsonb,
    result        JSONB,
    error         TEXT,
    latency_ms    INT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (turn_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_agent_tool_calls_turn_seq
    ON agent_tool_calls (turn_id, seq);

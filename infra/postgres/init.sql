CREATE EXTENSION IF NOT EXISTS vector;

-- Interest vectors for the proximity-matching engine.
-- Raw audio/transcripts are NEVER persisted; only curated interest tokens
-- and their embedding vectors leave the privacy pipeline.
CREATE TABLE IF NOT EXISTS interest_vectors (
    user_id      TEXT PRIMARY KEY,
    interests    TEXT[]   NOT NULL,
    embedding    vector(384) NOT NULL,
    similarity   FLOAT NOT NULL DEFAULT 0.0,
    last_seen    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Vector similarity index (brute-force is acceptable at 15m-candidate scale).
CREATE INDEX IF NOT EXISTS idx_interest_vectors_embedding
    ON interest_vectors USING hnsw (embedding vector_cosine_ops);

-- Ephemeral match audit trail (already scrubbed, token-only).
CREATE TABLE IF NOT EXISTS ephemeral_matches (
    id           TEXT PRIMARY KEY,
    user_a       TEXT NOT NULL,
    user_b       TEXT NOT NULL,
    shared_tokens TEXT[] NOT NULL,
    similarity   FLOAT NOT NULL,
    distance_m   FLOAT NOT NULL,
    icebreaker   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ephemeral_matches_expires
    ON ephemeral_matches (expires_at);
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
    id           SERIAL PRIMARY KEY,
    login        TEXT NOT NULL UNIQUE,
    display_name TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO users (id, login, display_name) VALUES (1, 'local', 'Local Dev User');
SELECT setval('users_id_seq', (SELECT MAX(id) FROM users));

CREATE TABLE videos (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               INTEGER NOT NULL REFERENCES users(id),
    karakeep_bookmark_id  TEXT,
    youtube_id            TEXT NOT NULL UNIQUE,
    title                 TEXT,
    channel               TEXT,
    duration_seconds      INTEGER,
    language              TEXT,
    transcript            TEXT,
    summary               TEXT,
    detail_level          TEXT NOT NULL DEFAULT 'medium',
    embedding             vector(1024),
    metadata              JSONB NOT NULL DEFAULT '{}',
    status                TEXT NOT NULL DEFAULT 'pending',
    error_message         TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_videos_embedding ON videos
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
CREATE INDEX idx_videos_user_created ON videos (user_id, created_at DESC);
CREATE INDEX idx_videos_metadata ON videos USING gin (metadata jsonb_path_ops);
CREATE INDEX idx_videos_youtube_id ON videos (youtube_id);
CREATE INDEX idx_videos_status ON videos (status);

CREATE OR REPLACE FUNCTION match_videos(
    query_embedding vector(1024),
    match_user_id   INTEGER,
    match_threshold FLOAT DEFAULT 0.3,
    match_count     INTEGER DEFAULT 10
) RETURNS TABLE (
    id UUID, youtube_id TEXT, title TEXT, channel TEXT, summary TEXT,
    metadata JSONB, similarity FLOAT, created_at TIMESTAMPTZ
) LANGUAGE sql STABLE AS $$
    SELECT v.id, v.youtube_id, v.title, v.channel, v.summary,
           v.metadata,
           1 - (v.embedding <=> query_embedding) AS similarity,
           v.created_at
    FROM videos v
    WHERE v.user_id = match_user_id
      AND v.embedding IS NOT NULL
      AND 1 - (v.embedding <=> query_embedding) > match_threshold
    ORDER BY v.embedding <=> query_embedding
    LIMIT match_count;
$$;

-- Widen `videos` from YouTube-only to a source-discriminated content table.
-- Podcast rows carry NULL in youtube_id; the UNIQUE(user_id, youtube_id) index
-- from 000003 survives because Postgres treats NULLs as distinct by default.

ALTER TABLE videos ADD COLUMN source         TEXT NOT NULL DEFAULT 'youtube';
ALTER TABLE videos ADD COLUMN external_id    TEXT;
ALTER TABLE videos ADD COLUMN source_url     TEXT;
ALTER TABLE videos ADD COLUMN source_feed_id TEXT;
ALTER TABLE videos ADD COLUMN thumbnail_url  TEXT;
ALTER TABLE videos ADD COLUMN published_at   TIMESTAMPTZ;

UPDATE videos SET external_id = youtube_id WHERE external_id IS NULL;
ALTER TABLE videos ALTER COLUMN external_id SET NOT NULL;
ALTER TABLE videos ALTER COLUMN youtube_id  DROP NOT NULL;

ALTER TABLE videos ADD CONSTRAINT videos_source_check
    CHECK (source IN ('youtube', 'podcast'));
ALTER TABLE videos ADD CONSTRAINT videos_user_source_external_key
    UNIQUE (user_id, source, external_id);

CREATE INDEX idx_videos_user_source_created ON videos (user_id, source, created_at DESC);
CREATE INDEX idx_videos_user_feed ON videos (user_id, source_feed_id) WHERE source = 'podcast';

-- match_videos gains a source filter and returns the new columns. The return
-- type changes, which CREATE OR REPLACE cannot do — the function has to go
-- first. Migration and Go code must ship in the same release.
DROP FUNCTION IF EXISTS match_videos(vector, integer, double precision, integer);

CREATE FUNCTION match_videos(
    query_embedding vector(1024),
    match_user_id   INTEGER,
    match_threshold FLOAT DEFAULT 0.3,
    match_count     INTEGER DEFAULT 10,
    match_source    TEXT DEFAULT NULL
) RETURNS TABLE (
    id UUID, youtube_id TEXT, source TEXT, external_id TEXT, source_url TEXT,
    title TEXT, channel TEXT, summary TEXT,
    metadata JSONB, similarity FLOAT, created_at TIMESTAMPTZ
) LANGUAGE sql STABLE AS $$
    SELECT v.id, COALESCE(v.youtube_id, '') AS youtube_id,
           v.source, v.external_id, COALESCE(v.source_url, '') AS source_url,
           v.title, v.channel, v.summary,
           v.metadata,
           1 - (v.embedding <=> query_embedding) AS similarity,
           v.created_at
    FROM videos v
    WHERE v.user_id = match_user_id
      AND v.embedding IS NOT NULL
      AND (match_source IS NULL OR v.source = match_source)
      AND 1 - (v.embedding <=> query_embedding) > match_threshold
    ORDER BY v.embedding <=> query_embedding
    LIMIT match_count;
$$;

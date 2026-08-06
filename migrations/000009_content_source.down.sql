-- Reverse 000009. Podcast rows have no representation in the narrowed schema,
-- so they are deleted rather than silently turned into YouTube rows with an
-- empty youtube_id.

DROP FUNCTION IF EXISTS match_videos(vector, integer, double precision, integer, text);

CREATE FUNCTION match_videos(
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

DELETE FROM videos WHERE source <> 'youtube';

DROP INDEX IF EXISTS idx_videos_user_feed;
DROP INDEX IF EXISTS idx_videos_user_source_created;

ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_user_source_external_key;
ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_source_check;

ALTER TABLE videos ALTER COLUMN youtube_id SET NOT NULL;

ALTER TABLE videos DROP COLUMN IF EXISTS published_at;
ALTER TABLE videos DROP COLUMN IF EXISTS thumbnail_url;
ALTER TABLE videos DROP COLUMN IF EXISTS source_feed_id;
ALTER TABLE videos DROP COLUMN IF EXISTS source_url;
ALTER TABLE videos DROP COLUMN IF EXISTS external_id;
ALTER TABLE videos DROP COLUMN IF EXISTS source;

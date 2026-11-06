-- Reverse 000019. Vectors of a model other than mistral-embed cannot be
-- searched by the restored function, which embeds queries with mistral-embed,
-- so they are cleared rather than left to compare as noise.

UPDATE videos SET embedding = NULL
WHERE embedding_model IS DISTINCT FROM 'mistral-embed';

DROP FUNCTION IF EXISTS match_videos(vector, text, integer, double precision, integer, text);

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

ALTER TABLE videos DROP COLUMN IF EXISTS embedding_model;

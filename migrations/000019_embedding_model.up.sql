-- Record which model produced each row's embedding.
--
-- Until this migration every embedding was mistral-embed at 1024 dimensions,
-- and that was the reason embeddings were not configurable (DECISIONS.md,
-- 2026-03): vectors of two models in one column compare as noise. With the
-- model recorded per row, search filters on the model it embedded the query
-- with, and the background re-embed (service.reembedStale) replaces every
-- vector whose model differs from the configured one. A change of embedding
-- model is then a config value, not a migration.

ALTER TABLE videos ADD COLUMN embedding_model TEXT;

UPDATE videos SET embedding_model = 'mistral-embed' WHERE embedding IS NOT NULL;

-- The signature changes, which CREATE OR REPLACE cannot do. Migration and Go
-- code ship in the same release.
DROP FUNCTION IF EXISTS match_videos(vector, integer, double precision, integer, text);

CREATE FUNCTION match_videos(
    query_embedding vector(1024),
    query_model     TEXT,
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
      AND v.embedding_model = query_model
      AND (match_source IS NULL OR v.source = match_source)
      AND 1 - (v.embedding <=> query_embedding) > match_threshold
    ORDER BY v.embedding <=> query_embedding
    LIMIT match_count;
$$;

-- Make the derived content of a video or episode shared across users.
--
-- Until this migration every user carried a full private copy: `videos` is
-- keyed UNIQUE(user_id, source, external_id) since 000009, and the ingest path
-- looked its work up user-scoped, so a second user bookmarking the same video
-- paid for a second InnerTube transcript fetch, a second Voxtral run where
-- captions were missing, a second LLM summary and a second embedding.
--
-- The row stays per user — the library, the Karakeep bookmark and the tokens
-- belong to a person. What is now shared is everything derived from the source:
-- transcript, segments, metadata, summary, embedding, topics. The write methods
-- in internal/storage/videos.go address every row with the same
-- (source, external_id), so a regeneration by any user is the version all users
-- see. Last write wins, deliberately; the settings that produce a summary are
-- written for all users too (storage.SetUserPrompt, storage.SetModelPreference).

-- The user-blind lookup the ingest short-circuit performs.
CREATE INDEX idx_videos_source_external ON videos (source, external_id);

-- Reconcile what the per-user era produced. For every (source, external_id)
-- carrying more than one row, the most recently updated completed row wins and
-- its derived fields are copied onto its siblings. Groups without a completed
-- row are left alone — there is nothing to share yet.
WITH winner AS (
    SELECT DISTINCT ON (source, external_id)
           id, source, external_id,
           transcript, transcript_segments, summary, detail_level,
           summary_provider, summary_model,
           summary_input_tokens, summary_output_tokens,
           embedding, metadata,
           title, channel, language, duration_seconds, thumbnail_url
    FROM videos
    WHERE status = 'completed'
    ORDER BY source, external_id, updated_at DESC
)
UPDATE videos v SET
    transcript            = w.transcript,
    transcript_segments   = w.transcript_segments,
    summary               = w.summary,
    detail_level          = w.detail_level,
    summary_provider      = w.summary_provider,
    summary_model         = w.summary_model,
    summary_input_tokens  = w.summary_input_tokens,
    summary_output_tokens = w.summary_output_tokens,
    embedding             = w.embedding,
    metadata              = w.metadata,
    title                 = w.title,
    channel               = w.channel,
    language              = w.language,
    duration_seconds      = w.duration_seconds,
    -- COALESCE rather than a plain copy: a winner predating the 000013
    -- thumbnail backfill would otherwise clear a sibling that has one.
    thumbnail_url         = COALESCE(w.thumbnail_url, v.thumbnail_url),
    status                = 'completed',
    error_message         = NULL,
    updated_at            = NOW()
FROM winner w
WHERE v.source = w.source AND v.external_id = w.external_id AND v.id <> w.id;

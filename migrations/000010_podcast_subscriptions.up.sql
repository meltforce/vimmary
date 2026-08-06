CREATE TABLE podcast_subscriptions (
    id             SERIAL PRIMARY KEY,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id        TEXT NOT NULL,             -- cast2md feed.id
    feed_title     TEXT NOT NULL DEFAULT '',
    image_url      TEXT,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    detail_level   TEXT NOT NULL DEFAULT 'medium',
    initialized    BOOLEAN NOT NULL DEFAULT FALSE,
    -- Opaque copy of cast2md's episode.updated_at. cast2md stores naive local
    -- timestamps, so the value is carried through as text and always comes
    -- from cast2md's response, never from vimmary's clock.
    watermark      TEXT NOT NULL DEFAULT '',
    last_polled_at TIMESTAMPTZ,
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, feed_id)
);
CREATE INDEX idx_podcast_subs_enabled ON podcast_subscriptions (enabled) WHERE enabled;

CREATE TABLE user_prompts (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source     TEXT NOT NULL,
    level      TEXT NOT NULL,
    prompt     TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, source, level),
    CHECK (source IN ('youtube', 'podcast')),
    CHECK (level  IN ('medium', 'deep'))
);

-- Carry the existing per-user prompts over. users.summary_prompt_medium/deep
-- stay in place until user_prompts is verified in production; migration 000011
-- removes them.
INSERT INTO user_prompts (user_id, source, level, prompt)
SELECT id, 'youtube', 'medium', summary_prompt_medium
FROM users WHERE summary_prompt_medium IS NOT NULL AND summary_prompt_medium <> '';

INSERT INTO user_prompts (user_id, source, level, prompt)
SELECT id, 'youtube', 'deep', summary_prompt_deep
FROM users WHERE summary_prompt_deep IS NOT NULL AND summary_prompt_deep <> '';

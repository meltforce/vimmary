-- YouTube channel subscriptions and their triage inbox.
--
-- Discovery runs over the public per-channel RSS feed, so the poller never
-- touches InnerTube. Dedup is by video ID, not by a timestamp watermark: RSS
-- reorders entries and republishes them on edits, so UNIQUE(user_id,
-- youtube_id) with ON CONFLICT DO NOTHING is the exact seen-set. Dismissed and
-- queued rows stay — deleting one while its video is still inside the feed's
-- ~15-entry window would resurrect it on the next poll.
CREATE TABLE channel_subscriptions (
    id             SERIAL PRIMARY KEY,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id     TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    thumbnail_url  TEXT,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    last_polled_at TIMESTAMPTZ,
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, channel_id)
);

CREATE INDEX idx_channel_subs_enabled ON channel_subscriptions (enabled) WHERE enabled;

-- No thumbnail column: item artwork is derived from youtube_id the way every
-- other YouTube row's is. Unfollowing a channel deletes its items via the
-- cascade; re-following re-imports the feed's current window.
CREATE TABLE inbox_items (
    id              SERIAL PRIMARY KEY,
    subscription_id INTEGER NOT NULL REFERENCES channel_subscriptions(id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    youtube_id      TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    published_at    TIMESTAMPTZ,
    state           TEXT NOT NULL DEFAULT 'new' CHECK (state IN ('new', 'queued', 'dismissed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, youtube_id)
);

CREATE INDEX idx_inbox_items_new ON inbox_items (user_id, published_at DESC) WHERE state = 'new';

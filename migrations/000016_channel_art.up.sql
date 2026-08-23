-- Avatar cache for library channels the user does not follow.
--
-- Followed channels carry their avatar on channel_subscriptions; the channels
-- that exist only through Karakeep imports are known by name alone. Their
-- avatar is resolved through one of their videos (the watch page names the
-- channel ID, the channel page carries the og:image) and cached here, keyed
-- by the same channel name the videos rows carry.
--
-- thumbnail_url NULL with a fetched_at records a failed resolution, so the
-- poller retries it only after a cool-down instead of on every cycle.
CREATE TABLE channel_art (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel       TEXT NOT NULL,
    thumbnail_url TEXT,
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, channel)
);

-- How many of a feed's newest completed episodes are summarized when the
-- subscription is switched on. 0 restores the original "from now on" behaviour,
-- where the first poll only records a watermark.
--
-- The default is 3 rather than 0 because a subscription that produces nothing
-- until the show publishes again gives no sign that it works.
ALTER TABLE podcast_subscriptions
    ADD COLUMN initial_backfill INTEGER NOT NULL DEFAULT 3;

ALTER TABLE podcast_subscriptions
    ADD CONSTRAINT podcast_subscriptions_initial_backfill_check
    CHECK (initial_backfill >= 0 AND initial_backfill <= 100);

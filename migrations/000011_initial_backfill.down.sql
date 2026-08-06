ALTER TABLE podcast_subscriptions
    DROP CONSTRAINT IF EXISTS podcast_subscriptions_initial_backfill_check;
ALTER TABLE podcast_subscriptions DROP COLUMN IF EXISTS initial_backfill;

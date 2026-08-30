-- The backfill merges rows that were already distinct copies; which value a
-- given row held before the merge is not recorded anywhere, so only the index
-- comes back off.
DROP INDEX IF EXISTS idx_videos_source_external;

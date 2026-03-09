-- Make youtube_id unique per user instead of globally unique,
-- so multiple users can bookmark the same video.
ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_youtube_id_key;
DROP INDEX IF EXISTS idx_videos_youtube_id;
ALTER TABLE videos ADD CONSTRAINT videos_user_id_youtube_id_key UNIQUE (user_id, youtube_id);
CREATE INDEX idx_videos_user_youtube_id ON videos (user_id, youtube_id);

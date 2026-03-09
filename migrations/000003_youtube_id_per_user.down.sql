ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_user_id_youtube_id_key;
DROP INDEX IF EXISTS idx_videos_user_youtube_id;
ALTER TABLE videos ADD CONSTRAINT videos_youtube_id_key UNIQUE (youtube_id);
CREATE INDEX idx_videos_youtube_id ON videos (youtube_id);

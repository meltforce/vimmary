-- Backfill the poster image for YouTube rows.
--
-- thumbnail_url was only ever written by the podcast path, so every YouTube row
-- created before this carries NULL and the media feed renders the neutral "no
-- art" block for all of them. The URL is derived from the ID exactly as
-- youtube.ThumbnailURL does it, so no network call is needed here.
--
-- Rows that already carry a value are left alone, and podcast rows are excluded
-- because their artwork comes from the feed's image_url.
UPDATE videos
SET thumbnail_url = 'https://i.ytimg.com/vi/' || youtube_id || '/hqdefault.jpg'
WHERE source = 'youtube'
  AND youtube_id IS NOT NULL
  AND youtube_id <> ''
  AND (thumbnail_url IS NULL OR thumbnail_url = '');

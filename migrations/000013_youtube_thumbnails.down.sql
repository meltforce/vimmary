-- Clear the derived thumbnails again. Only the exact derived form is matched,
-- so a URL that came from anywhere else survives.
UPDATE videos
SET thumbnail_url = NULL
WHERE source = 'youtube'
  AND thumbnail_url = 'https://i.ytimg.com/vi/' || youtube_id || '/hqdefault.jpg';

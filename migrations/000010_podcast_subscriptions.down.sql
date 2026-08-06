-- users.summary_prompt_medium/deep still exist at this point, so the YouTube
-- prompts survive the reversal. Podcast prompts have no target column and are
-- dropped with the table.
UPDATE users u SET summary_prompt_medium = p.prompt
FROM user_prompts p
WHERE p.user_id = u.id AND p.source = 'youtube' AND p.level = 'medium';

UPDATE users u SET summary_prompt_deep = p.prompt
FROM user_prompts p
WHERE p.user_id = u.id AND p.source = 'youtube' AND p.level = 'deep';

DROP TABLE IF EXISTS user_prompts;
DROP TABLE IF EXISTS podcast_subscriptions;

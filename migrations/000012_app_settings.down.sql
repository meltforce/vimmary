-- Dropping this loses the API keys, and nothing else holds them: the setec
-- secrets they came from are deleted once the migration up has been confirmed.
-- Read them out before rolling back.
DROP TABLE IF EXISTS app_settings;

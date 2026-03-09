ALTER TABLE users ADD COLUMN webhook_token TEXT UNIQUE;
ALTER TABLE users ADD COLUMN karakeep_api_key TEXT;

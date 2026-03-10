ALTER TABLE users ADD COLUMN claude_model TEXT;
ALTER TABLE users ADD COLUMN mistral_model TEXT;
ALTER TABLE videos ADD COLUMN summary_model TEXT NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN summary_input_tokens INT NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN summary_output_tokens INT NOT NULL DEFAULT 0;

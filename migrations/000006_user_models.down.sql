ALTER TABLE users DROP COLUMN claude_model;
ALTER TABLE users DROP COLUMN mistral_model;
ALTER TABLE videos DROP COLUMN summary_model;
ALTER TABLE videos DROP COLUMN summary_input_tokens;
ALTER TABLE videos DROP COLUMN summary_output_tokens;

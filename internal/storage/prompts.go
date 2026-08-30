package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The summary prompts are shared across all users, even though user_prompts is
// keyed by user: a summary is stored once per piece of content and reused by
// everyone (see migration 000018), so a prompt that differed per user would
// decide the shared text by whoever regenerated last without saying so. Setting
// one writes it for every user; reading one falls back to another user's row,
// which is what gives a user created after the change the same prompt.
//
// The per-user key is kept rather than folded into app_settings so the sharing
// can be withdrawn without a migration. The prompt itself is not a credential —
// users.karakeep_api_key is, and that one stays private.

// GetUserPrompt returns the custom prompt for one source and level. An empty
// string means none is stored and the built-in default applies.
func (db *DB) GetUserPrompt(ctx context.Context, userID int, source, level string) (string, error) {
	var prompt string
	err := db.Pool.QueryRow(ctx, `
		SELECT prompt FROM user_prompts
		WHERE source = $2 AND level = $3
		ORDER BY (user_id = $1) DESC, updated_at DESC
		LIMIT 1
	`, userID, source, level).Scan(&prompt)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get user prompt: %w", err)
	}
	return prompt, nil
}

// GetUserPrompts returns the custom prompts for one source, keyed by level.
// Levels without a custom prompt are absent from the map.
func (db *DB) GetUserPrompts(ctx context.Context, userID int, source string) (map[string]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT ON (level) level, prompt
		FROM user_prompts
		WHERE source = $2
		ORDER BY level, (user_id = $1) DESC, updated_at DESC
	`, userID, source)
	if err != nil {
		return nil, fmt.Errorf("get user prompts: %w", err)
	}
	defer rows.Close()

	prompts := make(map[string]string)
	for rows.Next() {
		var level, prompt string
		if err := rows.Scan(&level, &prompt); err != nil {
			return nil, fmt.Errorf("scan user prompt: %w", err)
		}
		prompts[level] = prompt
	}
	return prompts, rows.Err()
}

// SetUserPrompt stores a custom prompt for every user. An empty prompt deletes
// the rows, which resets that source and level to the built-in default. The
// userID argument names who asked; it does not narrow what is written.
func (db *DB) SetUserPrompt(ctx context.Context, userID int, source, level, prompt string) error {
	if prompt == "" {
		_, err := db.Pool.Exec(ctx,
			`DELETE FROM user_prompts WHERE source = $1 AND level = $2`,
			source, level)
		if err != nil {
			return fmt.Errorf("reset user prompt: %w", err)
		}
		return nil
	}
	// INSERT ... SELECT over users rather than one row: every user carries the
	// same prompt, so a user who has never opened Settings still summarizes
	// with it.
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO user_prompts (user_id, source, level, prompt)
		SELECT id, $1, $2, $3 FROM users
		ON CONFLICT (user_id, source, level) DO UPDATE SET
			prompt = EXCLUDED.prompt, updated_at = NOW()
	`, source, level, prompt)
	if err != nil {
		return fmt.Errorf("set user prompt: %w", err)
	}
	return nil
}

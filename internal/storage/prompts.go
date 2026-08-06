package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetUserPrompt returns the user's custom prompt for one source and level.
// An empty string means no custom prompt is stored and the default applies.
func (db *DB) GetUserPrompt(ctx context.Context, userID int, source, level string) (string, error) {
	var prompt string
	err := db.Pool.QueryRow(ctx,
		`SELECT prompt FROM user_prompts WHERE user_id = $1 AND source = $2 AND level = $3`,
		userID, source, level).Scan(&prompt)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get user prompt: %w", err)
	}
	return prompt, nil
}

// GetUserPrompts returns the user's custom prompts for one source, keyed by
// level. Levels without a custom prompt are absent from the map.
func (db *DB) GetUserPrompts(ctx context.Context, userID int, source string) (map[string]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT level, prompt FROM user_prompts WHERE user_id = $1 AND source = $2`,
		userID, source)
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

// SetUserPrompt stores a custom prompt. An empty prompt deletes the row, which
// resets that source and level to the built-in default.
func (db *DB) SetUserPrompt(ctx context.Context, userID int, source, level, prompt string) error {
	if prompt == "" {
		_, err := db.Pool.Exec(ctx,
			`DELETE FROM user_prompts WHERE user_id = $1 AND source = $2 AND level = $3`,
			userID, source, level)
		if err != nil {
			return fmt.Errorf("reset user prompt: %w", err)
		}
		return nil
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO user_prompts (user_id, source, level, prompt)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, source, level) DO UPDATE SET
			prompt = EXCLUDED.prompt, updated_at = NOW()
	`, userID, source, level, prompt)
	if err != nil {
		return fmt.Errorf("set user prompt: %w", err)
	}
	return nil
}

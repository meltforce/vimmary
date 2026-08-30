package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (db *DB) GetOrCreateUser(ctx context.Context, login, displayName string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (login, display_name)
		VALUES ($1, $2)
		ON CONFLICT (login) DO UPDATE
			SET last_seen = NOW(), display_name = COALESCE(NULLIF($2, ''), users.display_name)
		RETURNING id
	`, login, displayName).Scan(&id)
	return id, err
}

// GetUserIdentity returns the login and display name the identity middleware
// resolved this user to. The UI names it in Settings — on a shared Tailscale
// device the library on screen is otherwise unattributable, and every row in
// `videos` hangs off this ID.
func (db *DB) GetUserIdentity(ctx context.Context, userID int) (login, displayName string, err error) {
	var dn *string
	err = db.Pool.QueryRow(ctx,
		`SELECT login, display_name FROM users WHERE id = $1`, userID).Scan(&login, &dn)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("get user identity: %w", err)
	}
	if dn != nil {
		displayName = *dn
	}
	return login, displayName, nil
}

func (db *DB) GetPrimaryUser(ctx context.Context) (id int, login string, err error) {
	err = db.Pool.QueryRow(ctx, `
		SELECT id, login FROM users
		WHERE login LIKE '%@%'
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&id, &login)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return
}

// GetOrCreateWebhookToken returns the user's webhook token, generating one if it doesn't exist.
func (db *DB) GetOrCreateWebhookToken(ctx context.Context, userID int) (string, error) {
	// Try to read existing token first
	var token *string
	err := db.Pool.QueryRow(ctx, `SELECT webhook_token FROM users WHERE id = $1`, userID).Scan(&token)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}
	if token != nil && *token != "" {
		return *token, nil
	}

	// Generate new 32-byte hex token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	newToken := hex.EncodeToString(b)

	_, err = db.Pool.Exec(ctx, `UPDATE users SET webhook_token = $1 WHERE id = $2`, newToken, userID)
	if err != nil {
		return "", fmt.Errorf("save token: %w", err)
	}
	return newToken, nil
}

// GetUserByWebhookToken looks up a user ID by webhook token. Returns ErrNotFound
// if no user carries it.
func (db *DB) GetUserByWebhookToken(ctx context.Context, token string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `SELECT id FROM users WHERE webhook_token = $1`, token).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// GetOrCreateFeedToken returns the user's feed token, generating one if it doesn't exist.
func (db *DB) GetOrCreateFeedToken(ctx context.Context, userID int) (string, error) {
	var token *string
	err := db.Pool.QueryRow(ctx, `SELECT feed_token FROM users WHERE id = $1`, userID).Scan(&token)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}
	if token != nil && *token != "" {
		return *token, nil
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	newToken := hex.EncodeToString(b)

	_, err = db.Pool.Exec(ctx, `UPDATE users SET feed_token = $1 WHERE id = $2`, newToken, userID)
	if err != nil {
		return "", fmt.Errorf("save token: %w", err)
	}
	return newToken, nil
}

// GetUserByFeedToken looks up a user ID by feed token. Returns ErrNotFound if no
// user carries it — which is what turns an invalid feed URL into a 404 rather
// than a 403, per CLAUDE.md.
func (db *DB) GetUserByFeedToken(ctx context.Context, token string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `SELECT id FROM users WHERE feed_token = $1`, token).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// SetKarakeepAPIKey stores a user's Karakeep API key.
//
// In plain text. An earlier version of this comment claimed the key was
// encrypted; there is no crypto anywhere in this repo, and the same holds for
// the API keys in app_settings. DECISIONS.md carries why that is accepted and
// what would overturn it.
func (db *DB) SetKarakeepAPIKey(ctx context.Context, userID int, apiKey string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE users SET karakeep_api_key = $1 WHERE id = $2`, apiKey, userID)
	return err
}

// GetKarakeepAPIKey retrieves the Karakeep API key for a user. Returns empty string if not set.
func (db *DB) GetKarakeepAPIKey(ctx context.Context, userID int) (string, error) {
	var key *string
	err := db.Pool.QueryRow(ctx, `SELECT karakeep_api_key FROM users WHERE id = $1`, userID).Scan(&key)
	if err != nil {
		return "", err
	}
	if key == nil {
		return "", nil
	}
	return *key, nil
}

// GetSummaryPrompts returns the user's custom summary prompts. Nil means use default.
func (db *DB) GetSummaryPrompts(ctx context.Context, userID int) (medium, deep *string, err error) {
	err = db.Pool.QueryRow(ctx,
		`SELECT summary_prompt_medium, summary_prompt_deep FROM users WHERE id = $1`, userID,
	).Scan(&medium, &deep)
	return
}

// GetModelPreference returns the preferred summary model (provider + model ID).
// Empty means use default.
//
// The preference is shared across users for the same reason the prompts are —
// one summary per piece of content, so the model that produced it cannot be a
// private choice. A row that has none falls back to another user's, which is
// what a user created after the setting was made reads. See prompts.go for the
// full reasoning.
func (db *DB) GetModelPreference(ctx context.Context, userID int) (provider, model string, err error) {
	var pp, pm *string
	err = db.Pool.QueryRow(ctx, `
		SELECT preferred_model_provider, preferred_model_id
		FROM users
		ORDER BY (id = $1) DESC, (preferred_model_id IS NOT NULL) DESC, id
		LIMIT 1
	`, userID).Scan(&pp, &pm)
	if err != nil {
		return "", "", err
	}
	if pp != nil {
		provider = *pp
	}
	if pm != nil {
		model = *pm
	}
	return
}

// SetModelPreference sets the preferred summary model for every user. Empty
// values reset to default (NULL). The userID argument names who asked; it does
// not narrow what is written.
func (db *DB) SetModelPreference(ctx context.Context, userID int, provider, model string) error {
	var pp, pm *string
	if provider != "" {
		pp = &provider
	}
	if model != "" {
		pm = &model
	}
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET preferred_model_provider = $1, preferred_model_id = $2`,
		pp, pm,
	)
	return err
}

// SetSummaryPrompt sets a custom summary prompt for the given level. Empty string resets to default (NULL).
func (db *DB) SetSummaryPrompt(ctx context.Context, userID int, level, prompt string) error {
	col := "summary_prompt_medium"
	if level == "deep" {
		col = "summary_prompt_deep"
	}
	var val *string
	if prompt != "" {
		val = &prompt
	}
	_, err := db.Pool.Exec(ctx, fmt.Sprintf(`UPDATE users SET %s = $1 WHERE id = $2`, col), val, userID)
	return err
}

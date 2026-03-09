package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func (db *DB) GetPrimaryUser(ctx context.Context) (id int, login string, err error) {
	err = db.Pool.QueryRow(ctx, `
		SELECT id, login FROM users
		WHERE login LIKE '%@%'
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&id, &login)
	if err == pgx.ErrNoRows {
		return 0, "", pgx.ErrNoRows
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

// GetUserByWebhookToken looks up a user ID by webhook token. Returns pgx.ErrNoRows if not found.
func (db *DB) GetUserByWebhookToken(ctx context.Context, token string) (int, error) {
	var id int
	err := db.Pool.QueryRow(ctx, `SELECT id FROM users WHERE webhook_token = $1`, token).Scan(&id)
	return id, err
}

// SetKarakeepAPIKey stores an encrypted Karakeep API key for a user.
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

package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Keys in app_settings. They are named here rather than at the call sites so a
// typo is a compile error instead of a silently empty setting.
const (
	SettingMistralAPIKey   = "mistral_api_key"
	SettingAnthropicAPIKey = "anthropic_api_key"
	SettingSummaryProvider = "summary_provider"
)

// GetLLMKey returns a summary provider's API key, empty when unset. It is the
// one place that maps a provider name to the setting holding its key, so the
// model registry, the Mistral client and the summarizers cannot drift apart on
// which setting they read.
func (db *DB) GetLLMKey(ctx context.Context, provider string) (string, error) {
	var key string
	switch provider {
	case "claude":
		key = SettingAnthropicAPIKey
	case "mistral":
		key = SettingMistralAPIKey
	default:
		return "", nil
	}
	return db.GetAppSetting(ctx, key)
}

// GetAppSetting returns a service-wide setting. An unset key returns an empty
// string and no error — every caller treats "unset" as "fall back", and an
// error there would turn a missing API key into a failed request instead of a
// disabled provider.
func (db *DB) GetAppSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := db.Pool.QueryRow(ctx,
		`SELECT value FROM app_settings WHERE key = $1`, key).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get app setting %q: %w", key, err)
	}
	return value, nil
}

// SetAppSetting stores a service-wide setting. An empty value deletes the row,
// which is how a provider is switched off again — unlike the Karakeep key,
// which has no reset path, the Anthropic key is expected to be left empty.
func (db *DB) SetAppSetting(ctx context.Context, key, value string) error {
	if value == "" {
		if _, err := db.Pool.Exec(ctx,
			`DELETE FROM app_settings WHERE key = $1`, key); err != nil {
			return fmt.Errorf("clear app setting %q: %w", key, err)
		}
		return nil
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO app_settings (key, value)
		VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value, updated_at = NOW()
	`, key, value)
	if err != nil {
		return fmt.Errorf("set app setting %q: %w", key, err)
	}
	return nil
}

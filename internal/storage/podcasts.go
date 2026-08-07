package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PodcastSubscription is one user's subscription to one cast2md feed.
type PodcastSubscription struct {
	ID          int    `json:"id"`
	UserID      int    `json:"user_id"`
	FeedID      string `json:"feed_id"`
	FeedTitle   string `json:"feed_title"`
	ImageURL    string `json:"image_url,omitempty"`
	Enabled     bool   `json:"enabled"`
	DetailLevel string `json:"detail_level"`
	// InitialBackfill is how many of the feed's newest completed episodes the
	// first poll summarizes. 0 means "from now on" — watermark only.
	InitialBackfill int        `json:"initial_backfill"`
	Initialized     bool       `json:"initialized"`
	Watermark       string     `json:"watermark"`
	LastPolledAt    *time.Time `json:"last_polled_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

const subscriptionColumns = `id, user_id, feed_id, feed_title, COALESCE(image_url, ''), ` +
	`enabled, detail_level, initial_backfill, initialized, watermark, last_polled_at, ` +
	`COALESCE(last_error, ''), created_at, updated_at`

func scanSubscription(row rowScanner) (*PodcastSubscription, error) {
	var s PodcastSubscription
	err := row.Scan(&s.ID, &s.UserID, &s.FeedID, &s.FeedTitle, &s.ImageURL,
		&s.Enabled, &s.DetailLevel, &s.InitialBackfill, &s.Initialized, &s.Watermark,
		&s.LastPolledAt, &s.LastError, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func scanSubscriptions(rows pgx.Rows) ([]PodcastSubscription, error) {
	defer rows.Close()
	var subs []PodcastSubscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan podcast subscription: %w", err)
		}
		subs = append(subs, *s)
	}
	return subs, rows.Err()
}

// ListSubscriptions returns every subscription of one user, enabled or not.
func (db *DB) ListSubscriptions(ctx context.Context, userID int) ([]PodcastSubscription, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+subscriptionColumns+` FROM podcast_subscriptions
		 WHERE user_id = $1 ORDER BY feed_title, feed_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list podcast subscriptions: %w", err)
	}
	return scanSubscriptions(rows)
}

// ListEnabledSubscriptions returns the enabled subscriptions of every user —
// the poller's work list.
func (db *DB) ListEnabledSubscriptions(ctx context.Context) ([]PodcastSubscription, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+subscriptionColumns+` FROM podcast_subscriptions
		 WHERE enabled ORDER BY user_id, id`)
	if err != nil {
		return nil, fmt.Errorf("list enabled subscriptions: %w", err)
	}
	return scanSubscriptions(rows)
}

func (db *DB) GetSubscription(ctx context.Context, userID int, feedID string) (*PodcastSubscription, error) {
	return scanSubscription(db.Pool.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM podcast_subscriptions
		 WHERE user_id = $1 AND feed_id = $2`, userID, feedID))
}

// UpsertSubscription writes the user-controlled fields. Enabling a feed that
// was never initialized leaves initialized false, so the poller still performs
// its first run — which is where initial_backfill takes effect.
func (db *DB) UpsertSubscription(ctx context.Context, userID int, feedID, feedTitle, imageURL string, enabled bool, detailLevel string, initialBackfill int) (*PodcastSubscription, error) {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO podcast_subscriptions (user_id, feed_id, feed_title, image_url, enabled, detail_level, initial_backfill)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7)
		ON CONFLICT (user_id, feed_id) DO UPDATE SET
			feed_title       = EXCLUDED.feed_title,
			image_url        = EXCLUDED.image_url,
			enabled          = EXCLUDED.enabled,
			detail_level     = EXCLUDED.detail_level,
			initial_backfill = EXCLUDED.initial_backfill,
			updated_at       = NOW()
	`, userID, feedID, feedTitle, imageURL, enabled, detailLevel, initialBackfill)
	if err != nil {
		return nil, fmt.Errorf("upsert podcast subscription: %w", err)
	}
	return db.GetSubscription(ctx, userID, feedID)
}

// SetSubscriptionWatermark advances the watermark and clears the last error.
// The value is cast2md's own updated_at, carried through as opaque text.
func (db *DB) SetSubscriptionWatermark(ctx context.Context, id int, watermark string, initialized bool) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE podcast_subscriptions
		SET watermark = $2, initialized = $3, last_polled_at = NOW(),
		    last_error = NULL, updated_at = NOW()
		WHERE id = $1
	`, id, watermark, initialized)
	if err != nil {
		return fmt.Errorf("set subscription watermark: %w", err)
	}
	return nil
}

// SetSubscriptionError records a failed poll. The watermark stays where it is,
// so the next tick retries the same range.
func (db *DB) SetSubscriptionError(ctx context.Context, id int, message string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE podcast_subscriptions
		SET last_error = $2, last_polled_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id, message)
	if err != nil {
		return fmt.Errorf("set subscription error: %w", err)
	}
	return nil
}

// CountSummarizedByFeed returns, per cast2md feed ID, how many podcast rows the
// user has.
func (db *DB) CountSummarizedByFeed(ctx context.Context, userID int) (map[string]int, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT source_feed_id, COUNT(*) FROM videos
		WHERE user_id = $1 AND source = 'podcast' AND source_feed_id IS NOT NULL
		GROUP BY source_feed_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("count summarized by feed: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var feedID string
		var count int
		if err := rows.Scan(&feedID, &count); err != nil {
			return nil, fmt.Errorf("scan feed count: %w", err)
		}
		counts[feedID] = count
	}
	return counts, rows.Err()
}

package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Inbox item states. A row never leaves the table on triage — dismissed and
// queued rows are the dedup memory that keeps the poller from re-inserting a
// video that is still inside the feed's window.
const (
	InboxStateNew       = "new"
	InboxStateQueued    = "queued"
	InboxStateDismissed = "dismissed"
)

// ChannelSubscription is one user's followed YouTube channel.
type ChannelSubscription struct {
	ID           int        `json:"id"`
	UserID       int        `json:"user_id"`
	ChannelID    string     `json:"channel_id"`
	Title        string     `json:"title"`
	ThumbnailURL string     `json:"thumbnail_url,omitempty"`
	Enabled      bool       `json:"enabled"`
	LastPolledAt *time.Time `json:"last_polled_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// NewCount is filled by ListChannelSubscriptions only.
	NewCount int `json:"new_count"`
}

// InboxItem is one video awaiting triage.
type InboxItem struct {
	ID             int        `json:"id"`
	SubscriptionID int        `json:"subscription_id"`
	UserID         int        `json:"user_id"`
	YouTubeID      string     `json:"youtube_id"`
	Title          string     `json:"title"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	State          string     `json:"state"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// ChannelTitle is joined in by the list query for display.
	ChannelTitle string `json:"channel_title"`
}

const channelSubColumns = `id, user_id, channel_id, title, COALESCE(thumbnail_url, ''), ` +
	`enabled, last_polled_at, COALESCE(last_error, ''), created_at, updated_at`

func scanChannelSubscription(row rowScanner) (*ChannelSubscription, error) {
	var s ChannelSubscription
	err := row.Scan(&s.ID, &s.UserID, &s.ChannelID, &s.Title, &s.ThumbnailURL,
		&s.Enabled, &s.LastPolledAt, &s.LastError, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

// ListChannelSubscriptions returns every channel one user follows, with the
// count of items still awaiting triage.
func (db *DB) ListChannelSubscriptions(ctx context.Context, userID int) ([]ChannelSubscription, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+channelSubColumns+`,
			(SELECT COUNT(*) FROM inbox_items i
			 WHERE i.subscription_id = channel_subscriptions.id AND i.state = 'new') AS new_count
		FROM channel_subscriptions
		WHERE user_id = $1 ORDER BY title, channel_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list channel subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []ChannelSubscription
	for rows.Next() {
		var s ChannelSubscription
		err := rows.Scan(&s.ID, &s.UserID, &s.ChannelID, &s.Title, &s.ThumbnailURL,
			&s.Enabled, &s.LastPolledAt, &s.LastError, &s.CreatedAt, &s.UpdatedAt, &s.NewCount)
		if err != nil {
			return nil, fmt.Errorf("scan channel subscription: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// ListEnabledChannelSubscriptions returns the enabled subscriptions of every
// user — the poller's work list.
func (db *DB) ListEnabledChannelSubscriptions(ctx context.Context) ([]ChannelSubscription, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+channelSubColumns+` FROM channel_subscriptions
		 WHERE enabled ORDER BY user_id, id`)
	if err != nil {
		return nil, fmt.Errorf("list enabled channel subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []ChannelSubscription
	for rows.Next() {
		s, err := scanChannelSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan channel subscription: %w", err)
		}
		subs = append(subs, *s)
	}
	return subs, rows.Err()
}

func (db *DB) GetChannelSubscription(ctx context.Context, userID, id int) (*ChannelSubscription, error) {
	return scanChannelSubscription(db.Pool.QueryRow(ctx,
		`SELECT `+channelSubColumns+` FROM channel_subscriptions
		 WHERE id = $1 AND user_id = $2`, id, userID))
}

// UpsertChannelSubscription creates or revives a subscription. Re-following a
// channel re-enables it and refreshes title and artwork.
func (db *DB) UpsertChannelSubscription(ctx context.Context, userID int, channelID, title, thumbnailURL string) (*ChannelSubscription, error) {
	row := db.Pool.QueryRow(ctx, `
		INSERT INTO channel_subscriptions (user_id, channel_id, title, thumbnail_url)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		ON CONFLICT (user_id, channel_id) DO UPDATE SET
			title         = EXCLUDED.title,
			thumbnail_url = EXCLUDED.thumbnail_url,
			enabled       = TRUE,
			updated_at    = NOW()
		RETURNING `+channelSubColumns, userID, channelID, title, thumbnailURL)
	sub, err := scanChannelSubscription(row)
	if err != nil {
		return nil, fmt.Errorf("upsert channel subscription: %w", err)
	}
	return sub, nil
}

func (db *DB) SetChannelEnabled(ctx context.Context, userID, id int, enabled bool) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE channel_subscriptions SET enabled = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2`, id, userID, enabled)
	if err != nil {
		return fmt.Errorf("set channel enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteChannelSubscription unfollows a channel. Its inbox items go with it
// via the cascade, so re-following re-imports the feed's current window.
func (db *DB) DeleteChannelSubscription(ctx context.Context, userID, id int) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM channel_subscriptions WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete channel subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetChannelPolled records a successful poll and clears the last error.
func (db *DB) SetChannelPolled(ctx context.Context, id int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE channel_subscriptions
		SET last_polled_at = NOW(), last_error = NULL, updated_at = NOW()
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("set channel polled: %w", err)
	}
	return nil
}

// SetChannelError records a failed poll.
func (db *DB) SetChannelError(ctx context.Context, id int, message string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE channel_subscriptions
		SET last_error = $2, last_polled_at = NOW(), updated_at = NOW()
		WHERE id = $1`, id, message)
	if err != nil {
		return fmt.Errorf("set channel error: %w", err)
	}
	return nil
}

// InsertInboxItem writes one item and reports whether it was new. A conflict
// on (user_id, youtube_id) is the dedup working, not an error.
func (db *DB) InsertInboxItem(ctx context.Context, item *InboxItem) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `
		INSERT INTO inbox_items (subscription_id, user_id, youtube_id, title, published_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, youtube_id) DO NOTHING`,
		item.SubscriptionID, item.UserID, item.YouTubeID, item.Title, item.PublishedAt)
	if err != nil {
		return false, fmt.Errorf("insert inbox item: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

const inboxItemColumns = `i.id, i.subscription_id, i.user_id, i.youtube_id, i.title, ` +
	`i.published_at, i.state, i.created_at, i.updated_at, s.title`

func scanInboxItem(row rowScanner) (*InboxItem, error) {
	var i InboxItem
	err := row.Scan(&i.ID, &i.SubscriptionID, &i.UserID, &i.YouTubeID, &i.Title,
		&i.PublishedAt, &i.State, &i.CreatedAt, &i.UpdatedAt, &i.ChannelTitle)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &i, nil
}

// ListInboxItems returns one user's items in one state, newest first, plus the
// total for that state. subscriptionID narrows to one channel; 0 means all.
func (db *DB) ListInboxItems(ctx context.Context, userID int, state string, subscriptionID, limit, offset int) ([]InboxItem, int, error) {
	if state == "" {
		state = InboxStateNew
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	where := `i.user_id = $1 AND i.state = $2`
	args := []any{userID, state}
	if subscriptionID > 0 {
		where += ` AND i.subscription_id = $3`
		args = append(args, subscriptionID)
	}

	var total int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM inbox_items i WHERE `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count inbox items: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT `+inboxItemColumns+`
		FROM inbox_items i
		JOIN channel_subscriptions s ON s.id = i.subscription_id
		WHERE `+where+`
		ORDER BY i.published_at DESC NULLS LAST, i.id DESC
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list inbox items: %w", err)
	}
	defer rows.Close()

	var items []InboxItem
	for rows.Next() {
		item, err := scanInboxItem(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan inbox item: %w", err)
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (db *DB) GetInboxItem(ctx context.Context, userID, id int) (*InboxItem, error) {
	return scanInboxItem(db.Pool.QueryRow(ctx, `
		SELECT `+inboxItemColumns+`
		FROM inbox_items i
		JOIN channel_subscriptions s ON s.id = i.subscription_id
		WHERE i.id = $1 AND i.user_id = $2`, id, userID))
}

func (db *DB) SetInboxItemState(ctx context.Context, userID, id int, state string) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE inbox_items SET state = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2`, id, userID, state)
	if err != nil {
		return fmt.Errorf("set inbox item state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DismissAllInbox dismisses every new item of one user and returns how many.
func (db *DB) DismissAllInbox(ctx context.Context, userID int) (int, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE inbox_items SET state = 'dismissed', updated_at = NOW()
		WHERE user_id = $1 AND state = 'new'`, userID)
	if err != nil {
		return 0, fmt.Errorf("dismiss all inbox items: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

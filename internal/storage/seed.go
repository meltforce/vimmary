package storage

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// fixturesJSON is the preview data set: rows of both sources, a followed
// channel with inbox items and a podcast subscription. It contains no
// embedding, so semantic search finds none of it; keyword search does.
//
//go:embed fixtures/preview.json
var fixturesJSON []byte

// Fixtures is the shape of fixtures/preview.json.
type Fixtures struct {
	Videos   []Video `json:"videos"`
	Channels []struct {
		ChannelID    string `json:"channel_id"`
		Title        string `json:"title"`
		ThumbnailURL string `json:"thumbnail_url"`
		Enabled      bool   `json:"enabled"`
		Inbox        []struct {
			YouTubeID   string    `json:"youtube_id"`
			Title       string    `json:"title"`
			PublishedAt time.Time `json:"published_at"`
		} `json:"inbox"`
	} `json:"channels"`
	PodcastSubscriptions []struct {
		FeedID      string `json:"feed_id"`
		FeedTitle   string `json:"feed_title"`
		ImageURL    string `json:"image_url"`
		DetailLevel string `json:"detail_level"`
		Watermark   string `json:"watermark"`
	} `json:"podcast_subscriptions"`
}

// LoadFixtures parses the embedded preview data set.
func LoadFixtures() (*Fixtures, error) {
	var f Fixtures
	if err := json.Unmarshal(fixturesJSON, &f); err != nil {
		return nil, fmt.Errorf("parse fixtures: %w", err)
	}
	return &f, nil
}

// SeedFixtures writes the embedded data set for userID when the videos table
// is empty, and reports whether it wrote anything. A table with any row, of
// any user, is left alone, so a restart never duplicates the set and a real
// database is never touched. Everything is written in one transaction.
//
// Inbox items carry shorts_checked_at, and the channel is stored as given in
// the fixtures, so the seed itself starts no YouTube request.
func (db *DB) SeedFixtures(ctx context.Context, userID int) (bool, error) {
	f, err := LoadFixtures()
	if err != nil {
		return false, err
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Two instances starting at once on an empty database would both see it
	// empty; the lock makes the second wait and then see the first's rows.
	if _, err := tx.Exec(ctx, `LOCK TABLE videos IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return false, fmt.Errorf("lock videos: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM videos)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check videos: %w", err)
	}
	if exists {
		return false, nil
	}

	for i := range f.Videos {
		v := f.Videos[i]
		v.ID = uuid.New()
		v.UserID = userID
		v.Status = "completed"
		v.normalize()
		if _, err := tx.Exec(ctx, insertVideoSQL, v.insertArgs()...); err != nil {
			return false, fmt.Errorf("insert fixture %q: %w", v.Title, err)
		}
	}
	// Rows inserted in one transaction share NOW() as created_at, which leaves
	// the list order to chance; the publication date gives them a fixed one.
	if _, err := tx.Exec(ctx, `
		UPDATE videos SET created_at = published_at, updated_at = published_at
		WHERE user_id = $1 AND published_at IS NOT NULL`, userID); err != nil {
		return false, fmt.Errorf("date fixtures: %w", err)
	}

	for _, c := range f.Channels {
		var subID int
		err := tx.QueryRow(ctx, `
			INSERT INTO channel_subscriptions (user_id, channel_id, title, thumbnail_url, enabled)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5)
			RETURNING id`,
			userID, c.ChannelID, c.Title, c.ThumbnailURL, c.Enabled).Scan(&subID)
		if err != nil {
			return false, fmt.Errorf("insert fixture channel %q: %w", c.Title, err)
		}
		for _, it := range c.Inbox {
			if _, err := tx.Exec(ctx, `
				INSERT INTO inbox_items (subscription_id, user_id, youtube_id, title, published_at, shorts_checked_at)
				VALUES ($1, $2, $3, $4, $5, NOW())`,
				subID, userID, it.YouTubeID, it.Title, it.PublishedAt); err != nil {
				return false, fmt.Errorf("insert fixture inbox item %q: %w", it.Title, err)
			}
		}
	}

	for _, p := range f.PodcastSubscriptions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO podcast_subscriptions
				(user_id, feed_id, feed_title, image_url, enabled, detail_level, initialized, watermark)
			VALUES ($1, $2, $3, NULLIF($4, ''), TRUE, $5, TRUE, $6)`,
			userID, p.FeedID, p.FeedTitle, p.ImageURL, p.DetailLevel, p.Watermark); err != nil {
			return false, fmt.Errorf("insert fixture subscription %q: %w", p.FeedTitle, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit seed: %w", err)
	}
	return true, nil
}

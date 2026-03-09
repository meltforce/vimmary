package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
)

type Video struct {
	ID                  uuid.UUID       `json:"id"`
	UserID              int             `json:"user_id"`
	KarakeepBookmarkID  string          `json:"karakeep_bookmark_id,omitempty"`
	YouTubeID           string          `json:"youtube_id"`
	Title               string          `json:"title"`
	Channel             string          `json:"channel"`
	DurationSeconds     int             `json:"duration_seconds,omitempty"`
	Language            string          `json:"language,omitempty"`
	Transcript          string          `json:"transcript,omitempty"`
	Summary             string          `json:"summary,omitempty"`
	DetailLevel         string          `json:"detail_level"`
	Metadata            json.RawMessage `json:"metadata"`
	Status              string          `json:"status"`
	ErrorMessage        string          `json:"error_message,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type VideoMatch struct {
	ID         uuid.UUID       `json:"id"`
	YouTubeID  string          `json:"youtube_id"`
	Title      string          `json:"title"`
	Channel    string          `json:"channel"`
	Summary    string          `json:"summary"`
	Metadata   json.RawMessage `json:"metadata"`
	Similarity float64         `json:"similarity"`
	CreatedAt  time.Time       `json:"created_at"`
}

type VideoStats struct {
	TotalCount    int            `json:"total_count"`
	ByStatus      map[string]int `json:"by_status"`
	ByChannel     []ChannelCount `json:"by_channel"`
	TopTopics     []TopicCount   `json:"top_topics"`
	DailyActivity []DailyCount  `json:"daily_activity"`
}

type ChannelCount struct {
	Channel string `json:"channel"`
	Count   int    `json:"count"`
}

type TopicCount struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func (db *DB) InsertVideo(ctx context.Context, v *Video) error {
	var embeddingArg any
	if v.Metadata == nil {
		v.Metadata = json.RawMessage(`{}`)
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO videos (id, user_id, karakeep_bookmark_id, youtube_id, title, channel,
			duration_seconds, language, transcript, summary, detail_level, embedding, metadata, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, v.ID, v.UserID, v.KarakeepBookmarkID, v.YouTubeID, v.Title, v.Channel,
		v.DurationSeconds, v.Language, v.Transcript, v.Summary, v.DetailLevel,
		embeddingArg, v.Metadata, v.Status)
	if err != nil {
		return fmt.Errorf("insert video: %w", err)
	}
	return nil
}

func (db *DB) GetByYouTubeID(ctx context.Context, userID int, youtubeID string) (*Video, error) {
	var v Video
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, karakeep_bookmark_id, youtube_id, title, channel,
			duration_seconds, language, transcript, summary, detail_level, metadata,
			status, COALESCE(error_message, ''), created_at, updated_at
		FROM videos WHERE user_id = $1 AND youtube_id = $2
	`, userID, youtubeID).Scan(&v.ID, &v.UserID, &v.KarakeepBookmarkID, &v.YouTubeID,
		&v.Title, &v.Channel, &v.DurationSeconds, &v.Language, &v.Transcript,
		&v.Summary, &v.DetailLevel, &v.Metadata, &v.Status, &v.ErrorMessage,
		&v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (db *DB) UpdateBookmarkID(ctx context.Context, id uuid.UUID, bookmarkID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE videos SET karakeep_bookmark_id = $1, updated_at = NOW() WHERE id = $2`,
		bookmarkID, id)
	return err
}

func (db *DB) GetVideo(ctx context.Context, userID int, id uuid.UUID) (*Video, error) {
	var v Video
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, karakeep_bookmark_id, youtube_id, title, channel,
			duration_seconds, language, transcript, summary, detail_level, metadata,
			status, COALESCE(error_message, ''), created_at, updated_at
		FROM videos WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&v.ID, &v.UserID, &v.KarakeepBookmarkID, &v.YouTubeID,
		&v.Title, &v.Channel, &v.DurationSeconds, &v.Language, &v.Transcript,
		&v.Summary, &v.DetailLevel, &v.Metadata, &v.Status, &v.ErrorMessage,
		&v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (db *DB) UpdateVideoSummary(ctx context.Context, id uuid.UUID, summary string, detailLevel string, embedding []float32, metadata json.RawMessage) error {
	var embeddingArg any
	if embedding != nil {
		embeddingArg = pgvector.NewVector(embedding)
	}
	tag, err := db.Pool.Exec(ctx, `
		UPDATE videos SET summary = $1, detail_level = $2, embedding = $3,
			metadata = $4, status = 'completed', error_message = NULL, updated_at = NOW()
		WHERE id = $5
	`, summary, detailLevel, embeddingArg, metadata, id)
	if err != nil {
		return fmt.Errorf("update video summary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (db *DB) UpdateVideoTranscript(ctx context.Context, id uuid.UUID, transcript, title, channel, language string, durationSeconds int) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE videos SET transcript = $1, title = $2, channel = $3, language = $4,
			duration_seconds = $5, updated_at = NOW()
		WHERE id = $6
	`, transcript, title, channel, language, durationSeconds, id)
	if err != nil {
		return fmt.Errorf("update video transcript: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (db *DB) UpdateVideoStatus(ctx context.Context, id uuid.UUID, status, errorMsg string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE videos SET status = $1, error_message = $2, updated_at = NOW()
		WHERE id = $3
	`, status, errorMsg, id)
	return err
}

func (db *DB) DeleteVideo(ctx context.Context, userID int, id uuid.UUID) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM videos WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete video: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (db *DB) DeleteByBookmarkID(ctx context.Context, userID int, bookmarkID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM videos WHERE karakeep_bookmark_id = $1 AND user_id = $2`, bookmarkID, userID)
	return err
}

func (db *DB) SearchVideos(ctx context.Context, userID int, embedding []float32, threshold float64, limit int) ([]VideoMatch, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, youtube_id, title, channel, summary, metadata, similarity, created_at
		FROM match_videos($1, $2, $3, $4)
	`, pgvector.NewVector(embedding), userID, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("search videos: %w", err)
	}
	defer rows.Close()

	var matches []VideoMatch
	for rows.Next() {
		var m VideoMatch
		if err := rows.Scan(&m.ID, &m.YouTubeID, &m.Title, &m.Channel, &m.Summary,
			&m.Metadata, &m.Similarity, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan video match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func (db *DB) TextSearchVideos(ctx context.Context, userID int, query string, limit int) ([]VideoMatch, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, youtube_id, title, channel, summary, metadata, 0.0::float8 AS similarity, created_at
		FROM videos
		WHERE user_id = $1
		  AND status = 'completed'
		  AND (
		    title ILIKE '%' || $2 || '%'
		    OR channel ILIKE '%' || $2 || '%'
		    OR summary ILIKE '%' || $2 || '%'
		    OR transcript ILIKE '%' || $2 || '%'
		    OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(metadata->'topics') t WHERE t ILIKE '%' || $2 || '%')
		  )
		ORDER BY created_at DESC
		LIMIT $3
	`, userID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("text search videos: %w", err)
	}
	defer rows.Close()

	var matches []VideoMatch
	for rows.Next() {
		var m VideoMatch
		if err := rows.Scan(&m.ID, &m.YouTubeID, &m.Title, &m.Channel, &m.Summary,
			&m.Metadata, &m.Similarity, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan text match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

type ListFilters struct {
	Channel  string
	Language string
	Topic    string
	Status   string
}

func (db *DB) ListRecent(ctx context.Context, userID int, filters ListFilters, limit, offset int) ([]Video, int, error) {
	statusFilter := "status IN ('completed', 'failed', 'processing', 'pending')"
	if filters.Status != "" {
		statusFilter = fmt.Sprintf("status = '%s'", filters.Status)
	}
	query := fmt.Sprintf(`SELECT id, user_id, karakeep_bookmark_id, youtube_id, title, channel,
		duration_seconds, language, '', summary, detail_level, metadata,
		status, COALESCE(error_message, ''), created_at, updated_at
		FROM videos WHERE user_id = $1 AND %s`, statusFilter)
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM videos WHERE user_id = $1 AND %s`, statusFilter)
	args := []any{userID}
	argN := 2

	if filters.Channel != "" {
		clause := fmt.Sprintf(` AND channel ILIKE '%%' || $%d || '%%'`, argN)
		query += clause
		countQuery += clause
		args = append(args, filters.Channel)
		argN++
	}
	if filters.Language != "" {
		clause := fmt.Sprintf(` AND language = $%d`, argN)
		query += clause
		countQuery += clause
		args = append(args, filters.Language)
		argN++
	}
	if filters.Topic != "" {
		clause := fmt.Sprintf(` AND metadata->'topics' ? $%d`, argN)
		query += clause
		countQuery += clause
		args = append(args, filters.Topic)
		argN++
	}

	var total int
	if err := db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count videos: %w", err)
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list videos: %w", err)
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.UserID, &v.KarakeepBookmarkID, &v.YouTubeID,
			&v.Title, &v.Channel, &v.DurationSeconds, &v.Language, &v.Transcript,
			&v.Summary, &v.DetailLevel, &v.Metadata, &v.Status, &v.ErrorMessage,
			&v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan video: %w", err)
		}
		videos = append(videos, v)
	}
	return videos, total, rows.Err()
}

func (db *DB) GetStats(ctx context.Context, userID int) (*VideoStats, error) {
	stats := &VideoStats{
		ByStatus: make(map[string]int),
	}

	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM videos WHERE user_id = $1`, userID).Scan(&stats.TotalCount); err != nil {
		return nil, fmt.Errorf("count videos: %w", err)
	}

	// By status
	rows, err := db.Pool.Query(ctx, `
		SELECT status, COUNT(*) FROM videos WHERE user_id = $1
		GROUP BY status ORDER BY COUNT(*) DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("status counts: %w", err)
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.ByStatus[status] = count
	}
	rows.Close()

	// By channel
	rows, err = db.Pool.Query(ctx, `
		SELECT COALESCE(channel, 'unknown'), COUNT(*) as cnt
		FROM videos WHERE user_id = $1 AND status = 'completed'
		GROUP BY channel ORDER BY cnt DESC LIMIT 10
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("channel counts: %w", err)
	}
	for rows.Next() {
		var cc ChannelCount
		if err := rows.Scan(&cc.Channel, &cc.Count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.ByChannel = append(stats.ByChannel, cc)
	}
	rows.Close()

	// Top topics
	rows, err = db.Pool.Query(ctx, `
		SELECT topic, COUNT(*) as cnt
		FROM videos, jsonb_array_elements_text(metadata->'topics') AS topic
		WHERE user_id = $1 AND status = 'completed'
		GROUP BY topic ORDER BY cnt DESC LIMIT 10
	`, userID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("top topics: %w", err)
	}
	for rows.Next() {
		var tc TopicCount
		if err := rows.Scan(&tc.Topic, &tc.Count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.TopTopics = append(stats.TopTopics, tc)
	}
	rows.Close()

	// Daily activity (last 30 days)
	rows, err = db.Pool.Query(ctx, `
		SELECT created_at::date::text AS day, COUNT(*)
		FROM videos
		WHERE user_id = $1 AND created_at >= NOW() - INTERVAL '30 days'
		GROUP BY day ORDER BY day
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("daily activity: %w", err)
	}
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.DailyActivity = append(stats.DailyActivity, dc)
	}
	rows.Close()

	if stats.ByChannel == nil {
		stats.ByChannel = []ChannelCount{}
	}
	if stats.TopTopics == nil {
		stats.TopTopics = []TopicCount{}
	}
	if stats.DailyActivity == nil {
		stats.DailyActivity = []DailyCount{}
	}

	return stats, nil
}

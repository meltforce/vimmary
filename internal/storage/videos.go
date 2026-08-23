package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
)

// Content sources. `videos` holds both kinds of row, discriminated by this column.
const (
	SourceYouTube = "youtube"
	SourcePodcast = "podcast"
)

type Video struct {
	ID                  uuid.UUID       `json:"id"`
	UserID              int             `json:"user_id"`
	KarakeepBookmarkID  string          `json:"karakeep_bookmark_id,omitempty"`
	YouTubeID           string          `json:"youtube_id"`
	Source              string          `json:"source"`
	ExternalID          string          `json:"external_id"`
	SourceURL           string          `json:"source_url,omitempty"`
	SourceFeedID        string          `json:"source_feed_id,omitempty"`
	ThumbnailURL        string          `json:"thumbnail_url,omitempty"`
	PublishedAt         *time.Time      `json:"published_at,omitempty"`
	Title               string          `json:"title"`
	Channel             string          `json:"channel"`
	DurationSeconds     int             `json:"duration_seconds,omitempty"`
	Language            string          `json:"language,omitempty"`
	Transcript          string          `json:"transcript,omitempty"`
	Summary             string          `json:"summary,omitempty"`
	DetailLevel         string          `json:"detail_level"`
	SummaryProvider     string          `json:"summary_provider,omitempty"`
	SummaryModel        string          `json:"summary_model,omitempty"`
	SummaryInputTokens  int             `json:"summary_input_tokens,omitempty"`
	SummaryOutputTokens int             `json:"summary_output_tokens,omitempty"`
	Metadata            json.RawMessage `json:"metadata"`
	Status              string          `json:"status"`
	ErrorMessage        string          `json:"error_message,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// TranscriptSegment is one timed caption line as stored in
// videos.transcript_segments and served to the player. The keys are compact
// because a three-hour video carries thousands of these on every player load:
// s = start seconds, d = duration seconds, t = text.
type TranscriptSegment struct {
	Start    float64 `json:"s"`
	Duration float64 `json:"d"`
	Text     string  `json:"t"`
}

type VideoMatch struct {
	ID         uuid.UUID       `json:"id"`
	YouTubeID  string          `json:"youtube_id"`
	Source     string          `json:"source"`
	SourceURL  string          `json:"source_url,omitempty"`
	Title      string          `json:"title"`
	Channel    string          `json:"channel"`
	Summary    string          `json:"summary"`
	Metadata   json.RawMessage `json:"metadata"`
	Similarity float64         `json:"similarity"`
	CreatedAt  time.Time       `json:"created_at"`
}

type VideoStats struct {
	TotalCount           int            `json:"total_count"`
	TotalDurationSeconds int64          `json:"total_duration_seconds"`
	ByStatus             map[string]int `json:"by_status"`
	BySource             map[string]int `json:"by_source"`
	ByChannel            []ChannelCount `json:"by_channel"`
	TopTopics            []TopicCount   `json:"top_topics"`
	DailyActivity        []DailyCount   `json:"daily_activity"`
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

// videoColumns is the SELECT list for every Video read, in the order scanVideo
// expects. youtube_id is NULL on podcast rows, so it is coalesced and
// Video.YouTubeID stays a plain string.
const videoColumns = `id, user_id, karakeep_bookmark_id, COALESCE(youtube_id, ''), ` +
	`title, channel, duration_seconds, language, transcript, summary, detail_level, ` +
	`summary_provider, summary_model, summary_input_tokens, summary_output_tokens, ` +
	`metadata, status, COALESCE(error_message, ''), created_at, updated_at, ` +
	`source, external_id, COALESCE(source_url, ''), COALESCE(source_feed_id, ''), ` +
	`COALESCE(thumbnail_url, ''), published_at`

// videoColumnsNoTranscript is videoColumns with the transcript replaced by an
// empty string, for list queries where the full text is dead weight.
const videoColumnsNoTranscript = `id, user_id, karakeep_bookmark_id, COALESCE(youtube_id, ''), ` +
	`title, channel, duration_seconds, language, '', summary, detail_level, ` +
	`summary_provider, summary_model, summary_input_tokens, summary_output_tokens, ` +
	`metadata, status, COALESCE(error_message, ''), created_at, updated_at, ` +
	`source, external_id, COALESCE(source_url, ''), COALESCE(source_feed_id, ''), ` +
	`COALESCE(thumbnail_url, ''), published_at`

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanVideo(row rowScanner) (*Video, error) {
	var v Video
	err := row.Scan(&v.ID, &v.UserID, &v.KarakeepBookmarkID, &v.YouTubeID,
		&v.Title, &v.Channel, &v.DurationSeconds, &v.Language, &v.Transcript,
		&v.Summary, &v.DetailLevel, &v.SummaryProvider,
		&v.SummaryModel, &v.SummaryInputTokens, &v.SummaryOutputTokens, &v.Metadata,
		&v.Status, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt,
		&v.Source, &v.ExternalID, &v.SourceURL, &v.SourceFeedID,
		&v.ThumbnailURL, &v.PublishedAt)
	if err != nil {
		// Every single-row lookup in this file goes through here, so this is the
		// one place the driver's sentinel becomes ErrNotFound. In scanVideos the
		// branch is unreachable — rows.Next() gates the call.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

func scanVideos(rows pgx.Rows) ([]Video, error) {
	defer rows.Close()
	var videos []Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("scan video: %w", err)
		}
		videos = append(videos, *v)
	}
	return videos, rows.Err()
}

// normalize fills in the defaults a row needs before it is written.
func (v *Video) normalize() {
	if v.Source == "" {
		v.Source = SourceYouTube
	}
	if v.ExternalID == "" {
		v.ExternalID = v.YouTubeID
	}
	if v.Metadata == nil {
		v.Metadata = json.RawMessage(`{}`)
	}
}

// youtubeIDArg returns NULL for an empty YouTube ID. Writing the empty string
// instead would make two podcast rows collide on UNIQUE(user_id, youtube_id).
func (v *Video) youtubeIDArg() any {
	if v.YouTubeID == "" {
		return nil
	}
	return v.YouTubeID
}

const insertVideoSQL = `
	INSERT INTO videos (id, user_id, karakeep_bookmark_id, youtube_id, title, channel,
		duration_seconds, language, transcript, summary, detail_level, summary_provider,
		summary_model, summary_input_tokens, summary_output_tokens,
		embedding, metadata, status,
		source, external_id, source_url, source_feed_id, thumbnail_url, published_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
		$19, $20, NULLIF($21, ''), NULLIF($22, ''), NULLIF($23, ''), $24)`

func (v *Video) insertArgs() []any {
	var embeddingArg any
	return []any{v.ID, v.UserID, v.KarakeepBookmarkID, v.youtubeIDArg(), v.Title, v.Channel,
		v.DurationSeconds, v.Language, v.Transcript, v.Summary, v.DetailLevel, v.SummaryProvider,
		v.SummaryModel, v.SummaryInputTokens, v.SummaryOutputTokens,
		embeddingArg, v.Metadata, v.Status,
		v.Source, v.ExternalID, v.SourceURL, v.SourceFeedID, v.ThumbnailURL, v.PublishedAt}
}

func (db *DB) InsertVideo(ctx context.Context, v *Video) error {
	v.normalize()
	if _, err := db.Pool.Exec(ctx, insertVideoSQL, v.insertArgs()...); err != nil {
		return fmt.Errorf("insert video: %w", err)
	}
	return nil
}

// EnsureVideoRow inserts the row if the (user, source, external ID) triple is
// new and returns whatever row exists afterwards. Concurrent pollers and a
// manual submission for the same episode therefore converge on one row.
func (db *DB) EnsureVideoRow(ctx context.Context, v *Video) (*Video, error) {
	v.normalize()
	_, err := db.Pool.Exec(ctx,
		insertVideoSQL+` ON CONFLICT (user_id, source, external_id) DO NOTHING`,
		v.insertArgs()...)
	if err != nil {
		return nil, fmt.Errorf("ensure video row: %w", err)
	}
	return db.GetBySourceID(ctx, v.UserID, v.Source, v.ExternalID)
}

func (db *DB) GetByYouTubeID(ctx context.Context, userID int, youtubeID string) (*Video, error) {
	return scanVideo(db.Pool.QueryRow(ctx,
		`SELECT `+videoColumns+` FROM videos WHERE user_id = $1 AND youtube_id = $2`,
		userID, youtubeID))
}

// GetBySourceID looks a row up by its source-native identifier.
func (db *DB) GetBySourceID(ctx context.Context, userID int, source, externalID string) (*Video, error) {
	return scanVideo(db.Pool.QueryRow(ctx,
		`SELECT `+videoColumns+`
		FROM videos WHERE user_id = $1 AND source = $2 AND external_id = $3`,
		userID, source, externalID))
}

func (db *DB) UpdateBookmarkID(ctx context.Context, id uuid.UUID, bookmarkID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE videos SET karakeep_bookmark_id = $1, updated_at = NOW() WHERE id = $2`,
		bookmarkID, id)
	return err
}

func (db *DB) GetVideo(ctx context.Context, userID int, id uuid.UUID) (*Video, error) {
	return scanVideo(db.Pool.QueryRow(ctx,
		`SELECT `+videoColumns+` FROM videos WHERE id = $1 AND user_id = $2`,
		id, userID))
}

func (db *DB) UpdateVideoSummary(ctx context.Context, id uuid.UUID, summary string, detailLevel string, provider string, model string, inputTokens, outputTokens int, embedding []float32, metadata json.RawMessage) error {
	var embeddingArg any
	if embedding != nil {
		embeddingArg = pgvector.NewVector(embedding)
	}
	tag, err := db.Pool.Exec(ctx, `
		UPDATE videos SET summary = $1, detail_level = $2, summary_provider = $3,
			summary_model = $4, summary_input_tokens = $5, summary_output_tokens = $6,
			embedding = $7, metadata = $8, status = 'completed', error_message = NULL, updated_at = NOW()
		WHERE id = $9
	`, summary, detailLevel, provider, model, inputTokens, outputTokens, embeddingArg, metadata, id)
	if err != nil {
		return fmt.Errorf("update video summary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) UpdateVideoMetadata(ctx context.Context, id uuid.UUID, title, channel, language string, durationSeconds int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE videos SET title = $1, channel = $2, language = $3,
			duration_seconds = $4, updated_at = NOW()
		WHERE id = $5
	`, title, channel, language, durationSeconds, id)
	if err != nil {
		return fmt.Errorf("update video metadata: %w", err)
	}
	return nil
}

func (db *DB) UpdateVideoTranscript(ctx context.Context, id uuid.UUID, transcript, title, channel, language string, durationSeconds int, segments json.RawMessage) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE videos SET transcript = $1, title = $2, channel = $3, language = $4,
			duration_seconds = $5, transcript_segments = $6, updated_at = NOW()
		WHERE id = $7
	`, transcript, title, channel, language, durationSeconds, segmentsArg(segments), id)
	if err != nil {
		return fmt.Errorf("update video transcript: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// segmentsArg maps a nil segment payload to SQL NULL, keeping the column's
// tri-state: NULL = never fetched, '[]' = fetched and empty (negative cache).
func segmentsArg(segments json.RawMessage) any {
	if segments == nil {
		return nil
	}
	return segments
}

// GetVideoSegments returns the raw transcript_segments payload, nil when the
// column is NULL. The column travels with no other read — videoColumns
// deliberately excludes it, the way videoColumnsNoTranscript blanks the
// transcript for list queries.
func (db *DB) GetVideoSegments(ctx context.Context, userID int, id uuid.UUID) (json.RawMessage, error) {
	var segments []byte
	err := db.Pool.QueryRow(ctx,
		`SELECT transcript_segments FROM videos WHERE id = $1 AND user_id = $2`,
		id, userID).Scan(&segments)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get video segments: %w", err)
	}
	return segments, nil
}

func (db *DB) UpdateVideoSegments(ctx context.Context, id uuid.UUID, segments json.RawMessage) error {
	tag, err := db.Pool.Exec(ctx,
		`UPDATE videos SET transcript_segments = $1, updated_at = NOW() WHERE id = $2`,
		segmentsArg(segments), id)
	if err != nil {
		return fmt.Errorf("update video segments: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
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

// The three batch queries below feed the YouTube pipeline, which addresses rows
// by their YouTube ID. Without the source filter the first failed podcast row
// would produce jobs with an empty YouTube ID.

func (db *DB) ListFailedVideos(ctx context.Context, userID int) ([]Video, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+videoColumnsNoTranscript+`
		FROM videos WHERE user_id = $1 AND status = 'failed' AND source = 'youtube'`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list failed videos: %w", err)
	}
	return scanVideos(rows)
}

func (db *DB) ListVideosWithoutMetadata(ctx context.Context, userID int) ([]Video, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+videoColumnsNoTranscript+`
		FROM videos WHERE user_id = $1 AND title = '' AND source = 'youtube'`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list videos without metadata: %w", err)
	}
	return scanVideos(rows)
}

func (db *DB) ListNoCaptionsVideos(ctx context.Context, userID int) ([]Video, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+videoColumnsNoTranscript+`
		FROM videos WHERE user_id = $1 AND status = 'no_captions' AND source = 'youtube'`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list no_captions videos: %w", err)
	}
	return scanVideos(rows)
}

// ListStalePodcasts returns podcast rows that have been pending or processing
// for longer than the given number of minutes, across all users. A restart in
// the middle of processing, a queue overflow and a crashed job all look the
// same from here, and all three are fixed by re-queuing.
func (db *DB) ListStalePodcasts(ctx context.Context, olderThanMinutes int, limit int) ([]Video, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+videoColumnsNoTranscript+`
		FROM videos
		WHERE source = 'podcast'
		  AND status IN ('pending', 'processing')
		  AND updated_at < NOW() - make_interval(mins => $1)
		ORDER BY updated_at ASC
		LIMIT $2`,
		olderThanMinutes, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale podcasts: %w", err)
	}
	return scanVideos(rows)
}

func (db *DB) DeleteVideo(ctx context.Context, userID int, id uuid.UUID) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM videos WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete video: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) DeleteByBookmarkID(ctx context.Context, userID int, bookmarkID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM videos WHERE karakeep_bookmark_id = $1 AND user_id = $2`, bookmarkID, userID)
	return err
}

func scanMatches(rows pgx.Rows, what string) ([]VideoMatch, error) {
	defer rows.Close()
	var matches []VideoMatch
	for rows.Next() {
		var m VideoMatch
		if err := rows.Scan(&m.ID, &m.YouTubeID, &m.Source, &m.SourceURL, &m.Title,
			&m.Channel, &m.Summary, &m.Metadata, &m.Similarity, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan %s: %w", what, err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// SearchVideos runs the semantic match. An empty source searches both kinds.
func (db *DB) SearchVideos(ctx context.Context, userID int, embedding []float32, threshold float64, limit int, source string) ([]VideoMatch, error) {
	var sourceArg any
	if source != "" {
		sourceArg = source
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, youtube_id, source, source_url, title, channel, summary, metadata, similarity, created_at
		FROM match_videos($1, $2, $3, $4, $5)
	`, pgvector.NewVector(embedding), userID, threshold, limit, sourceArg)
	if err != nil {
		return nil, fmt.Errorf("search videos: %w", err)
	}
	return scanMatches(rows, "video match")
}

// TextSearchVideos runs the keyword match. An empty source searches both kinds.
func (db *DB) TextSearchVideos(ctx context.Context, userID int, query string, limit int, source string) ([]VideoMatch, error) {
	var sourceArg any
	if source != "" {
		sourceArg = source
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, COALESCE(youtube_id, ''), source, COALESCE(source_url, ''), title, channel,
		       summary, metadata, 0.0::float8 AS similarity, created_at
		FROM videos
		WHERE user_id = $1
		  AND status = 'completed'
		  AND ($4::text IS NULL OR source = $4)
		  AND (
		    title ILIKE '%' || $2 || '%'
		    OR channel ILIKE '%' || $2 || '%'
		    OR summary ILIKE '%' || $2 || '%'
		    OR transcript ILIKE '%' || $2 || '%'
		    OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(metadata->'topics') t WHERE t ILIKE '%' || $2 || '%')
		  )
		ORDER BY created_at DESC
		LIMIT $3
	`, userID, query, limit, sourceArg)
	if err != nil {
		return nil, fmt.Errorf("text search videos: %w", err)
	}
	return scanMatches(rows, "text match")
}

type ListFilters struct {
	Channel string
	// ChannelExact matches the stored channel value verbatim. The facet UI
	// sends it, because its values come from the column itself and an ILIKE
	// partial match would conflate "Go" with "Google".
	ChannelExact string
	Language     string
	Topic        string
	Status       string
	Source       string
}

func (db *DB) ListRecent(ctx context.Context, userID int, filters ListFilters, limit, offset int) ([]Video, int, error) {
	statusFilter := "status IN ('completed', 'failed', 'processing', 'pending', 'no_captions')"
	if filters.Status != "" {
		statusFilter = fmt.Sprintf("status = '%s'", filters.Status)
	}
	query := fmt.Sprintf(`SELECT `+videoColumnsNoTranscript+`
		FROM videos WHERE user_id = $1 AND %s`, statusFilter)
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM videos WHERE user_id = $1 AND %s`, statusFilter)
	args := []any{userID}
	argN := 2

	addFilter := func(clauseFmt string, value any) {
		clause := fmt.Sprintf(clauseFmt, argN)
		query += clause
		countQuery += clause
		args = append(args, value)
		argN++
	}

	if filters.Source != "" {
		addFilter(` AND source = $%d`, filters.Source)
	}
	if filters.Channel != "" {
		addFilter(` AND channel ILIKE '%%' || $%d || '%%'`, filters.Channel)
	}
	if filters.ChannelExact != "" {
		addFilter(` AND channel = $%d`, filters.ChannelExact)
	}
	if filters.Language != "" {
		addFilter(` AND language = $%d`, filters.Language)
	}
	if filters.Topic != "" {
		addFilter(` AND metadata->'topics' ? $%d`, filters.Topic)
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
	videos, err := scanVideos(rows)
	if err != nil {
		return nil, 0, err
	}
	return videos, total, nil
}

// ChannelFacet is one channel of the library with its count and, when the
// channel is followed (or is a subscribed podcast feed), its artwork. The
// artwork joins in by title: the subscription's title and the video rows'
// channel column both carry YouTube's author name, so they match for rows
// that arrived through any path.
type ChannelFacet struct {
	Channel      string `json:"channel"`
	Count        int    `json:"count"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

// VideoFacets are the navigable dimensions of the library: the channels and
// the LLM topics of completed rows, with counts. They feed the filter controls
// above the list — the values come from the columns themselves, which is why
// the list query matches them with ChannelExact rather than ILIKE.
type VideoFacets struct {
	Channels []ChannelFacet `json:"channels"`
	Topics   []TopicCount   `json:"topics"`
}

// ListVideoFacets aggregates over one source, or over both when it is empty.
// Only completed rows count: a facet is a way into readable summaries, and a
// pending row has none.
func (db *DB) ListVideoFacets(ctx context.Context, userID int, source string) (*VideoFacets, error) {
	var sourceArg any
	if source != "" {
		sourceArg = source
	}
	const srcClause = ` AND ($2::text IS NULL OR source = $2)`

	facets := &VideoFacets{Channels: []ChannelFacet{}, Topics: []TopicCount{}}

	// Artwork preference: the followed channel's avatar, the podcast feed's
	// cover, then the newest video's thumbnail — a video frame is not an
	// avatar, but it beats an initial for the channels that only exist here
	// through Karakeep, where nothing but the channel name is known.
	rows, err := db.Pool.Query(ctx, `
		SELECT v.channel, COUNT(*),
			COALESCE(
				MAX(cs.thumbnail_url),
				MAX(ps.image_url),
				(array_agg(v.thumbnail_url ORDER BY v.created_at DESC)
					FILTER (WHERE v.thumbnail_url IS NOT NULL AND v.thumbnail_url <> ''))[1],
				'')
		FROM videos v
		LEFT JOIN channel_subscriptions cs ON cs.user_id = v.user_id AND cs.title = v.channel
		LEFT JOIN podcast_subscriptions ps ON ps.user_id = v.user_id AND ps.feed_title = v.channel
		WHERE v.user_id = $1 AND v.status = 'completed' AND v.channel <> ''`+
		strings.ReplaceAll(srcClause, "source", "v.source")+`
		GROUP BY v.channel ORDER BY COUNT(*) DESC, v.channel`, userID, sourceArg)
	if err != nil {
		return nil, fmt.Errorf("list channel facets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c ChannelFacet
		if err := rows.Scan(&c.Channel, &c.Count, &c.ThumbnailURL); err != nil {
			return nil, fmt.Errorf("scan channel facet: %w", err)
		}
		facets.Channels = append(facets.Channels, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	topicRows, err := db.Pool.Query(ctx, `
		SELECT topic, COUNT(*) FROM videos,
			jsonb_array_elements_text(metadata->'topics') AS topic
		WHERE user_id = $1 AND status = 'completed'`+srcClause+`
		GROUP BY topic ORDER BY COUNT(*) DESC, topic`, userID, sourceArg)
	if err != nil {
		return nil, fmt.Errorf("list topic facets: %w", err)
	}
	defer topicRows.Close()
	for topicRows.Next() {
		var t TopicCount
		if err := topicRows.Scan(&t.Topic, &t.Count); err != nil {
			return nil, fmt.Errorf("scan topic facet: %w", err)
		}
		facets.Topics = append(facets.Topics, t)
	}
	return facets, topicRows.Err()
}

// VideoTopics is one row's id with its topic tags, for bulk topic rewrites.
type VideoTopics struct {
	ID     uuid.UUID
	Topics []string
}

// ListTopicCounts returns every topic tag of one user with its usage count,
// most used first, across all sources and statuses — topic maintenance covers
// the whole library, unlike the facets, which only navigate completed rows.
func (db *DB) ListTopicCounts(ctx context.Context, userID int) ([]TopicCount, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT topic, COUNT(*) FROM videos,
			jsonb_array_elements_text(metadata->'topics') AS topic
		WHERE user_id = $1
		GROUP BY topic ORDER BY COUNT(*) DESC, topic`, userID)
	if err != nil {
		return nil, fmt.Errorf("list topic counts: %w", err)
	}
	defer rows.Close()

	var counts []TopicCount
	for rows.Next() {
		var t TopicCount
		if err := rows.Scan(&t.Topic, &t.Count); err != nil {
			return nil, fmt.Errorf("scan topic count: %w", err)
		}
		counts = append(counts, t)
	}
	return counts, rows.Err()
}

// ListVideoTopics returns every row that carries topic tags.
func (db *DB) ListVideoTopics(ctx context.Context, userID int) ([]VideoTopics, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, metadata->'topics' FROM videos
		WHERE user_id = $1 AND jsonb_array_length(COALESCE(metadata->'topics', '[]'::jsonb)) > 0`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list video topics: %w", err)
	}
	defer rows.Close()

	var result []VideoTopics
	for rows.Next() {
		var vt VideoTopics
		var raw []byte
		if err := rows.Scan(&vt.ID, &raw); err != nil {
			return nil, fmt.Errorf("scan video topics: %w", err)
		}
		if err := json.Unmarshal(raw, &vt.Topics); err != nil {
			return nil, fmt.Errorf("decode topics for %s: %w", vt.ID, err)
		}
		result = append(result, vt)
	}
	return result, rows.Err()
}

// UpdateVideoTopics rewrites one row's topic tags inside metadata, leaving
// key_points and action_items untouched.
func (db *DB) UpdateVideoTopics(ctx context.Context, id uuid.UUID, topics []string) error {
	payload, err := json.Marshal(topics)
	if err != nil {
		return fmt.Errorf("marshal topics: %w", err)
	}
	tag, err := db.Pool.Exec(ctx, `
		UPDATE videos
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{topics}', $1::jsonb),
			updated_at = NOW()
		WHERE id = $2`, payload, id)
	if err != nil {
		return fmt.Errorf("update video topics: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetStats aggregates over one source, or over both when source is empty.
func (db *DB) GetStats(ctx context.Context, userID int, source string) (*VideoStats, error) {
	stats := &VideoStats{
		ByStatus: make(map[string]int),
		BySource: make(map[string]int),
	}

	var sourceArg any
	if source != "" {
		sourceArg = source
	}
	// $2 is the optional source filter, repeated in every query below.
	const srcClause = ` AND ($2::text IS NULL OR source = $2)`

	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos WHERE user_id = $1`+srcClause,
		userID, sourceArg).Scan(&stats.TotalCount); err != nil {
		return nil, fmt.Errorf("count videos: %w", err)
	}

	if err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_seconds), 0)
		FROM videos
		WHERE user_id = $1 AND status = 'completed' AND duration_seconds IS NOT NULL`+srcClause,
		userID, sourceArg).Scan(&stats.TotalDurationSeconds); err != nil {
		return nil, fmt.Errorf("sum durations: %w", err)
	}

	// By status
	rows, err := db.Pool.Query(ctx, `
		SELECT status, COUNT(*) FROM videos WHERE user_id = $1`+srcClause+`
		GROUP BY status ORDER BY COUNT(*) DESC
	`, userID, sourceArg)
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

	// By source — always unfiltered, so the UI can offer the other tab's count.
	rows, err = db.Pool.Query(ctx, `
		SELECT source, COUNT(*) FROM videos WHERE user_id = $1 GROUP BY source
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("source counts: %w", err)
	}
	for rows.Next() {
		var src string
		var count int
		if err := rows.Scan(&src, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.BySource[src] = count
	}
	rows.Close()

	// By channel
	rows, err = db.Pool.Query(ctx, `
		SELECT COALESCE(channel, 'unknown'), COUNT(*) as cnt
		FROM videos WHERE user_id = $1 AND status = 'completed'`+srcClause+`
		GROUP BY channel ORDER BY cnt DESC LIMIT 10
	`, userID, sourceArg)
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
		WHERE user_id = $1 AND status = 'completed'`+srcClause+`
		GROUP BY topic ORDER BY cnt DESC LIMIT 10
	`, userID, sourceArg)
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
		WHERE user_id = $1 AND created_at >= NOW() - INTERVAL '30 days'`+srcClause+`
		GROUP BY day ORDER BY day
	`, userID, sourceArg)
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

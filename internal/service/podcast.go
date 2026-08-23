package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/cast2md"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
)

// PodcastSource is the slice of the cast2md client the service uses. Naming it
// keeps the service free of the concrete client and lets the poller be driven
// by a fake in tests.
//
// Pass a nil interface value, not a nil *cast2md.Client, when podcasts are off:
// a typed nil pointer in an interface is not nil, and every podcast entry point
// tests this field against nil to decide whether the feature is available.
type PodcastSource interface {
	BaseURL() string
	EpisodeURL(episodeID int) string
	ListFeeds(ctx context.Context) ([]cast2md.Feed, error)
	GetFeed(ctx context.Context, feedID int) (*cast2md.Feed, error)
	GetEpisode(ctx context.Context, episodeID int) (*cast2md.Episode, error)
	ListCompleted(ctx context.Context, opts cast2md.ListCompletedOptions) ([]cast2md.Episode, error)
	GetTranscript(ctx context.Context, episodeID int) (string, error)
	ProcessFeed(ctx context.Context, feedID int) (*cast2md.BatchResult, error)
}

// errEpisodeNotReady means cast2md has the episode but has not finished
// transcribing it. No row is written in that case, so a later poll or a second
// click can still pick it up.
var errEpisodeNotReady = errors.New("episode transcript is not ready in cast2md")

// ErrPodcastDisabled is returned when a podcast operation is requested but no
// cast2md client is configured.
var ErrPodcastDisabled = errors.New("cast2md integration is not enabled")

// PodcastEnabled reports whether a cast2md client is configured.
func (s *Service) PodcastEnabled() bool {
	return s.cast2md != nil
}

// Cast2MDBaseURL returns the configured cast2md base URL, or an empty string.
func (s *Service) Cast2MDBaseURL() string {
	if s.cast2md == nil {
		return ""
	}
	return s.cast2md.BaseURL()
}

// IsEpisodeNotReady reports whether err means the episode is still being
// transcribed in cast2md.
func IsEpisodeNotReady(err error) bool {
	return errors.Is(err, errEpisodeNotReady)
}

// ProcessEpisodeAsync enqueues an episode on the podcast worker.
func (s *Service) ProcessEpisodeAsync(userID, episodeID int, level string) {
	s.enqueueEpisode(episodeJob{userID: userID, episodeID: episodeID, level: level})
}

func (s *Service) enqueueEpisode(job episodeJob) {
	select {
	case s.episodeQueue <- job:
	default:
		s.log.Warn("episode queue full, dropping job", "episode_id", job.episodeID)
	}
}

// EnsureEpisodeRow creates the podcast row for an episode without processing it
// and returns the row. The poller and the manual submit path both call this
// before enqueuing, so a crash between the two leaves a row the requeue pass
// finds rather than a job that never existed.
func (s *Service) EnsureEpisodeRow(ctx context.Context, userID, episodeID int, level string) (*storage.Video, error) {
	if s.cast2md == nil {
		return nil, ErrPodcastDisabled
	}

	ep, err := s.cast2md.GetEpisode(ctx, episodeID)
	if err != nil {
		return nil, fmt.Errorf("get episode %d: %w", episodeID, err)
	}
	if ep.Status != cast2md.StatusCompleted {
		return nil, fmt.Errorf("%w (status: %s)", errEpisodeNotReady, ep.Status)
	}

	feedTitle := ""
	imageURL := ""
	if feed, err := s.cast2md.GetFeed(ctx, ep.FeedID); err == nil {
		feedTitle = feed.Name()
		imageURL = feed.ImageURL
	} else {
		s.log.Warn("feed lookup failed, storing episode without feed title",
			"episode_id", episodeID, "feed_id", ep.FeedID, "error", err)
	}

	if level == "" {
		level = s.summaryCfg.DefaultLevel
	}

	row := &storage.Video{
		ID:           uuid.New(),
		UserID:       userID,
		Source:       storage.SourcePodcast,
		ExternalID:   strconv.Itoa(ep.ID),
		SourceURL:    s.cast2md.EpisodeURL(ep.ID),
		SourceFeedID: strconv.Itoa(ep.FeedID),
		ThumbnailURL: imageURL,
		PublishedAt:  parsePublished(ep.PublishedAt),
		Title:        ep.Title,
		// Channel carries the feed title, which is what makes the existing
		// channel filter, stats and text search work for podcasts unchanged.
		Channel:         feedTitle,
		DurationSeconds: ep.DurationSeconds,
		DetailLevel:     level,
		Status:          "pending",
	}

	return s.db.EnsureVideoRow(ctx, row)
}

// parsePublished converts cast2md's ISO timestamp, tolerating both the naive
// and the offset-bearing form. An unparseable value yields nil rather than an
// error — the field is decorative.
func parsePublished(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// ProcessEpisode fetches the transcript for a cast2md episode and summarizes it.
// There is no Karakeep writeback — podcasts do not come from bookmarks.
func (s *Service) ProcessEpisode(ctx context.Context, userID, episodeID int, level string) error {
	if s.cast2md == nil {
		return ErrPodcastDisabled
	}

	externalID := strconv.Itoa(episodeID)
	existing, err := s.db.GetBySourceID(ctx, userID, storage.SourcePodcast, externalID)
	if err == nil && existing.Status == "completed" {
		s.log.Info("episode already summarized", "episode_id", episodeID)
		return nil
	}

	video := existing
	if video == nil {
		video, err = s.EnsureEpisodeRow(ctx, userID, episodeID, level)
		if err != nil {
			return err
		}
	}
	if level == "" {
		level = video.DetailLevel
	}

	if err := s.db.UpdateVideoStatus(ctx, video.ID, "processing", ""); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	transcript, err := s.cast2md.GetTranscript(ctx, episodeID)
	if err != nil {
		errMsg := fmt.Sprintf("transcript fetch failed: %v", err)
		_ = s.db.UpdateVideoStatus(ctx, video.ID, "failed", errMsg)
		return fmt.Errorf("transcript fetch failed: %w", err)
	}

	if err := s.db.UpdateVideoTranscript(ctx, video.ID, transcript,
		video.Title, video.Channel, video.Language, video.DurationSeconds, nil); err != nil {
		return fmt.Errorf("update transcript: %w", err)
	}

	// The prompt is given "Feed — Episode" as the title so the summary can name
	// the show without a separate placeholder.
	title := video.Title
	if video.Channel != "" {
		title = video.Channel + " — " + video.Title
	}

	if _, err := s.summarizeAndStore(ctx, summarizeRequest{
		userID:     userID,
		video:      video,
		transcript: transcript,
		title:      title,
		// cast2md reports no language for an episode, so the summary follows
		// the transcript's own.
		language: summary.LangSameAsTranscript,
		level:    level,
	}); err != nil {
		errMsg := fmt.Sprintf("summary generation failed: %v", err)
		_ = s.db.UpdateVideoStatus(ctx, video.ID, "failed", errMsg)
		return err
	}

	s.log.Info("episode summarized", "episode_id", episodeID, "title", video.Title, "level", level)
	return nil
}

// How many of a feed's newest episodes a fresh subscription summarizes.
// The database carries the same default and the same ceiling.
const (
	DefaultInitialBackfill = 3
	MaxInitialBackfill     = 100
)

// PodcastFeed is one cast2md feed joined with this user's subscription state.
type PodcastFeed struct {
	FeedID       string `json:"feed_id"`
	Title        string `json:"title"`
	ImageURL     string `json:"image_url,omitempty"`
	EpisodeCount int    `json:"episode_count"`
	// CompletedCount is how many episodes cast2md has a transcript for, and so
	// how many "Summarize all" would take on.
	CompletedCount int `json:"completed_count"`
	// TranscribableCount is how many episodes cast2md could still turn into a
	// transcript, and so how many "Transcribe all" would queue.
	TranscribableCount int        `json:"transcribable_count"`
	Subscribed         bool       `json:"subscribed"`
	DetailLevel        string     `json:"detail_level"`
	InitialBackfill    int        `json:"initial_backfill"`
	Initialized        bool       `json:"initialized"`
	SummarizedCount    int        `json:"summarized_count"`
	LastPolledAt       *time.Time `json:"last_polled_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
}

// ListPodcastFeeds returns every cast2md feed with this user's subscription
// state attached.
func (s *Service) ListPodcastFeeds(ctx context.Context, userID int) ([]PodcastFeed, error) {
	if s.cast2md == nil {
		return nil, ErrPodcastDisabled
	}

	feeds, err := s.cast2md.ListFeeds(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cast2md feeds: %w", err)
	}
	subs, err := s.db.ListSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	counts, err := s.db.CountSummarizedByFeed(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count summarized episodes: %w", err)
	}

	byFeedID := make(map[string]storage.PodcastSubscription, len(subs))
	for _, sub := range subs {
		byFeedID[sub.FeedID] = sub
	}

	out := make([]PodcastFeed, 0, len(feeds))
	for _, f := range feeds {
		feedID := strconv.Itoa(f.ID)
		pf := PodcastFeed{
			FeedID:             feedID,
			Title:              f.Name(),
			ImageURL:           f.ImageURL,
			EpisodeCount:       f.EpisodeCount,
			CompletedCount:     f.Completed(),
			TranscribableCount: f.Transcribable(),
			DetailLevel:        s.summaryCfg.DefaultLevel,
			InitialBackfill:    DefaultInitialBackfill,
			SummarizedCount:    counts[feedID],
		}
		if sub, ok := byFeedID[feedID]; ok {
			pf.Subscribed = sub.Enabled
			pf.DetailLevel = sub.DetailLevel
			pf.InitialBackfill = sub.InitialBackfill
			pf.Initialized = sub.Initialized
			pf.LastPolledAt = sub.LastPolledAt
			pf.LastError = sub.LastError
		}
		out = append(out, pf)
	}
	return out, nil
}

// GetPodcastSubscription returns one subscription, or an error when the user
// has none for that feed.
func (s *Service) GetPodcastSubscription(ctx context.Context, userID int, feedID string) (*storage.PodcastSubscription, error) {
	return s.db.GetSubscription(ctx, userID, feedID)
}

// initialPollTimeout bounds the first poll that runs when a feed is switched
// on. The default backfill of three episodes takes about a second; the cap is
// there for a large backfill or a slow cast2md, and exceeding it is not an
// error — the ticker picks the subscription up afterwards.
const initialPollTimeout = 60 * time.Second

// SetPodcastSubscription switches a feed on or off and sets its detail level.
//
// Switching a feed on runs its first poll immediately, so the backfill appears
// within seconds instead of at the next tick up to a poll interval away. The
// poll is best effort: the subscription is saved first, and a failure leaves
// `initialized` false so the poller retries on its own schedule.
//
// Switching a feed off keeps the watermark, so switching it back on later
// fetches the gap rather than starting over.
func (s *Service) SetPodcastSubscription(ctx context.Context, userID int, feedID string, enabled bool, detailLevel string, initialBackfill int) (*storage.PodcastSubscription, error) {
	if s.cast2md == nil {
		return nil, ErrPodcastDisabled
	}
	if detailLevel == "" {
		detailLevel = s.summaryCfg.DefaultLevel
	}
	if detailLevel != "medium" && detailLevel != "deep" {
		return nil, fmt.Errorf("invalid detail level: %q (must be medium or deep)", detailLevel)
	}
	if initialBackfill < 0 || initialBackfill > MaxInitialBackfill {
		return nil, fmt.Errorf("invalid initial backfill: %d (must be between 0 and %d)", initialBackfill, MaxInitialBackfill)
	}

	numericFeedID, err := strconv.Atoi(feedID)
	if err != nil {
		return nil, fmt.Errorf("invalid feed ID %q", feedID)
	}
	feed, err := s.cast2md.GetFeed(ctx, numericFeedID)
	if err != nil {
		return nil, err
	}

	sub, err := s.db.UpsertSubscription(ctx, userID, feedID, feed.Name(), feed.ImageURL, enabled, detailLevel, initialBackfill)
	if err != nil {
		return nil, err
	}
	if !sub.Enabled || sub.Initialized {
		return sub, nil
	}

	pollCtx, cancel := context.WithTimeout(ctx, initialPollTimeout)
	defer cancel()
	if err := s.pollSubscription(pollCtx, *sub); err != nil {
		// Not returned to the caller: enabling the feed succeeded, and the
		// poller retries what this attempt did not manage.
		s.log.Warn("initial poll after subscribe failed, leaving it to the poller",
			"feed_id", feedID, "user_id", userID, "error", err)
		if dbErr := s.db.SetSubscriptionError(ctx, sub.ID, err.Error()); dbErr != nil {
			s.log.Error("failed to record initial poll error", "feed_id", feedID, "error", dbErr)
		}
	}

	// Re-read so the response carries the watermark and the initialized flag
	// the poll just wrote.
	return s.db.GetSubscription(ctx, userID, feedID)
}

// EpisodePreview is what the deep-link landing page shows before summarizing.
type EpisodePreview struct {
	EpisodeID       int    `json:"episode_id"`
	FeedID          string `json:"feed_id"`
	FeedTitle       string `json:"feed_title"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	Status          string `json:"status"`
	ImageURL        string `json:"image_url,omitempty"`
	SourceURL       string `json:"source_url"`
	// ExistingID is set when this episode already has a summary, so the
	// frontend can redirect instead of offering to summarize again.
	ExistingID     string `json:"existing_id,omitempty"`
	ExistingStatus string `json:"existing_status,omitempty"`
}

// GetEpisodePreview returns episode and feed detail plus any existing row.
func (s *Service) GetEpisodePreview(ctx context.Context, userID, episodeID int) (*EpisodePreview, error) {
	if s.cast2md == nil {
		return nil, ErrPodcastDisabled
	}

	ep, err := s.cast2md.GetEpisode(ctx, episodeID)
	if err != nil {
		return nil, err
	}

	preview := &EpisodePreview{
		EpisodeID:       ep.ID,
		FeedID:          strconv.Itoa(ep.FeedID),
		Title:           ep.Title,
		Description:     ep.Description,
		DurationSeconds: ep.DurationSeconds,
		PublishedAt:     ep.PublishedAt,
		Status:          ep.Status,
		SourceURL:       s.cast2md.EpisodeURL(ep.ID),
	}
	if feed, err := s.cast2md.GetFeed(ctx, ep.FeedID); err == nil {
		preview.FeedTitle = feed.Name()
		preview.ImageURL = feed.ImageURL
	}
	if existing, err := s.db.GetBySourceID(ctx, userID, storage.SourcePodcast, strconv.Itoa(ep.ID)); err == nil {
		preview.ExistingID = existing.ID.String()
		preview.ExistingStatus = existing.Status
	}
	return preview, nil
}

// SubmitEpisode creates the row synchronously and enqueues the work, so the
// caller can navigate to the row's page straight away.
func (s *Service) SubmitEpisode(ctx context.Context, userID, episodeID int, level string) (*storage.Video, error) {
	row, err := s.EnsureEpisodeRow(ctx, userID, episodeID, level)
	if err != nil {
		return nil, err
	}
	if row.Status == "completed" {
		return row, nil
	}
	s.ProcessEpisodeAsync(userID, episodeID, level)
	return row, nil
}

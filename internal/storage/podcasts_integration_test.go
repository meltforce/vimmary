package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
)

func newPodcastRow(externalID string) *storage.Video {
	return &storage.Video{
		ID:           uuid.New(),
		UserID:       1,
		Source:       storage.SourcePodcast,
		ExternalID:   externalID,
		SourceURL:    "https://cast2md.test/episodes/" + externalID,
		SourceFeedID: "42",
		ThumbnailURL: "https://img.test/cover.jpg",
		Title:        "Episode " + externalID,
		Channel:      "Test Show",
		DetailLevel:  "medium",
		Status:       "pending",
	}
}

// A podcast row carries no YouTube ID. It has to land as NULL rather than the
// empty string, or the second such row collides on UNIQUE(user_id, youtube_id).
func TestInsertPodcastRow_YouTubeIDIsNull(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	first := newPodcastRow("test-" + uuid.NewString()[:8])
	second := newPodcastRow("test-" + uuid.NewString()[:8])
	for _, v := range []*storage.Video{first, second} {
		if err := store.InsertVideo(ctx, v); err != nil {
			t.Fatalf("insert podcast row %s: %v", v.ExternalID, err)
		}
		t.Cleanup(func() { _ = store.DeleteVideo(ctx, v.UserID, v.ID) })
	}

	got, err := store.GetVideo(ctx, first.UserID, first.ID)
	if err != nil {
		t.Fatalf("get video: %v", err)
	}
	if got.YouTubeID != "" {
		t.Errorf("YouTubeID = %q, want empty (the column is NULL)", got.YouTubeID)
	}
	if got.Source != storage.SourcePodcast {
		t.Errorf("Source = %q", got.Source)
	}
	if got.SourceURL != first.SourceURL {
		t.Errorf("SourceURL = %q, want %q", got.SourceURL, first.SourceURL)
	}
	if got.ThumbnailURL != first.ThumbnailURL {
		t.Errorf("ThumbnailURL = %q", got.ThumbnailURL)
	}

	var nullCount int
	if err := store.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos WHERE id = ANY($1) AND youtube_id IS NULL`,
		[]uuid.UUID{first.ID, second.ID}).Scan(&nullCount); err != nil {
		t.Fatalf("count NULL youtube_id: %v", err)
	}
	if nullCount != 2 {
		t.Errorf("%d of 2 podcast rows have a NULL youtube_id", nullCount)
	}
}

func TestGetBySourceIDAndEnsureVideoRow(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	externalID := "test-" + uuid.NewString()[:8]
	row := newPodcastRow(externalID)

	created, err := store.EnsureVideoRow(ctx, row)
	if err != nil {
		t.Fatalf("EnsureVideoRow: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, created.UserID, created.ID) })

	// A second call with a fresh UUID must return the row that already exists.
	again := newPodcastRow(externalID)
	second, err := store.EnsureVideoRow(ctx, again)
	if err != nil {
		t.Fatalf("second EnsureVideoRow: %v", err)
	}
	if second.ID != created.ID {
		t.Errorf("EnsureVideoRow created a duplicate: %s then %s", created.ID, second.ID)
	}

	fetched, err := store.GetBySourceID(ctx, 1, storage.SourcePodcast, externalID)
	if err != nil {
		t.Fatalf("GetBySourceID: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("GetBySourceID returned %s, want %s", fetched.ID, created.ID)
	}

	// The same external ID under the other source is a different row.
	if _, err := store.GetBySourceID(ctx, 1, storage.SourceYouTube, externalID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("GetBySourceID(youtube) error = %v, want storage.ErrNotFound", err)
	}
}

// The source filter is what keeps the videos page video-only and the podcasts
// page podcast-only.
func TestListRecentSourceFilter(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	podcast := newPodcastRow("test-" + uuid.NewString()[:8])
	podcast.Status = "completed"
	if err := store.InsertVideo(ctx, podcast); err != nil {
		t.Fatalf("insert podcast row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, podcast.UserID, podcast.ID) })

	video := &storage.Video{
		ID:          uuid.New(),
		UserID:      1,
		YouTubeID:   "test_src_" + uuid.NewString()[:8],
		Title:       "A video",
		DetailLevel: "medium",
		Status:      "completed",
	}
	if err := store.InsertVideo(ctx, video); err != nil {
		t.Fatalf("insert video row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, video.UserID, video.ID) })

	contains := func(rows []storage.Video, id uuid.UUID) bool {
		for _, r := range rows {
			if r.ID == id {
				return true
			}
		}
		return false
	}

	yt, _, err := store.ListRecent(ctx, 1, storage.ListFilters{Source: storage.SourceYouTube}, 200, 0)
	if err != nil {
		t.Fatalf("ListRecent(youtube): %v", err)
	}
	if contains(yt, podcast.ID) {
		t.Error("the youtube listing must not contain the podcast row")
	}
	if !contains(yt, video.ID) {
		t.Error("the youtube listing is missing the video row")
	}

	pods, _, err := store.ListRecent(ctx, 1, storage.ListFilters{Source: storage.SourcePodcast}, 200, 0)
	if err != nil {
		t.Fatalf("ListRecent(podcast): %v", err)
	}
	if !contains(pods, podcast.ID) {
		t.Error("the podcast listing is missing the podcast row")
	}
	if contains(pods, video.ID) {
		t.Error("the podcast listing must not contain the video row")
	}

	both, _, err := store.ListRecent(ctx, 1, storage.ListFilters{}, 200, 0)
	if err != nil {
		t.Fatalf("ListRecent(all): %v", err)
	}
	if !contains(both, podcast.ID) || !contains(both, video.ID) {
		t.Error("an empty source filter should return both kinds")
	}
}

// The three batch queries feed the YouTube pipeline, which addresses rows by
// their YouTube ID. A podcast row reaching them produces a job with an empty ID.
func TestBatchQueriesExcludePodcasts(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	failed := newPodcastRow("test-" + uuid.NewString()[:8])
	failed.Status = "failed"
	failed.Title = ""

	noCaptions := newPodcastRow("test-" + uuid.NewString()[:8])
	noCaptions.Status = "no_captions"

	for _, v := range []*storage.Video{failed, noCaptions} {
		if err := store.InsertVideo(ctx, v); err != nil {
			t.Fatalf("insert podcast row: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteVideo(ctx, v.UserID, v.ID) })
	}

	checks := []struct {
		name string
		list func() ([]storage.Video, error)
	}{
		{"ListFailedVideos", func() ([]storage.Video, error) { return store.ListFailedVideos(ctx, 1) }},
		{"ListVideosWithoutMetadata", func() ([]storage.Video, error) { return store.ListVideosWithoutMetadata(ctx, 1) }},
		{"ListNoCaptionsVideos", func() ([]storage.Video, error) { return store.ListNoCaptionsVideos(ctx, 1) }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			rows, err := c.list()
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			for _, r := range rows {
				if r.Source == storage.SourcePodcast {
					t.Fatalf("%s returned podcast row %s", c.name, r.ID)
				}
			}
		})
	}
}

func TestListStalePodcasts(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	stuck := newPodcastRow("test-" + uuid.NewString()[:8])
	stuck.Status = "processing"
	if err := store.InsertVideo(ctx, stuck); err != nil {
		t.Fatalf("insert: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, stuck.UserID, stuck.ID) })

	// A row that has just been touched is not stale yet.
	fresh, err := store.ListStalePodcasts(ctx, 15, 100)
	if err != nil {
		t.Fatalf("ListStalePodcasts: %v", err)
	}
	for _, r := range fresh {
		if r.ID == stuck.ID {
			t.Fatal("a row updated seconds ago must not count as stale")
		}
	}

	// Age it past the threshold.
	if _, err := store.Pool.Exec(ctx,
		`UPDATE videos SET updated_at = NOW() - INTERVAL '30 minutes' WHERE id = $1`, stuck.ID); err != nil {
		t.Fatalf("age row: %v", err)
	}

	stale, err := store.ListStalePodcasts(ctx, 15, 100)
	if err != nil {
		t.Fatalf("ListStalePodcasts: %v", err)
	}
	found := false
	for _, r := range stale {
		if r.ID == stuck.ID {
			found = true
		}
		if r.Source != storage.SourcePodcast {
			t.Errorf("ListStalePodcasts returned a %s row", r.Source)
		}
	}
	if !found {
		t.Error("a podcast row stuck in processing for 30 minutes should be listed as stale")
	}
}

func TestSearchVideosSourceFilter(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	marker := "zqx" + uuid.NewString()[:8]

	podcast := newPodcastRow("test-" + uuid.NewString()[:8])
	podcast.Status = "completed"
	podcast.Summary = "A summary mentioning " + marker
	if err := store.InsertVideo(ctx, podcast); err != nil {
		t.Fatalf("insert podcast row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, podcast.UserID, podcast.ID) })

	video := &storage.Video{
		ID:          uuid.New(),
		UserID:      1,
		YouTubeID:   "test_search_" + uuid.NewString()[:8],
		Title:       "A video about " + marker,
		DetailLevel: "medium",
		Status:      "completed",
	}
	if err := store.InsertVideo(ctx, video); err != nil {
		t.Fatalf("insert video row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, video.UserID, video.ID) })

	all, err := store.TextSearchVideos(ctx, 1, marker, 50, "")
	if err != nil {
		t.Fatalf("TextSearchVideos(all): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered search returned %d rows, want 2", len(all))
	}

	pods, err := store.TextSearchVideos(ctx, 1, marker, 50, storage.SourcePodcast)
	if err != nil {
		t.Fatalf("TextSearchVideos(podcast): %v", err)
	}
	if len(pods) != 1 || pods[0].ID != podcast.ID {
		t.Fatalf("podcast-filtered search returned %+v", pods)
	}
	if pods[0].Source != storage.SourcePodcast {
		t.Errorf("match Source = %q", pods[0].Source)
	}
	if pods[0].SourceURL != podcast.SourceURL {
		t.Errorf("match SourceURL = %q, want %q", pods[0].SourceURL, podcast.SourceURL)
	}
	if pods[0].YouTubeID != "" {
		t.Errorf("match YouTubeID = %q, want empty for a podcast", pods[0].YouTubeID)
	}
}

// match_videos changed its return type in migration 000009. This exercises the
// new signature — including the source filter — against the real function.
func TestMatchVideosWithSourceFilter(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	embedding := make([]float32, 1024)
	for i := range embedding {
		embedding[i] = 0.01
	}

	podcast := newPodcastRow("test-" + uuid.NewString()[:8])
	podcast.Status = "completed"
	podcast.Summary = "Podcast summary"
	if err := store.InsertVideo(ctx, podcast); err != nil {
		t.Fatalf("insert podcast row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, podcast.UserID, podcast.ID) })

	if err := store.UpdateVideoSummary(ctx, podcast.ID, "Podcast summary", "medium",
		"claude", "test-model", 10, 20, embedding, []byte(`{}`)); err != nil {
		t.Fatalf("UpdateVideoSummary: %v", err)
	}

	matches, err := store.SearchVideos(ctx, 1, embedding, 0.5, 50, storage.SourcePodcast)
	if err != nil {
		t.Fatalf("SearchVideos(podcast): %v", err)
	}
	var found *storage.VideoMatch
	for i := range matches {
		if matches[i].ID == podcast.ID {
			found = &matches[i]
		}
		if matches[i].Source != storage.SourcePodcast {
			t.Errorf("source-filtered match has Source = %q", matches[i].Source)
		}
	}
	if found == nil {
		t.Fatal("the podcast row should match its own embedding")
	} else if found.SourceURL != podcast.SourceURL {
		t.Errorf("SourceURL = %q, want %q", found.SourceURL, podcast.SourceURL)
	}

	// The same query restricted to videos must not return it.
	ytMatches, err := store.SearchVideos(ctx, 1, embedding, 0.5, 50, storage.SourceYouTube)
	if err != nil {
		t.Fatalf("SearchVideos(youtube): %v", err)
	}
	for _, m := range ytMatches {
		if m.ID == podcast.ID {
			t.Fatal("the youtube-filtered match set must not contain the podcast row")
		}
	}
}

func TestUserPromptsPerSource(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		_ = store.SetUserPrompt(ctx, 1, storage.SourceYouTube, "medium", "")
		_ = store.SetUserPrompt(ctx, 1, storage.SourcePodcast, "medium", "")
	})

	if err := store.SetUserPrompt(ctx, 1, storage.SourcePodcast, "medium", "podcast prompt"); err != nil {
		t.Fatalf("SetUserPrompt(podcast): %v", err)
	}

	got, err := store.GetUserPrompt(ctx, 1, storage.SourcePodcast, "medium")
	if err != nil {
		t.Fatalf("GetUserPrompt(podcast): %v", err)
	}
	if got != "podcast prompt" {
		t.Errorf("podcast prompt = %q", got)
	}

	// Setting the podcast prompt must leave the video prompt alone.
	ytPrompt, err := store.GetUserPrompt(ctx, 1, storage.SourceYouTube, "medium")
	if err != nil {
		t.Fatalf("GetUserPrompt(youtube): %v", err)
	}
	if ytPrompt != "" {
		t.Errorf("video prompt = %q, want it untouched (empty)", ytPrompt)
	}

	// An empty prompt resets to the default by deleting the row.
	if err := store.SetUserPrompt(ctx, 1, storage.SourcePodcast, "medium", ""); err != nil {
		t.Fatalf("reset SetUserPrompt: %v", err)
	}
	got, err = store.GetUserPrompt(ctx, 1, storage.SourcePodcast, "medium")
	if err != nil {
		t.Fatalf("GetUserPrompt after reset: %v", err)
	}
	if got != "" {
		t.Errorf("prompt after reset = %q, want empty", got)
	}
}

func TestSubscriptionWatermarkRoundTrip(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	feedID := "99" + uuid.NewString()[:4]
	t.Cleanup(func() {
		_, _ = store.Pool.Exec(ctx, `DELETE FROM podcast_subscriptions WHERE feed_id = $1`, feedID)
	})

	sub, err := store.UpsertSubscription(ctx, 1, feedID, "Show", "https://img", true, "deep", 3)
	if err != nil {
		t.Fatalf("UpsertSubscription: %v", err)
	}
	if sub.Initialized {
		t.Error("a new subscription must start uninitialized")
	}
	if sub.DetailLevel != "deep" {
		t.Errorf("DetailLevel = %q", sub.DetailLevel)
	}
	if sub.InitialBackfill != 3 {
		t.Errorf("InitialBackfill = %d, want 3", sub.InitialBackfill)
	}

	// cast2md writes naive local timestamps. They are stored as text and must
	// come back byte-for-byte — a round trip through time parsing is exactly
	// where episodes would go missing.
	const watermark = "2026-08-06T09:15:00.123456"
	if err := store.SetSubscriptionWatermark(ctx, sub.ID, watermark, true); err != nil {
		t.Fatalf("SetSubscriptionWatermark: %v", err)
	}

	got, err := store.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if got.Watermark != watermark {
		t.Errorf("watermark = %q, want %q verbatim", got.Watermark, watermark)
	}
	if !got.Initialized {
		t.Error("Initialized should be true")
	}
	if got.LastError != "" {
		t.Errorf("LastError = %q, want it cleared by a successful poll", got.LastError)
	}

	// Disabling keeps the watermark, so re-enabling fetches the gap.
	if _, err := store.UpsertSubscription(ctx, 1, feedID, "Show", "https://img", false, "deep", 3); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got, err = store.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription after disable: %v", err)
	}
	if got.Enabled {
		t.Error("Enabled should be false")
	}
	if got.Watermark != watermark {
		t.Errorf("watermark = %q after disabling, want it preserved", got.Watermark)
	}
}

// initial_backfill has a default and a ceiling in the database, so a bad value
// cannot reach the poller even if a caller skips the service-layer check.
func TestInitialBackfillDefaultAndBounds(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	feedID := "98" + uuid.NewString()[:4]
	t.Cleanup(func() {
		_, _ = store.Pool.Exec(ctx, `DELETE FROM podcast_subscriptions WHERE feed_id = $1`, feedID)
	})

	// A row inserted without the column takes the schema default.
	if _, err := store.Pool.Exec(ctx,
		`INSERT INTO podcast_subscriptions (user_id, feed_id, feed_title) VALUES (1, $1, 'Show')`,
		feedID); err != nil {
		t.Fatalf("insert: %v", err)
	}
	sub, err := store.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub.InitialBackfill != 3 {
		t.Errorf("default initial_backfill = %d, want 3", sub.InitialBackfill)
	}

	// 0 is valid and means "from now on".
	if _, err := store.UpsertSubscription(ctx, 1, feedID, "Show", "", true, "medium", 0); err != nil {
		t.Fatalf("UpsertSubscription(0): %v", err)
	}
	sub, _ = store.GetSubscription(ctx, 1, feedID)
	if sub.InitialBackfill != 0 {
		t.Errorf("initial_backfill = %d, want 0", sub.InitialBackfill)
	}

	// Out of range is rejected by the CHECK constraint.
	if _, err := store.UpsertSubscription(ctx, 1, feedID, "Show", "", true, "medium", -1); err == nil {
		t.Error("a negative initial_backfill should be rejected")
	}
	if _, err := store.UpsertSubscription(ctx, 1, feedID, "Show", "", true, "medium", 101); err == nil {
		t.Error("an initial_backfill above the ceiling should be rejected")
	}
}

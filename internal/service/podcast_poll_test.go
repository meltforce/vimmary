package service

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"testing"

	mkdb "github.com/meltforce/meltkit/pkg/db"
	vimmary "github.com/meltforce/vimmary"
	"github.com/meltforce/vimmary/internal/cast2md"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/storage"
)

const pollTestDSN = "postgres://vimmary:vimmary@localhost:5434/vimmary?sslmode=disable"

// fakeSource is a PodcastSource driven entirely from the test.
type fakeSource struct {
	feeds    []cast2md.Feed
	episodes map[int]cast2md.Episode
	// listResult is returned by ListCompleted, filtered by the Since option so
	// the watermark semantics are exercised rather than stubbed away.
	listResult []cast2md.Episode
	listErr    error
	lastOpts   cast2md.ListCompletedOptions
	calls      int
}

func (f *fakeSource) BaseURL() string { return "https://cast2md.test" }
func (f *fakeSource) EpisodeURL(id int) string {
	return fmt.Sprintf("https://cast2md.test/episodes/%d", id)
}
func (f *fakeSource) ListFeeds(context.Context) ([]cast2md.Feed, error) { return f.feeds, nil }

func (f *fakeSource) GetFeed(_ context.Context, feedID int) (*cast2md.Feed, error) {
	for i := range f.feeds {
		if f.feeds[i].ID == feedID {
			return &f.feeds[i], nil
		}
	}
	return nil, fmt.Errorf("feed %d not found", feedID)
}

func (f *fakeSource) GetEpisode(_ context.Context, episodeID int) (*cast2md.Episode, error) {
	ep, ok := f.episodes[episodeID]
	if !ok {
		return nil, fmt.Errorf("episode %d not found", episodeID)
	}
	return &ep, nil
}

func (f *fakeSource) ListCompleted(_ context.Context, opts cast2md.ListCompletedOptions) ([]cast2md.Episode, error) {
	f.calls++
	f.lastOpts = opts
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []cast2md.Episode
	for _, ep := range f.listResult {
		if opts.Since != "" && ep.UpdatedAt <= opts.Since {
			continue
		}
		out = append(out, ep)
	}
	if opts.Order == cast2md.OrderUpdatedDesc && len(out) > 0 {
		reversed := make([]cast2md.Episode, len(out))
		for i, ep := range out {
			reversed[len(out)-1-i] = ep
		}
		out = reversed
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func (f *fakeSource) GetTranscript(context.Context, int) (string, error) {
	return "transcript body", nil
}

// newPollTestService wires a Service against the development database and a
// fake cast2md. The summarizer is never reached: these tests stop at the queue.
func newPollTestService(t *testing.T, src PodcastSource) *Service {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Skip("skipping poller test in CI (no database)")
		}
		dsn = pollTestDSN
	}

	migrationsFS, err := fs.Sub(vimmary.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if err := mkdb.RunMigrations(dsn, migrationsFS); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	d, err := mkdb.New(context.Background(), dsn, mkdb.WithPgvector())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	return &Service{
		db:           storage.NewDB(d),
		cast2md:      src,
		cast2mdCfg:   config.Cast2MDConfig{MaxPerPoll: 25},
		summaryCfg:   config.SummaryConfig{DefaultLevel: "medium"},
		log:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		queue:        make(chan processJob, 10),
		episodeQueue: make(chan episodeJob, 200),
	}
}

// uniqueFeedID derives a per-test feed ID so tests do not collide on
// UNIQUE(user_id, feed_id). The ID has to stay numeric because the poller
// parses it back into cast2md's integer feed ID.
func uniqueFeedID(t *testing.T) (string, int) {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	id := 900000 + int(h.Sum32()%90000)
	return strconv.Itoa(id), id
}

func cleanupFeed(t *testing.T, svc *Service, userID int, feedID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = svc.db.Pool.Exec(ctx,
			`DELETE FROM videos WHERE user_id = $1 AND source = 'podcast' AND source_feed_id = $2`,
			userID, feedID)
		_, _ = svc.db.Pool.Exec(ctx,
			`DELETE FROM podcast_subscriptions WHERE user_id = $1 AND feed_id = $2`,
			userID, feedID)
	})
}

func drainEpisodeQueue(s *Service) []episodeJob {
	var jobs []episodeJob
	for {
		select {
		case j := <-s.episodeQueue:
			jobs = append(jobs, j)
		default:
			return jobs
		}
	}
}

// The first poll of a newly enabled feed adopts the newest episode's timestamp
// as the watermark and processes nothing. That is the "from now on" guarantee.
func TestPollSubscription_FirstRunOnlySetsWatermark(t *testing.T) {
	feedID, numericFeedID := uniqueFeedID(t)
	src := &fakeSource{
		feeds: []cast2md.Feed{{ID: numericFeedID, Title: "Test Show"}},
		episodes: map[int]cast2md.Episode{
			1: {ID: 1, FeedID: numericFeedID, Title: "Old", Status: cast2md.StatusCompleted, UpdatedAt: "2026-08-01T10:00:00"},
			2: {ID: 2, FeedID: numericFeedID, Title: "New", Status: cast2md.StatusCompleted, UpdatedAt: "2026-08-02T10:00:00"},
		},
		listResult: []cast2md.Episode{
			{ID: 1, FeedID: numericFeedID, UpdatedAt: "2026-08-01T10:00:00"},
			{ID: 2, FeedID: numericFeedID, UpdatedAt: "2026-08-02T10:00:00"},
		},
	}
	svc := newPollTestService(t, src)
	ctx := context.Background()
	cleanupFeed(t, svc, 1, feedID)

	sub, err := svc.db.UpsertSubscription(ctx, 1, feedID, "Test Show", "", true, "medium")
	if err != nil {
		t.Fatalf("UpsertSubscription: %v", err)
	}
	if sub.Initialized {
		t.Fatal("a newly enabled subscription must start uninitialized")
	}

	if err := svc.pollSubscription(ctx, *sub); err != nil {
		t.Fatalf("pollSubscription: %v", err)
	}

	if jobs := drainEpisodeQueue(svc); len(jobs) != 0 {
		t.Errorf("first run queued %d jobs, want 0", len(jobs))
	}
	if src.lastOpts.Order != cast2md.OrderUpdatedDesc || src.lastOpts.Limit != 1 {
		t.Errorf("first run used %+v, want order=updated_desc limit=1", src.lastOpts)
	}

	after, err := svc.db.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if !after.Initialized {
		t.Error("subscription should be initialized after the first run")
	}
	if after.Watermark != "2026-08-02T10:00:00" {
		t.Errorf("watermark = %q, want the newest episode's updated_at", after.Watermark)
	}

	var rows int
	if err := svc.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos WHERE user_id = 1 AND source = 'podcast' AND source_feed_id = $1`,
		feedID).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 0 {
		t.Errorf("first run created %d rows, want 0", rows)
	}
}

// The second poll queues exactly the episodes newer than the watermark, and
// writes their rows before the watermark moves past them.
func TestPollSubscription_SecondRunQueuesOnlyNewEpisodes(t *testing.T) {
	feedID, numericFeedID := uniqueFeedID(t)
	src := &fakeSource{
		feeds: []cast2md.Feed{{ID: numericFeedID, Title: "Test Show", ImageURL: "https://img"}},
		episodes: map[int]cast2md.Episode{
			11: {ID: 11, FeedID: numericFeedID, Title: "Ep 11", Status: cast2md.StatusCompleted, UpdatedAt: "2026-08-03T10:00:00"},
			12: {ID: 12, FeedID: numericFeedID, Title: "Ep 12", Status: cast2md.StatusCompleted, UpdatedAt: "2026-08-04T10:00:00"},
		},
		listResult: []cast2md.Episode{
			{ID: 10, FeedID: numericFeedID, UpdatedAt: "2026-08-02T10:00:00"},
			{ID: 11, FeedID: numericFeedID, UpdatedAt: "2026-08-03T10:00:00"},
			{ID: 12, FeedID: numericFeedID, UpdatedAt: "2026-08-04T10:00:00"},
		},
	}
	svc := newPollTestService(t, src)
	ctx := context.Background()
	cleanupFeed(t, svc, 1, feedID)

	sub, err := svc.db.UpsertSubscription(ctx, 1, feedID, "Test Show", "https://img", true, "deep")
	if err != nil {
		t.Fatalf("UpsertSubscription: %v", err)
	}
	// Pretend the first run already happened and stopped at episode 10.
	if err := svc.db.SetSubscriptionWatermark(ctx, sub.ID, "2026-08-02T10:00:00", true); err != nil {
		t.Fatalf("SetSubscriptionWatermark: %v", err)
	}
	sub, err = svc.db.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}

	if err := svc.pollSubscription(ctx, *sub); err != nil {
		t.Fatalf("pollSubscription: %v", err)
	}

	jobs := drainEpisodeQueue(svc)
	if len(jobs) != 2 {
		t.Fatalf("queued %d jobs, want 2", len(jobs))
	}
	if jobs[0].episodeID != 11 || jobs[1].episodeID != 12 {
		t.Errorf("queued episodes %d and %d, want 11 and 12 in that order", jobs[0].episodeID, jobs[1].episodeID)
	}
	if jobs[0].level != "deep" {
		t.Errorf("job level = %q, want the subscription's detail level", jobs[0].level)
	}

	after, err := svc.db.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if after.Watermark != "2026-08-04T10:00:00" {
		t.Errorf("watermark = %q, want the last processed episode's updated_at", after.Watermark)
	}

	// The rows exist before the worker runs, which is what makes the in-memory
	// queue recoverable after a restart.
	row, err := svc.db.GetBySourceID(ctx, 1, storage.SourcePodcast, "11")
	if err != nil {
		t.Fatalf("GetBySourceID: %v", err)
	}
	if row.Status != "pending" {
		t.Errorf("row status = %q, want pending", row.Status)
	}
	if row.Channel != "Test Show" {
		t.Errorf("channel = %q, want the feed title", row.Channel)
	}
	if row.YouTubeID != "" {
		t.Errorf("YouTubeID = %q, want empty for a podcast row", row.YouTubeID)
	}
	if row.SourceURL != "https://cast2md.test/episodes/11" {
		t.Errorf("SourceURL = %q", row.SourceURL)
	}
}

// A cast2md that cannot be reached must leave the watermark alone, so the next
// tick retries the same range instead of skipping it.
func TestPollSubscription_ClientErrorLeavesWatermark(t *testing.T) {
	feedID, numericFeedID := uniqueFeedID(t)
	src := &fakeSource{
		feeds:   []cast2md.Feed{{ID: numericFeedID, Title: "Test Show"}},
		listErr: errors.New("connection refused"),
	}
	svc := newPollTestService(t, src)
	ctx := context.Background()
	cleanupFeed(t, svc, 1, feedID)

	sub, err := svc.db.UpsertSubscription(ctx, 1, feedID, "Test Show", "", true, "medium")
	if err != nil {
		t.Fatalf("UpsertSubscription: %v", err)
	}
	if err := svc.db.SetSubscriptionWatermark(ctx, sub.ID, "2026-08-02T10:00:00", true); err != nil {
		t.Fatalf("SetSubscriptionWatermark: %v", err)
	}
	sub, _ = svc.db.GetSubscription(ctx, 1, feedID)

	pollErr := svc.pollSubscription(ctx, *sub)
	if pollErr == nil {
		t.Fatal("pollSubscription should return the client error")
	}
	// pollOnce records the error; do the same here so the stored state matches.
	if err := svc.db.SetSubscriptionError(ctx, sub.ID, pollErr.Error()); err != nil {
		t.Fatalf("SetSubscriptionError: %v", err)
	}

	after, err := svc.db.GetSubscription(ctx, 1, feedID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if after.Watermark != "2026-08-02T10:00:00" {
		t.Errorf("watermark = %q, want it unchanged after a client error", after.Watermark)
	}
	if after.LastError == "" {
		t.Error("last_error should record the failure")
	}
	if after.LastPolledAt == nil {
		t.Error("last_polled_at should be set even on a failed poll")
	}
	if jobs := drainEpisodeQueue(svc); len(jobs) != 0 {
		t.Errorf("a failed poll queued %d jobs, want 0", len(jobs))
	}
}

// A broken episode must not block the rest of the feed: the watermark moves
// past it so later episodes are still reached.
func TestPollSubscription_BrokenEpisodeDoesNotBlockFeed(t *testing.T) {
	feedID, numericFeedID := uniqueFeedID(t)
	src := &fakeSource{
		feeds: []cast2md.Feed{{ID: numericFeedID, Title: "Test Show"}},
		episodes: map[int]cast2md.Episode{
			// Episode 21 is deliberately absent from the map, so
			// EnsureEpisodeRow fails for it.
			22: {ID: 22, FeedID: numericFeedID, Title: "Ep 22", Status: cast2md.StatusCompleted, UpdatedAt: "2026-08-06T10:00:00"},
		},
		listResult: []cast2md.Episode{
			{ID: 21, FeedID: numericFeedID, UpdatedAt: "2026-08-05T10:00:00"},
			{ID: 22, FeedID: numericFeedID, UpdatedAt: "2026-08-06T10:00:00"},
		},
	}
	svc := newPollTestService(t, src)
	ctx := context.Background()
	cleanupFeed(t, svc, 1, feedID)

	sub, err := svc.db.UpsertSubscription(ctx, 1, feedID, "Test Show", "", true, "medium")
	if err != nil {
		t.Fatalf("UpsertSubscription: %v", err)
	}
	if err := svc.db.SetSubscriptionWatermark(ctx, sub.ID, "2026-08-04T10:00:00", true); err != nil {
		t.Fatalf("SetSubscriptionWatermark: %v", err)
	}
	sub, _ = svc.db.GetSubscription(ctx, 1, feedID)

	if err := svc.pollSubscription(ctx, *sub); err != nil {
		t.Fatalf("pollSubscription: %v", err)
	}

	jobs := drainEpisodeQueue(svc)
	if len(jobs) != 1 || jobs[0].episodeID != 22 {
		t.Fatalf("queued %+v, want only episode 22", jobs)
	}
	after, _ := svc.db.GetSubscription(ctx, 1, feedID)
	if after.Watermark != "2026-08-06T10:00:00" {
		t.Errorf("watermark = %q, want it past the broken episode", after.Watermark)
	}
}

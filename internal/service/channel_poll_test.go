package service

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/youtube"
)

// fakeChannelSource is a channelFeedSource driven entirely from the test.
type fakeChannelSource struct {
	info     *youtube.ChannelInfo
	entries  []youtube.FeedEntry
	feedErr  error
	fetches  int
	resolves int
	// shorts marks video IDs the probe reports as Shorts; probes counts calls.
	shorts   map[string]bool
	shortErr error
	probes   int
}

func (f *fakeChannelSource) ResolveChannel(context.Context, string) (*youtube.ChannelInfo, error) {
	f.resolves++
	if f.info == nil {
		return nil, errors.New("no channel ID found")
	}
	return f.info, nil
}

func (f *fakeChannelSource) FetchChannelFeed(context.Context, string) ([]youtube.FeedEntry, error) {
	f.fetches++
	if f.feedErr != nil {
		return nil, f.feedErr
	}
	return f.entries, nil
}

func (f *fakeChannelSource) IsShort(_ context.Context, videoID string) (bool, error) {
	f.probes++
	if f.shortErr != nil {
		return false, f.shortErr
	}
	return f.shorts[videoID], nil
}

// newChannelTestService wires a Service against the development database and a
// fake feed source, reusing the podcast poller's DSN and skip rules.
func newChannelTestService(t *testing.T, src channelFeedSource) *Service {
	t.Helper()
	svc := newPollTestService(t, nil)
	svc.channels = src
	return svc
}

// uniqueChannelID derives a per-test channel ID so tests do not collide on
// UNIQUE(user_id, channel_id).
func uniqueChannelID(t *testing.T) string {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	return fmt.Sprintf("UCtest%016x", h.Sum32())
}

func cleanupChannel(t *testing.T, svc *Service, userID int, channelID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		// inbox_items cascade with the subscription; the video rows a
		// summarize test created need their own sweep.
		_, _ = svc.db.Pool.Exec(ctx,
			`DELETE FROM videos WHERE user_id = $1 AND youtube_id LIKE 'chtest_%'`, userID)
		_, _ = svc.db.Pool.Exec(ctx,
			`DELETE FROM channel_subscriptions WHERE user_id = $1 AND channel_id = $2`,
			userID, channelID)
	})
}

func drainProcessQueue(s *Service) []processJob {
	var jobs []processJob
	for {
		select {
		case j := <-s.queue:
			jobs = append(jobs, j)
		default:
			return jobs
		}
	}
}

// Subscribing imports the feed's current window and a second poll of the same
// window inserts nothing — the unique index is the whole dedup mechanism.
func TestSubscribeChannel_ImportsWindowOnce(t *testing.T) {
	channelID := uniqueChannelID(t)
	published := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	src := &fakeChannelSource{
		info: &youtube.ChannelInfo{ID: channelID, Title: "Test Channel"},
		entries: []youtube.FeedEntry{
			{VideoID: "chtest_a1", Title: "First video", Published: published},
			{VideoID: "chtest_a2", Title: "Second video", Published: published.Add(time.Hour)},
		},
	}
	svc := newChannelTestService(t, src)
	ctx := context.Background()
	cleanupChannel(t, svc, 1, channelID)

	sub, err := svc.SubscribeChannel(ctx, 1, "https://www.youtube.com/@testchannel")
	if err != nil {
		t.Fatalf("SubscribeChannel: %v", err)
	}
	if sub.Title != "Test Channel" || !sub.Enabled {
		t.Errorf("subscription = %+v, want enabled with the resolved title", sub)
	}
	if sub.LastPolledAt == nil {
		t.Error("the subscribe response should reflect the completed first poll")
	}

	items, total, err := svc.ListInbox(ctx, 1, "", sub.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("inbox holds %d items, want 2", total)
	}
	// Newest first.
	if items[0].YouTubeID != "chtest_a2" || items[1].YouTubeID != "chtest_a1" {
		t.Errorf("order = %s, %s — want newest first", items[0].YouTubeID, items[1].YouTubeID)
	}
	if items[0].ChannelTitle != "Test Channel" {
		t.Errorf("channel title = %q, want the subscription's title joined in", items[0].ChannelTitle)
	}
	if jobs := drainProcessQueue(svc); len(jobs) != 0 {
		t.Errorf("subscribing queued %d jobs, want 0 — the inbox is triage, not auto-summarize", len(jobs))
	}

	// The same window again: nothing new.
	if err := svc.pollChannel(ctx, *sub); err != nil {
		t.Fatalf("second pollChannel: %v", err)
	}
	_, total, err = svc.ListInbox(ctx, 1, "", sub.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListInbox after second poll: %v", err)
	}
	if total != 2 {
		t.Errorf("second poll grew the inbox to %d, want it unchanged at 2", total)
	}
}

// A video already in the library never becomes an inbox item, and a #shorts
// title is skipped.
func TestPollChannel_SkipsLibraryRowsAndShorts(t *testing.T) {
	channelID := uniqueChannelID(t)
	src := &fakeChannelSource{
		info: &youtube.ChannelInfo{ID: channelID, Title: "Skips"},
		entries: []youtube.FeedEntry{
			{VideoID: "chtest_lib", Title: "Already summarized"},
			{VideoID: "chtest_short", Title: "Quick tip #Shorts"},
			{VideoID: "chtest_new", Title: "Genuinely new"},
		},
	}
	svc := newChannelTestService(t, src)
	ctx := context.Background()
	cleanupChannel(t, svc, 1, channelID)

	if err := svc.db.InsertVideo(ctx, &storage.Video{
		ID:        uuid.New(),
		UserID:    1,
		YouTubeID: "chtest_lib",
		Status:    "completed",
	}); err != nil {
		t.Fatalf("insert library row: %v", err)
	}

	sub, err := svc.SubscribeChannel(ctx, 1, "@skips")
	if err != nil {
		t.Fatalf("SubscribeChannel: %v", err)
	}

	items, total, err := svc.ListInbox(ctx, 1, "", sub.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if total != 1 || items[0].YouTubeID != "chtest_new" {
		t.Errorf("inbox = %+v, want only the genuinely new video", items)
	}
}

// A failed fetch lands in last_error and a later success clears it.
func TestPollChannelsOnce_RecordsAndClearsError(t *testing.T) {
	channelID := uniqueChannelID(t)
	src := &fakeChannelSource{
		info:    &youtube.ChannelInfo{ID: channelID, Title: "Flaky"},
		feedErr: errors.New("channel feed returned status 404"),
	}
	svc := newChannelTestService(t, src)
	ctx := context.Background()
	cleanupChannel(t, svc, 1, channelID)

	// Subscribe records the failed first poll but keeps the subscription.
	sub, err := svc.SubscribeChannel(ctx, 1, "@flaky")
	if err != nil {
		t.Fatalf("SubscribeChannel: %v", err)
	}
	if sub.LastError == "" {
		t.Error("last_error should record the failed first poll")
	}

	src.feedErr = nil
	src.entries = []youtube.FeedEntry{{VideoID: "chtest_ok", Title: "Back again"}}
	svc.pollChannelsOnce(ctx)

	after, err := svc.db.GetChannelSubscription(ctx, 1, sub.ID)
	if err != nil {
		t.Fatalf("GetChannelSubscription: %v", err)
	}
	if after.LastError != "" {
		t.Errorf("last_error = %q, want it cleared by the successful poll", after.LastError)
	}
	if after.LastPolledAt == nil {
		t.Error("last_polled_at should be set")
	}
}

// SummarizeInboxItem creates the pending row synchronously, queues the job,
// marks the item, and a second call converges on the same row.
func TestSummarizeInboxItem(t *testing.T) {
	channelID := uniqueChannelID(t)
	published := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	src := &fakeChannelSource{
		info: &youtube.ChannelInfo{ID: channelID, Title: "Watchable"},
		entries: []youtube.FeedEntry{
			{VideoID: "chtest_w1", Title: "Worth watching", Published: published},
		},
	}
	svc := newChannelTestService(t, src)
	ctx := context.Background()
	cleanupChannel(t, svc, 1, channelID)

	sub, err := svc.SubscribeChannel(ctx, 1, "@watchable")
	if err != nil {
		t.Fatalf("SubscribeChannel: %v", err)
	}
	items, _, err := svc.ListInbox(ctx, 1, "", sub.ID, 0, 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListInbox: %v (%d items)", err, len(items))
	}

	video, err := svc.SummarizeInboxItem(ctx, 1, items[0].ID)
	if err != nil {
		t.Fatalf("SummarizeInboxItem: %v", err)
	}
	if video.Status != "pending" || video.YouTubeID != "chtest_w1" {
		t.Errorf("video = status %q id %q, want a pending row for the item", video.Status, video.YouTubeID)
	}
	if video.Title != "Worth watching" || video.Channel != "Watchable" {
		t.Errorf("title/channel = %q/%q, want them carried from the inbox item", video.Title, video.Channel)
	}
	if video.PublishedAt == nil || !video.PublishedAt.Equal(published) {
		t.Errorf("published_at = %v, want the feed's timestamp", video.PublishedAt)
	}

	jobs := drainProcessQueue(svc)
	if len(jobs) != 1 || jobs[0].youtubeID != "chtest_w1" {
		t.Fatalf("queued %+v, want one job for the video", jobs)
	}

	item, err := svc.db.GetInboxItem(ctx, 1, items[0].ID)
	if err != nil {
		t.Fatalf("GetInboxItem: %v", err)
	}
	if item.State != storage.InboxStateQueued {
		t.Errorf("item state = %q, want queued", item.State)
	}
	// The item left the triage list.
	if _, total, _ := svc.ListInbox(ctx, 1, "", sub.ID, 0, 0); total != 0 {
		t.Errorf("triage list still holds %d items, want 0", total)
	}

	// A second click converges on the same row.
	again, err := svc.SummarizeInboxItem(ctx, 1, items[0].ID)
	if err != nil {
		t.Fatalf("second SummarizeInboxItem: %v", err)
	}
	if again.ID != video.ID {
		t.Errorf("second call returned row %s, want the same row %s", again.ID, video.ID)
	}
}

// A video the probe marks as a Short lands dismissed, never in the triage
// list — and is probed exactly once, because the dismissed row blocks the
// re-insert on the next poll.
func TestPollChannel_ProbeFiltersShorts(t *testing.T) {
	channelID := uniqueChannelID(t)
	src := &fakeChannelSource{
		info: &youtube.ChannelInfo{ID: channelID, Title: "Mixed"},
		entries: []youtube.FeedEntry{
			{VideoID: "chtest_long", Title: "A real video"},
			{VideoID: "chtest_untagged", Title: "Sneaky short without tag"},
		},
		shorts: map[string]bool{"chtest_untagged": true},
	}
	svc := newChannelTestService(t, src)
	ctx := context.Background()
	cleanupChannel(t, svc, 1, channelID)

	sub, err := svc.SubscribeChannel(ctx, 1, "@mixed")
	if err != nil {
		t.Fatalf("SubscribeChannel: %v", err)
	}

	items, total, err := svc.ListInbox(ctx, 1, "", sub.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if total != 1 || items[0].YouTubeID != "chtest_long" {
		t.Errorf("inbox = %+v, want only the real video", items)
	}
	if src.probes != 2 {
		t.Errorf("first poll probed %d times, want 2 (every fresh video once)", src.probes)
	}

	// The next poll re-sees both entries but probes neither: the rows exist.
	if err := svc.pollChannel(ctx, *sub); err != nil {
		t.Fatalf("second pollChannel: %v", err)
	}
	if src.probes != 2 {
		t.Errorf("second poll probed again (total %d), want the dedup to prevent that", src.probes)
	}

	// A probe failure keeps the video rather than losing it.
	src.shortErr = errors.New("probe timeout")
	src.entries = append(src.entries, youtube.FeedEntry{VideoID: "chtest_flaky", Title: "Arrives despite probe error"})
	if err := svc.pollChannel(ctx, *sub); err != nil {
		t.Fatalf("third pollChannel: %v", err)
	}
	_, total, _ = svc.ListInbox(ctx, 1, "", sub.ID, 0, 0)
	if total != 2 {
		t.Errorf("inbox holds %d after flaky probe, want 2 — the video must not be lost", total)
	}
}

// A subscription whose channel cannot be resolved is not created.
func TestSubscribeChannel_ResolveFailure(t *testing.T) {
	src := &fakeChannelSource{}
	svc := newChannelTestService(t, src)

	if _, err := svc.SubscribeChannel(context.Background(), 1, "not a channel"); err == nil {
		t.Fatal("SubscribeChannel should fail when resolution fails")
	}
}

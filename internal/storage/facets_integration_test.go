package storage_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
)

func TestListVideoFacets(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	insert := func(channel, status string, topics []string, source string) uuid.UUID {
		t.Helper()
		meta := json.RawMessage(`{}`)
		if topics != nil {
			payload, err := json.Marshal(map[string]any{"topics": topics})
			if err != nil {
				t.Fatalf("marshal topics: %v", err)
			}
			meta = payload
		}
		v := &storage.Video{
			ID:           uuid.New(),
			UserID:       1,
			YouTubeID:    "facet_" + uuid.NewString()[:8],
			Source:       source,
			Channel:      channel,
			ThumbnailURL: "https://i.ytimg.com/vi/facet_" + channel + "/hqdefault.jpg",
			DetailLevel:  "medium",
			Metadata:     meta,
			Status:       status,
		}
		if source == storage.SourcePodcast {
			v.YouTubeID = ""
			v.ExternalID = "facet_" + uuid.NewString()[:8]
		}
		if err := store.InsertVideo(ctx, v); err != nil {
			t.Fatalf("insert video: %v", err)
		}
		t.Cleanup(func() { _ = store.DeleteVideo(ctx, 1, v.ID) })
		return v.ID
	}

	// A followed channel whose title matches the video rows' channel column
	// lends the facet its artwork.
	sub, err := store.UpsertChannelSubscription(ctx, 1, "UCtest_facets_0000000000", "Facet Go Channel", "https://art.example/go.jpg")
	if err != nil {
		t.Fatalf("UpsertChannelSubscription: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteChannelSubscription(ctx, 1, sub.ID) })

	insert("Facet Go Channel", "completed", []string{"facet-go", "facet-testing"}, storage.SourceYouTube)
	insert("Facet Go Channel", "completed", []string{"facet-go"}, storage.SourceYouTube)
	insert("Facet Google Talks", "completed", nil, storage.SourceYouTube)
	// Pending rows and other sources stay out of the youtube facets.
	insert("Facet Go Channel", "pending", []string{"facet-go"}, storage.SourceYouTube)
	insert("Facet Podcast Show", "completed", []string{"facet-audio"}, storage.SourcePodcast)

	facets, err := store.ListVideoFacets(ctx, 1, storage.SourceYouTube)
	if err != nil {
		t.Fatalf("ListVideoFacets: %v", err)
	}

	channelCount := func(name string) int {
		for _, c := range facets.Channels {
			if c.Channel == name {
				return c.Count
			}
		}
		return 0
	}
	if got := channelCount("Facet Go Channel"); got != 2 {
		t.Errorf("Facet Go Channel count = %d, want 2 (pending row excluded)", got)
	}
	if got := channelCount("Facet Google Talks"); got != 1 {
		t.Errorf("Facet Google Talks count = %d, want 1", got)
	}
	for _, c := range facets.Channels {
		// The subscription's avatar outranks the video-thumbnail fallback.
		if c.Channel == "Facet Go Channel" && c.ThumbnailURL != "https://art.example/go.jpg" {
			t.Errorf("followed channel artwork = %q, want the subscription's thumbnail", c.ThumbnailURL)
		}
		// An unfollowed channel gets its newest video's thumbnail instead.
		if c.Channel == "Facet Google Talks" &&
			c.ThumbnailURL != "https://i.ytimg.com/vi/facet_Facet Google Talks/hqdefault.jpg" {
			t.Errorf("unfollowed channel artwork = %q, want the video-thumbnail fallback", c.ThumbnailURL)
		}
	}
	if got := channelCount("Facet Podcast Show"); got != 0 {
		t.Errorf("podcast channel leaked into youtube facets with count %d", got)
	}

	topicCount := func(name string) int {
		for _, tc := range facets.Topics {
			if tc.Topic == name {
				return tc.Count
			}
		}
		return 0
	}
	if got := topicCount("facet-go"); got != 2 {
		t.Errorf("facet-go count = %d, want 2", got)
	}
	if got := topicCount("facet-audio"); got != 0 {
		t.Errorf("podcast topic leaked into youtube facets with count %d", got)
	}

	// The exact channel filter separates what ILIKE conflates.
	exact, _, err := store.ListRecent(ctx, 1,
		storage.ListFilters{ChannelExact: "Facet Go Channel", Source: storage.SourceYouTube}, 50, 0)
	if err != nil {
		t.Fatalf("ListRecent exact: %v", err)
	}
	partial, _, err := store.ListRecent(ctx, 1,
		storage.ListFilters{Channel: "Facet Go", Source: storage.SourceYouTube}, 50, 0)
	if err != nil {
		t.Fatalf("ListRecent partial: %v", err)
	}
	if len(exact) >= len(partial) {
		t.Errorf("exact matched %d rows, partial %d — exact must be the narrower filter here",
			len(exact), len(partial))
	}
	for _, v := range exact {
		if v.Channel != "Facet Go Channel" {
			t.Errorf("exact filter returned channel %q", v.Channel)
		}
	}
}

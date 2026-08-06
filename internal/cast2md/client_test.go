package cast2md

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client(), 5*time.Second, 5*time.Second), srv
}

func TestListFeeds(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/feeds" {
			t.Errorf("path = %q, want /api/feeds", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"feeds":[{"id":3,"title":"Raw","display_title":"The Show","image_url":"https://img","episode_count":42}]}`)
	}))

	feeds, err := c.ListFeeds(context.Background())
	if err != nil {
		t.Fatalf("ListFeeds: %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("got %d feeds, want 1", len(feeds))
	}
	if feeds[0].Name() != "The Show" {
		t.Errorf("Name() = %q, want the display title", feeds[0].Name())
	}
	if feeds[0].EpisodeCount != 42 {
		t.Errorf("EpisodeCount = %d, want 42", feeds[0].EpisodeCount)
	}

	// The second call is served from the cache.
	if _, err := c.ListFeeds(context.Background()); err != nil {
		t.Fatalf("second ListFeeds: %v", err)
	}
	if calls != 1 {
		t.Errorf("upstream called %d times, want 1 (the list is cached)", calls)
	}
}

func TestFeedNameFallsBackToTitle(t *testing.T) {
	f := Feed{Title: "Raw Title"}
	if f.Name() != "Raw Title" {
		t.Errorf("Name() = %q, want the raw title when display_title is empty", f.Name())
	}
}

func TestGetEpisode(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/episodes/11481" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"id":11481,"feed_id":3,"title":"Episode 12","status":"completed",
			"duration_seconds":16776,"published_at":"2026-07-30T08:00:00",
			"updated_at":"2026-08-01T10:11:12"}`)
	}))

	ep, err := c.GetEpisode(context.Background(), 11481)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}
	if ep.Status != StatusCompleted {
		t.Errorf("Status = %q", ep.Status)
	}
	if ep.UpdatedAt != "2026-08-01T10:11:12" {
		t.Errorf("UpdatedAt = %q — the watermark must be carried through verbatim", ep.UpdatedAt)
	}
}

func TestListCompletedPassesFilters(t *testing.T) {
	var gotQuery string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/episodes/status/completed" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, `{"episodes":[{"id":7,"feed_id":3,"updated_at":"2026-08-02T09:00:00"}],"total":1}`)
	}))

	eps, err := c.ListCompleted(context.Background(), ListCompletedOptions{
		Since:  "2026-08-01T10:11:12",
		FeedID: 3,
		Order:  OrderUpdatedAsc,
		Limit:  25,
	})
	if err != nil {
		t.Fatalf("ListCompleted: %v", err)
	}
	if len(eps) != 1 || eps[0].ID != 7 {
		t.Fatalf("episodes = %+v", eps)
	}
	for _, want := range []string{"since=2026-08-01T10%3A11%3A12", "feed_id=3", "order=updated_asc", "limit=25"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q is missing %q", gotQuery, want)
		}
	}
}

func TestListCompletedOmitsEmptyFilters(t *testing.T) {
	var gotQuery string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, `{"episodes":[],"total":0}`)
	}))

	if _, err := c.ListCompleted(context.Background(), ListCompletedOptions{Order: OrderUpdatedDesc, Limit: 1}); err != nil {
		t.Fatalf("ListCompleted: %v", err)
	}
	if strings.Contains(gotQuery, "since=") {
		t.Errorf("query %q should not carry an empty since", gotQuery)
	}
	if strings.Contains(gotQuery, "feed_id=") {
		t.Errorf("query %q should not carry a zero feed_id", gotQuery)
	}
}

func TestGetTranscript(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "txt" {
			t.Errorf("format = %q, want txt", r.URL.Query().Get("format"))
		}
		_, _ = fmt.Fprint(w, "  Speaker one says a thing.\n")
	}))

	text, err := c.GetTranscript(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetTranscript: %v", err)
	}
	if text != "Speaker one says a thing." {
		t.Errorf("transcript = %q", text)
	}
}

func TestGetTranscriptRejectsEmptyBody(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "   \n")
	}))

	if _, err := c.GetTranscript(context.Background(), 7); err == nil {
		t.Fatal("an empty transcript should be an error, not an empty summary input")
	}
}

func TestServerErrorSurfaces(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))

	_, err := c.ListFeeds(context.Background())
	if err == nil {
		t.Fatal("a 500 should be an error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q should name the status code", err)
	}
}

func TestBodyIsCapped(t *testing.T) {
	// A body larger than the cap is truncated rather than read into memory in
	// full; the truncated JSON then fails to parse, which is the visible effect.
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"feeds":[`)
		chunk := strings.Repeat("x", 1<<20)
		for i := 0; i < 20; i++ {
			_, _ = fmt.Fprintf(w, `{"title":"%s"},`, chunk)
		}
		_, _ = fmt.Fprint(w, `]}`)
	}))

	if _, err := c.ListFeeds(context.Background()); err == nil {
		t.Fatal("an over-long body should fail to parse rather than be accepted")
	}
}

func TestEpisodeURL(t *testing.T) {
	c := New("https://cast2md.example.ts.net/", nil, 0, 0)
	if got := c.EpisodeURL(42); got != "https://cast2md.example.ts.net/episodes/42" {
		t.Errorf("EpisodeURL = %q", got)
	}
	if got := c.BaseURL(); got != "https://cast2md.example.ts.net" {
		t.Errorf("BaseURL = %q — the trailing slash should be trimmed", got)
	}
}

package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withFeedServer points FetchChannelFeed at handler and shortens the retry
// delays, so a test that exercises all three attempts finishes in
// milliseconds.
func withFeedServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	baseURL, delays := channelFeedBaseURL, channelFeedRetryDelays
	channelFeedBaseURL = srv.URL + "/feeds/videos.xml?channel_id="
	channelFeedRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() {
		channelFeedBaseURL, channelFeedRetryDelays = baseURL, delays
	})
}

// The endpoint answers 404 for a live channel about half the time, so two
// failures followed by a feed is the case the retry exists for.
func TestFetchChannelFeed_RetriesTransient404(t *testing.T) {
	calls := 0
	withFeedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(feedFixture))
	})

	entries, err := (&Client{}).FetchChannelFeed(context.Background(), "UCs6KfncB4OV6Vug4o_bzijg")
	if err != nil {
		t.Fatalf("FetchChannelFeed: %v", err)
	}
	if calls != 3 {
		t.Errorf("server saw %d requests, want 3", calls)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want the fixture's 2", len(entries))
	}
}

// A deleted channel answers 404 on every attempt, and that error is what the
// subscription's last_error should carry.
func TestFetchChannelFeed_PermanentFailureAfterRetries(t *testing.T) {
	calls := 0
	withFeedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := (&Client{}).FetchChannelFeed(context.Background(), "UCdeleted00000000000000")
	if err == nil {
		t.Fatal("a feed that always answers 404 should fail")
	}
	if calls != len(channelFeedRetryDelays)+1 {
		t.Errorf("server saw %d requests, want %d", calls, len(channelFeedRetryDelays)+1)
	}
}

// A status no retry can turn into a feed costs exactly one request.
func TestFetchChannelFeed_NoRetryOnClientError(t *testing.T) {
	calls := 0
	withFeedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	})

	if _, err := (&Client{}).FetchChannelFeed(context.Background(), "UCforbidden000000000000"); err == nil {
		t.Fatal("a 403 should fail")
	}
	if calls != 1 {
		t.Errorf("server saw %d requests, want 1", calls)
	}
}

// A cancelled context ends the wait instead of sleeping out the backoff.
func TestFetchChannelFeed_ContextCancelledDuringBackoff(t *testing.T) {
	withFeedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	channelFeedRetryDelays = []time.Duration{time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (&Client{}).FetchChannelFeed(ctx, "UCcancelled00000000000")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled fetch should fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FetchChannelFeed sat out the backoff instead of following the context")
	}
}

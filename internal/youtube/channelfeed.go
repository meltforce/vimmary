// Channel discovery via YouTube's public per-channel RSS feed. The name avoids
// a clash with internal/feed, which is vimmary's own Atom output.
package youtube

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FeedEntry is one video in a channel's RSS feed.
type FeedEntry struct {
	VideoID   string
	Title     string
	Published time.Time
}

// channelFeedXML mirrors the Atom shape of
// https://www.youtube.com/feeds/videos.xml?channel_id=UC…
type channelFeedXML struct {
	Entries []struct {
		VideoID   string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
		Title     string `xml:"title"`
		Published string `xml:"published"`
	} `xml:"entry"`
}

// channelFeedBaseURL is a variable so a test can point the fetch at an
// httptest server. Nothing at runtime writes it.
var channelFeedBaseURL = "https://www.youtube.com/feeds/videos.xml?channel_id="

// channelFeedRetryDelays spaces the retries inside one feed fetch, and its
// existence is the whole point: the endpoint answers 404 for a live channel
// with a correct ID in roughly half of all requests. Measured 2026-08-25, 12
// consecutive requests per channel — Techlore 5 of 12 answered 200, Veritasium
// 3 of 12 — with the same quota across User-Agent, Accept header, consent
// cookie, HTTP/1.1 instead of HTTP/2, and the www-less host, and with Google's
// generic error page as the 404 body rather than YouTube's. Three attempts take
// the per-cycle failure probability from ~50% to ~12%. A deleted channel
// answers 404 on every attempt and still ends up in last_error.
var channelFeedRetryDelays = []time.Duration{time.Second, 3 * time.Second}

// FetchChannelFeed returns the channel's ~15 newest videos. It never touches
// InnerTube, so polling many channels puts no pressure on the transcript
// pipeline's rate budget.
func (c *Client) FetchChannelFeed(ctx context.Context, channelID string) ([]FeedEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelFeedBaseURL+channelID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	// The request carries no body, so the same value serves every attempt.
	for attempt := 0; ; attempt++ {
		body, status, err := fetchChannelFeedOnce(req)
		if err == nil {
			return parseChannelFeed(body)
		}
		if attempt >= len(channelFeedRetryDelays) || !retryableFeedStatus(status) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(channelFeedRetryDelays[attempt]):
		}
	}
}

// retryableFeedStatus decides which failures a second attempt can still turn
// into a feed. 0 stands for a request that produced no response at all.
func retryableFeedStatus(status int) bool {
	return status == 0 || status == http.StatusNotFound || status >= 500
}

// fetchChannelFeedOnce performs one request and returns the response status
// alongside the error, so the caller decides about the retry. The status is 0
// when the request produced no response.
func fetchChannelFeedOnce(req *http.Request) ([]byte, int, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch channel feed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// A 404 that survives every attempt is a deleted or mistyped channel; it
	// belongs in the subscription's last_error, not in a silent empty result.
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("channel feed returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read channel feed: %w", err)
	}
	return body, resp.StatusCode, nil
}

// parseChannelFeed decodes the Atom payload. Entries without a video ID are
// dropped; an unparsable published date leaves the zero time rather than
// failing the whole feed.
func parseChannelFeed(body []byte) ([]FeedEntry, error) {
	var feed channelFeedXML
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse channel feed: %w", err)
	}

	entries := make([]FeedEntry, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if e.VideoID == "" {
			continue
		}
		published, _ := time.Parse(time.RFC3339, e.Published)
		entries = append(entries, FeedEntry{VideoID: e.VideoID, Title: e.Title, Published: published})
	}
	return entries, nil
}

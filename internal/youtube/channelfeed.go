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

// FetchChannelFeed returns the channel's ~15 newest videos. It never touches
// InnerTube, so polling many channels puts no pressure on the transcript
// pipeline's rate budget.
func (c *Client) FetchChannelFeed(ctx context.Context, channelID string) ([]FeedEntry, error) {
	url := "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch channel feed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// A 404 is a deleted or mistyped channel; either way it belongs in the
	// subscription's last_error, not in a silent empty result.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("channel feed returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read channel feed: %w", err)
	}
	return parseChannelFeed(body)
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

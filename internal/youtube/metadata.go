package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
)

var playerResponseRe = regexp.MustCompile(`var ytInitialPlayerResponse\s*=\s*(\{.+?\});`)

// FetchMetadata retrieves video metadata from the YouTube watch page.
func (c *Client) FetchMetadata(ctx context.Context, youtubeID string) (*Metadata, error) {
	url := "https://www.youtube.com/watch?v=" + youtubeID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch watch page: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("watch page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read watch page: %w", err)
	}

	matches := playerResponseRe.FindSubmatch(body)
	if matches == nil {
		return nil, fmt.Errorf("ytInitialPlayerResponse not found in watch page for %s", youtubeID)
	}

	var playerResp struct {
		VideoDetails struct {
			Title         string `json:"title"`
			Author        string `json:"author"`
			LengthSeconds string `json:"lengthSeconds"`
		} `json:"videoDetails"`
	}
	if err := json.Unmarshal(matches[1], &playerResp); err != nil {
		return nil, fmt.Errorf("parse player response: %w", err)
	}

	duration, _ := strconv.Atoi(playerResp.VideoDetails.LengthSeconds)

	return &Metadata{
		Title:           playerResp.VideoDetails.Title,
		Channel:         playerResp.VideoDetails.Author,
		DurationSeconds: duration,
	}, nil
}

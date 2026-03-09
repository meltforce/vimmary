package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// FetchMetadata retrieves video metadata via yt-dlp --dump-json.
func (c *Client) FetchMetadata(ctx context.Context, youtubeID string) (*Metadata, error) {
	url := "https://www.youtube.com/watch?v=" + youtubeID

	cmd := exec.CommandContext(ctx, c.ytdlpPath,
		"--dump-json",
		"--no-download",
		"--no-playlist",
		url,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp metadata: %w", err)
	}

	var info struct {
		Title    string  `json:"title"`
		Channel  string  `json:"channel"`
		Uploader string  `json:"uploader"`
		Duration float64 `json:"duration"`
		Language string  `json:"language"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("parse yt-dlp metadata: %w", err)
	}

	channel := info.Channel
	if channel == "" {
		channel = info.Uploader
	}

	return &Metadata{
		Title:           info.Title,
		Channel:         channel,
		DurationSeconds: int(info.Duration),
		Language:        info.Language,
	}, nil
}

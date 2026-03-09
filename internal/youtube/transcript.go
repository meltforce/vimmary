package youtube

import (
	"context"
	"fmt"
	"strings"
)

// FetchTranscript fetches captions using the YouTube InnerTube API.
func (c *Client) FetchTranscript(_ context.Context, youtubeID string) (*Transcript, error) {
	transcripts, err := c.transcriptClient.GetTranscripts(youtubeID, c.subLangs)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript for %s: %w", youtubeID, err)
	}

	if len(transcripts) == 0 {
		return nil, fmt.Errorf("no transcripts available for %s", youtubeID)
	}

	// Use the first available transcript (ordered by subLangs preference)
	t := transcripts[0]

	var lines []string
	for _, line := range t.Lines {
		text := strings.TrimSpace(line.Text)
		if text != "" {
			lines = append(lines, text)
		}
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("empty transcript for %s", youtubeID)
	}

	source := "manual"
	if t.IsGenerated {
		source = "auto"
	}

	return &Transcript{
		Text:     strings.Join(lines, " "),
		Language: t.LanguageCode,
		Source:   source,
	}, nil
}

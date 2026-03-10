package youtube

import (
	"context"
	"fmt"
	"sort"
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

	// The library returns transcripts in non-deterministic order (goroutines).
	// Re-sort by subLangs preference, preferring manual over auto-generated.
	langRank := make(map[string]int, len(c.subLangs))
	for i, lang := range c.subLangs {
		langRank[lang] = i
	}
	sort.SliceStable(transcripts, func(i, j int) bool {
		ri, oki := langRank[transcripts[i].LanguageCode]
		rj, okj := langRank[transcripts[j].LanguageCode]
		if !oki {
			ri = len(c.subLangs)
		}
		if !okj {
			rj = len(c.subLangs)
		}
		if ri != rj {
			return ri < rj
		}
		// Same language: prefer manual over auto-generated
		return !transcripts[i].IsGenerated && transcripts[j].IsGenerated
	})

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

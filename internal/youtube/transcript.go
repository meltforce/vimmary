package youtube

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// FetchTranscript fetches captions using the YouTube InnerTube API.
//
// spokenLang is the video's original language as Metadata.Language reports
// it, or "". A track in that language wins over the subtitle preference, so a
// German video with an English track is transcribed — and summarised — in
// German.
func (c *Client) FetchTranscript(_ context.Context, youtubeID, spokenLang string) (*Transcript, error) {
	langs := transcriptLanguages(spokenLang, c.subLangs)
	transcripts, err := c.transcriptClient.GetTranscripts(youtubeID, langs)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript for %s: %w", youtubeID, err)
	}

	if len(transcripts) == 0 {
		return nil, fmt.Errorf("no transcripts available for %s", youtubeID)
	}

	// The library returns transcripts in non-deterministic order (goroutines).
	// Re-sort by language preference, preferring manual over auto-generated.
	langRank := make(map[string]int, len(langs))
	for i, lang := range langs {
		langRank[lang] = i
	}
	sort.SliceStable(transcripts, func(i, j int) bool {
		ri, oki := langRank[transcripts[i].LanguageCode]
		rj, okj := langRank[transcripts[j].LanguageCode]
		if !oki {
			ri = len(langs)
		}
		if !okj {
			rj = len(langs)
		}
		if ri != rj {
			return ri < rj
		}
		// Same language: prefer manual over auto-generated
		return !transcripts[i].IsGenerated && transcripts[j].IsGenerated
	})

	t := transcripts[0]

	var lines []string
	var timed []Line
	for _, line := range t.Lines {
		text := strings.TrimSpace(line.Text)
		if text != "" {
			lines = append(lines, text)
			timed = append(timed, Line{Start: line.Start, Duration: line.Duration, Text: text})
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
		Lines:    timed,
	}, nil
}

// transcriptLanguages is the caption language order for one video: the spoken
// language first when it is known, then the configured preference.
func transcriptLanguages(spokenLang string, subLangs []string) []string {
	if spokenLang == "" || (len(subLangs) > 0 && subLangs[0] == spokenLang) {
		return subLangs
	}
	langs := []string{spokenLang}
	for _, l := range subLangs {
		if l != spokenLang {
			langs = append(langs, l)
		}
	}
	return langs
}

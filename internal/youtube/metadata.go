package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
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

	var playerResp playerResponse
	if err := json.Unmarshal(matches[1], &playerResp); err != nil {
		return nil, fmt.Errorf("parse player response: %w", err)
	}

	duration, _ := strconv.Atoi(playerResp.VideoDetails.LengthSeconds)

	return &Metadata{
		Title:           playerResp.VideoDetails.Title,
		Channel:         playerResp.VideoDetails.Author,
		DurationSeconds: duration,
		Language:        playerResp.spokenLanguage(),
	}, nil
}

type playerResponse struct {
	VideoDetails struct {
		Title         string `json:"title"`
		Author        string `json:"author"`
		LengthSeconds string `json:"lengthSeconds"`
	} `json:"videoDetails"`
	Captions struct {
		Renderer struct {
			CaptionTracks []struct {
				LanguageCode string `json:"languageCode"`
				Kind         string `json:"kind"`
			} `json:"captionTracks"`
		} `json:"playerCaptionsTracklistRenderer"`
	} `json:"captions"`
	StreamingData struct {
		AdaptiveFormats []struct {
			AudioTrack *struct {
				ID           string `json:"id"`
				DisplayName  string `json:"displayName"`
				IsAutoDubbed bool   `json:"isAutoDubbed"`
			} `json:"audioTrack"`
		} `json:"adaptiveFormats"`
	} `json:"streamingData"`
}

// spokenLanguage is the base language code ("de", "en") of the video's
// original audio, or "" when the watch page does not settle it.
//
// The caption tracks alone do not: a German video can carry an English track,
// either uploaded by the creator or recognised from an English auto-dub
// (YouTube then lists "a.en" beside "a.de", both kind "asr"). The subtitle
// preference picks English in both cases, and the summary followed it.
//
// The original audio track settles it where audio tracks exist: its ID is
// "<lang>.<n>" ("de-DE.4"), it is not auto-dubbed, and its display name ends
// in "original" (the request asks for English). Without audio tracks, a single
// speech-recognition track is in the spoken language, because YouTube
// recognises speech only on audio it has.
func (p *playerResponse) spokenLanguage() string {
	var undubbed []string
	for _, f := range p.StreamingData.AdaptiveFormats {
		t := f.AudioTrack
		if t == nil || t.ID == "" {
			continue
		}
		lang, _, _ := strings.Cut(t.ID, ".")
		if strings.HasSuffix(strings.ToLower(t.DisplayName), "original") {
			return baseLanguage(lang)
		}
		if !t.IsAutoDubbed && !slices.Contains(undubbed, lang) {
			undubbed = append(undubbed, lang)
		}
	}
	if len(undubbed) == 1 {
		return baseLanguage(undubbed[0])
	}

	var asr []string
	for _, t := range p.Captions.Renderer.CaptionTracks {
		if t.Kind == "asr" && !slices.Contains(asr, t.LanguageCode) {
			asr = append(asr, t.LanguageCode)
		}
	}
	if len(asr) == 1 && len(undubbed) == 0 {
		return baseLanguage(asr[0])
	}
	return ""
}

// baseLanguage reduces "de-DE" to "de".
func baseLanguage(code string) string {
	base, _, _ := strings.Cut(strings.ToLower(code), "-")
	return base
}

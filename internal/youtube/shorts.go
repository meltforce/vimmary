package youtube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// shortsProbeClient does not follow redirects — the redirect IS the answer.
var shortsProbeClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Timeout: 15 * time.Second,
}

// IsShort reports whether a video is a YouTube Short, by requesting
// /shorts/{id} without following redirects: a Short answers 200, a regular
// video redirects to its /watch page. The channel RSS feed carries no format
// marker, so this probe is the only reliable signal, at one request per
// newly seen video.
//
// The SOCS cookie skips the EU consent interstitial, which otherwise answers
// 302 to consent.youtube.com for both kinds (verified 2026-08-23 from a
// German network).
func (c *Client) IsShort(ctx context.Context, videoID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.youtube.com/shorts/"+videoID, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", "SOCS=CAI")

	resp, err := shortsProbeClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("shorts probe: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	return classifyShortsProbe(resp.StatusCode, resp.Header.Get("Location"))
}

// classifyShortsProbe maps the probe response onto short / not-short /
// unknown. A redirect to anything but a /watch page — the consent wall, a
// region block — is unknown rather than a guess.
func classifyShortsProbe(status int, location string) (bool, error) {
	switch {
	case status == http.StatusOK:
		return true, nil
	case status >= 300 && status < 400 && strings.Contains(location, "/watch"):
		return false, nil
	default:
		return false, fmt.Errorf("shorts probe answered %d (location %q)", status, location)
	}
}

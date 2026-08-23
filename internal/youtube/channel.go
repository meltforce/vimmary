package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// ChannelInfo identifies a channel for a subscription row.
type ChannelInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnail_url"`
}

var (
	// A channel ID inside a URL or a page body: UC plus 22 ID characters.
	channelURLIDRe = regexp.MustCompile(`/channel/(UC[\w-]{22})`)
	// The canonical link names the channel the page is actually about. The
	// first "channelId" in the body does not: on a handle page YouTube may
	// localize the data island to a regional sibling channel — @veritasium
	// carried "Veritasium en Français" there while the canonical link held
	// the real channel (observed 2026-08-23).
	canonicalChannelRe = regexp.MustCompile(`rel="canonical" href="https://www\.youtube\.com/channel/(UC[\w-]{22})"`)
	channelPageIDRe    = regexp.MustCompile(`"channelId":"(UC[\w-]{22})"`)
	ogTitleRe          = regexp.MustCompile(`<meta property="og:title" content="([^"]*)"`)
	ogImageRe          = regexp.MustCompile(`<meta property="og:image" content="([^"]*)"`)
)

// ResolveChannel turns whatever a user pastes — a /channel/UC… URL, an
// @handle, a /c/ or /user/ URL, or a bare handle — into a channel ID with a
// display title. A direct /channel/ URL resolves without a network call;
// everything else costs one fetch of the channel page, once, at subscribe
// time.
func (c *Client) ResolveChannel(ctx context.Context, input string) (*ChannelInfo, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty channel input")
	}

	if m := channelURLIDRe.FindStringSubmatch(input); m != nil {
		// The ID alone is enough to subscribe; title and artwork still come
		// from the page so the subscription row is not blank.
		info, err := c.fetchChannelPage(ctx, "https://www.youtube.com/channel/"+m[1])
		if err != nil {
			return &ChannelInfo{ID: m[1]}, nil
		}
		info.ID = m[1]
		return info, nil
	}

	pageURL, err := normalizeChannelURL(input)
	if err != nil {
		return nil, err
	}
	info, err := c.fetchChannelPage(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	if info.ID == "" {
		return nil, fmt.Errorf("no channel ID found at %s", pageURL)
	}
	return info, nil
}

// normalizeChannelURL maps the accepted input forms onto a fetchable page URL.
func normalizeChannelURL(input string) (string, error) {
	if strings.HasPrefix(input, "@") {
		return "https://www.youtube.com/" + input, nil
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		if !strings.Contains(input, "youtube.com/") {
			return "", fmt.Errorf("not a YouTube URL: %s", input)
		}
		return input, nil
	}
	if strings.Contains(input, "youtube.com/") {
		return "https://" + input, nil
	}
	// A bare word is treated as a handle.
	return "https://www.youtube.com/@" + input, nil
}

// fetchChannelPage loads a channel page and extracts ID, title and artwork.
func (c *Client) fetchChannelPage(ctx context.Context, url string) (*ChannelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch channel page: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("channel page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read channel page: %w", err)
	}
	return parseChannelPage(body), nil
}

// parseChannelPage extracts the channel identity from a channel page body.
// The canonical link wins over the body's first "channelId" — see the regex
// comment above.
func parseChannelPage(body []byte) *ChannelInfo {
	info := &ChannelInfo{}
	if m := canonicalChannelRe.FindSubmatch(body); m != nil {
		info.ID = string(m[1])
	} else if m := channelPageIDRe.FindSubmatch(body); m != nil {
		info.ID = string(m[1])
	}
	if m := ogTitleRe.FindSubmatch(body); m != nil {
		info.Title = string(m[1])
	}
	if m := ogImageRe.FindSubmatch(body); m != nil {
		info.ThumbnailURL = string(m[1])
	}
	return info
}

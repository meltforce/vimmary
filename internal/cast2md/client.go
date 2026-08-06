// Package cast2md is a read-only client for the cast2md podcast transcription
// service. cast2md lives inside the tailnet and has no authentication, so the
// HTTP client passed to New must be able to dial tailnet addresses.
package cast2md

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxBodyBytes caps every response read. The largest real transcript measured
// in cast2md is 311 kB, so 16 MB is headroom rather than a working limit.
const maxBodyBytes = 16 << 20

// feedCacheTTL is how long the feed list is reused. Feeds change when the user
// adds one in cast2md, which is rare enough that ten minutes of staleness
// costs nothing.
const feedCacheTTL = 10 * time.Minute

// Feed is one podcast feed in cast2md.
type Feed struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	DisplayTitle string `json:"display_title"`
	ImageURL     string `json:"image_url"`
	Link         string `json:"link"`
	EpisodeCount int    `json:"episode_count"`
}

// Name returns the title to show, preferring cast2md's display title.
func (f Feed) Name() string {
	if f.DisplayTitle != "" {
		return f.DisplayTitle
	}
	return f.Title
}

// Episode is one podcast episode in cast2md.
type Episode struct {
	ID              int    `json:"id"`
	FeedID          int    `json:"feed_id"`
	GUID            string `json:"guid"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	DurationSeconds int    `json:"duration_seconds"`
	PublishedAt     string `json:"published_at"`
	Status          string `json:"status"`
	TranscriptPath  string `json:"transcript_path"`
	CreatedAt       string `json:"created_at"`
	// UpdatedAt is a naive local timestamp written by cast2md. It is the
	// watermark value and is never parsed or reformatted on this side.
	UpdatedAt string `json:"updated_at"`
}

// StatusCompleted is the only episode status vimmary summarizes.
const StatusCompleted = "completed"

// Sort orders accepted by ListCompleted, matching cast2md's whitelist.
const (
	OrderCreatedAsc  = "created_asc"
	OrderUpdatedAsc  = "updated_asc"
	OrderUpdatedDesc = "updated_desc"
)

// Client talks to one cast2md instance.
type Client struct {
	baseURL string
	// Two clients over the same transport: the metadata one is short, the
	// transcript one has to survive a few hundred kilobytes of text. A single
	// client with the metadata timeout would cut long transcripts off.
	metadata   *http.Client
	transcript *http.Client

	mu          sync.Mutex
	feeds       []Feed
	feedsCached time.Time
}

// New creates a client. base supplies the transport and must reach the
// tailnet; its own Timeout is not used.
func New(baseURL string, base *http.Client, metadataTimeout, transcriptTimeout time.Duration) *Client {
	if metadataTimeout <= 0 {
		metadataTimeout = 15 * time.Second
	}
	if transcriptTimeout <= 0 {
		transcriptTimeout = 60 * time.Second
	}
	var transport http.RoundTripper
	if base != nil {
		transport = base.Transport
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		metadata:   &http.Client{Transport: transport, Timeout: metadataTimeout},
		transcript: &http.Client{Transport: transport, Timeout: transcriptTimeout},
	}
}

// BaseURL returns the configured cast2md base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// EpisodeURL returns the cast2md web page for one episode.
func (c *Client) EpisodeURL(episodeID int) string {
	return fmt.Sprintf("%s/episodes/%d", c.baseURL, episodeID)
}

func (c *Client) do(ctx context.Context, path string, httpClient *http.Client) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cast2md request %s: %w", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read cast2md response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("cast2md %s returned %d: %s", path, resp.StatusCode, snippet)
	}
	return body, nil
}

// ListFeeds returns every feed known to cast2md, cached for feedCacheTTL.
func (c *Client) ListFeeds(ctx context.Context) ([]Feed, error) {
	c.mu.Lock()
	if c.feeds != nil && time.Since(c.feedsCached) < feedCacheTTL {
		feeds := c.feeds
		c.mu.Unlock()
		return feeds, nil
	}
	c.mu.Unlock()

	body, err := c.do(ctx, "/api/feeds", c.metadata)
	if err != nil {
		return nil, err
	}
	var result struct {
		Feeds []Feed `json:"feeds"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse feed list: %w", err)
	}

	c.mu.Lock()
	c.feeds = result.Feeds
	c.feedsCached = time.Now()
	c.mu.Unlock()

	return result.Feeds, nil
}

// GetFeed returns one feed by ID from the cached list.
func (c *Client) GetFeed(ctx context.Context, feedID int) (*Feed, error) {
	feeds, err := c.ListFeeds(ctx)
	if err != nil {
		return nil, err
	}
	for i := range feeds {
		if feeds[i].ID == feedID {
			return &feeds[i], nil
		}
	}
	return nil, fmt.Errorf("feed %d not found in cast2md", feedID)
}

// GetEpisode returns one episode by ID.
func (c *Client) GetEpisode(ctx context.Context, episodeID int) (*Episode, error) {
	body, err := c.do(ctx, "/api/episodes/"+strconv.Itoa(episodeID), c.metadata)
	if err != nil {
		return nil, err
	}
	var ep Episode
	if err := json.Unmarshal(body, &ep); err != nil {
		return nil, fmt.Errorf("parse episode: %w", err)
	}
	return &ep, nil
}

// ListCompletedOptions selects a slice of the completed episodes.
type ListCompletedOptions struct {
	// Since is an opaque cast2md updated_at value; only episodes updated
	// strictly after it are returned. Pass back a value from a previous
	// response, never one derived from vimmary's clock.
	Since  string
	FeedID int
	Order  string
	Limit  int
}

// ListCompleted returns completed episodes in the requested order.
func (c *Client) ListCompleted(ctx context.Context, opts ListCompletedOptions) ([]Episode, error) {
	q := url.Values{}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.FeedID > 0 {
		q.Set("feed_id", strconv.Itoa(opts.FeedID))
	}
	if opts.Order != "" {
		q.Set("order", opts.Order)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}

	path := "/api/episodes/status/" + StatusCompleted
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	body, err := c.do(ctx, path, c.metadata)
	if err != nil {
		return nil, err
	}
	var result struct {
		Episodes []Episode `json:"episodes"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse episode list: %w", err)
	}
	return result.Episodes, nil
}

// GetTranscript returns the plain-text transcript without timestamps.
func (c *Client) GetTranscript(ctx context.Context, episodeID int) (string, error) {
	body, err := c.do(ctx,
		fmt.Sprintf("/api/episodes/%d/transcript?format=txt", episodeID),
		c.transcript)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", fmt.Errorf("cast2md returned an empty transcript for episode %d", episodeID)
	}
	return text, nil
}

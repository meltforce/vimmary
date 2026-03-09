package karakeep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client interacts with the Karakeep REST API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient creates a Karakeep API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Bookmark represents a Karakeep bookmark.
type Bookmark struct {
	ID      string `json:"id"`
	Content struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"content"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

// GetBookmark retrieves a bookmark by ID.
func (c *Client) GetBookmark(ctx context.Context, id string) (*Bookmark, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/bookmarks/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get bookmark: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get bookmark %s: status %d: %s", id, resp.StatusCode, string(body))
	}

	var bm Bookmark
	if err := json.Unmarshal(body, &bm); err != nil {
		return nil, fmt.Errorf("parse bookmark: %w", err)
	}
	return &bm, nil
}

// UpdateNote sets the note on a bookmark.
func (c *Client) UpdateNote(ctx context.Context, bookmarkID, note string) error {
	payload := map[string]string{"note": note}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal note: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", c.baseURL+"/api/v1/bookmarks/"+bookmarkID, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update note: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// AddTag adds a tag to a bookmark.
func (c *Client) AddTag(ctx context.Context, bookmarkID, tagName string) error {
	payload := map[string]any{
		"tags": []map[string]string{
			{"tagName": tagName},
		},
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal tag: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/api/v1/bookmarks/"+bookmarkID+"/tags", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("add tag: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add tag: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

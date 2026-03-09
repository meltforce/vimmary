package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	baseURL    = "https://api.mistral.ai/v1"
	embedModel = "mistral-embed"
)

// Client provides Mistral API access for embeddings.
type Client struct {
	apiKey string
	http   *http.Client
}

// NewClient creates a Mistral API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed returns a 1024-dimensional embedding for the given text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body := map[string]any{
		"model": embedModel,
		"input": []string{text},
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := c.post(ctx, "/embeddings", body, &result); err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embed: empty response")
	}
	return result.Data[0].Embedding, nil
}

func (c *Client) post(ctx context.Context, path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	deadline := time.Now().Add(10 * time.Minute)
	backoff := 500 * time.Millisecond
	const maxBackoff = 30 * time.Second

	for {
		req, err := http.NewRequestWithContext(ctx, "POST", baseURL+path, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			if time.Now().After(deadline) {
				return fmt.Errorf("request failed (retries exhausted): %w", err)
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			return nil
		}

		if !isRetryable(resp.StatusCode) || time.Now().After(deadline) {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		}

		wait := backoff
		if resp.StatusCode == http.StatusTooManyRequests {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
		}
		if remaining := time.Until(deadline); wait > remaining {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		}

		if err := sleepCtx(ctx, wait); err != nil {
			return fmt.Errorf("API error %d (context cancelled): %s", resp.StatusCode, string(respBody))
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

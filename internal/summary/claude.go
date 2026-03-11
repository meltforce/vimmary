package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClaudeSummarizer uses the Anthropic API for summary generation.
type ClaudeSummarizer struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewClaudeSummarizer creates a Claude-based summarizer.
// If baseURL is empty, defaults to the Anthropic API.
// An optional httpClient can be provided (e.g. for Tailscale-internal requests); if nil, a default client is used.
func NewClaudeSummarizer(apiKey, baseURL string, httpClient *http.Client) *ClaudeSummarizer {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	return &ClaudeSummarizer{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    httpClient,
	}
}

func (c *ClaudeSummarizer) Summarize(ctx context.Context, title, transcript, level, language, customPrompt, model string) (*Summary, error) {
	template := promptForLevel(level)
	if customPrompt != "" {
		template = customPrompt
	}
	prompt := BuildPrompt(template, title, language, truncateTranscript(transcript))

	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude API request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	sum, err := parseSummaryJSON(result.Content[0].Text)
	if err != nil {
		return nil, err
	}
	sum.Usage = Usage{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}
	return sum, nil
}

// truncateTranscript limits transcript length to avoid token limits.
// Claude supports ~200k tokens, so we allow generous transcripts.
func truncateTranscript(transcript string) string {
	const maxChars = 500_000
	if len(transcript) > maxChars {
		return transcript[:maxChars] + "\n\n[Transcript truncated]"
	}
	return transcript
}

func parseSummaryJSON(text string) (*Summary, error) {
	// Try to extract JSON from the response (may be wrapped in markdown code blocks)
	cleaned := text
	if idx := findJSONStart(cleaned); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := findJSONEnd(cleaned); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	var s Summary
	if err := json.Unmarshal([]byte(cleaned), &s); err != nil {
		// If JSON parsing fails, use the raw text as summary
		return &Summary{
			Text:        text,
			Topics:      []string{},
			KeyPoints:   []string{},
			ActionItems: []string{},
		}, nil
	}

	if s.Topics == nil {
		s.Topics = []string{}
	}
	if s.KeyPoints == nil {
		s.KeyPoints = []string{}
	}
	if s.ActionItems == nil {
		s.ActionItems = []string{}
	}

	return &s, nil
}

func findJSONStart(s string) int {
	for i, c := range s {
		if c == '{' {
			return i
		}
	}
	return -1
}

func findJSONEnd(s string) int {
	depth := 0
	for i, c := range s {
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

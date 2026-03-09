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

const claudeAPIURL = "https://api.anthropic.com/v1/messages"

// ClaudeSummarizer uses the Anthropic API for summary generation.
type ClaudeSummarizer struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewClaudeSummarizer creates a Claude-based summarizer.
func NewClaudeSummarizer(apiKey, model string) *ClaudeSummarizer {
	if model == "" {
		model = "claude-sonnet-4-5-20250514"
	}
	return &ClaudeSummarizer{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *ClaudeSummarizer) Summarize(ctx context.Context, title, transcript, level, language string) (*Summary, error) {
	prompt := fmt.Sprintf(promptForLevel(level), title, languageInstruction(language), truncateTranscript(transcript))

	body := map[string]any{
		"model":      c.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", claudeAPIURL, bytes.NewReader(jsonBody))
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
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	return parseSummaryJSON(result.Content[0].Text)
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

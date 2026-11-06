package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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

func (c *ClaudeSummarizer) Summarize(ctx context.Context, req Request) (*Summary, error) {
	template := promptFor(req.Source, req.Level)
	if req.CustomPrompt != "" {
		template = req.CustomPrompt
	}
	prompt := WithTopicReuseHint(
		BuildPrompt(template, req.Title, req.Language, truncateTranscript(req.Transcript)),
		req.ExistingTopics)

	text, usage, err := c.complete(ctx, req.Model, prompt, claudeMaxTokens(req.Level))
	if err != nil {
		return nil, err
	}

	sum, err := parseSummaryJSON(text)
	if err != nil {
		return nil, err
	}
	sum.Usage = *usage
	return sum, nil
}

// Complete sends one free-form prompt and returns the raw text answer. The
// budget is raised by the thinking headroom, because the caller sized it for
// the answer alone.
func (c *ClaudeSummarizer) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	text, _, err := c.complete(ctx, "", prompt, maxTokens+claudeThinkingHeadroom)
	return text, err
}

// DefaultClaudeModel is used when neither the request nor the settings name a
// model.
const DefaultClaudeModel = "claude-sonnet-5-5"

// fallbackModels are the models that accept the server-side refusal fallback.
// Sending `fallbacks` to any other model is a 400, so the field is added only
// for these.
var fallbackModels = map[string]bool{
	"claude-sonnet-5-5": true,
	"claude-opus-5-5":   true,
	"claude-opus-5":     true,
	"claude-fable-5-1":  true,
}

// ErrRefused reports a response the API's safety classifiers declined.
var ErrRefused = errors.New("claude declined the request")

func (c *ClaudeSummarizer) complete(ctx context.Context, model, prompt string, maxTokens int) (string, *Usage, error) {
	if model == "" {
		model = DefaultClaudeModel
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	// Security talks are part of the library, and the cyber classifier can
	// decline one. With the fallback the API re-serves a declined request on
	// a model that accepts it, inside the same call, instead of the summary
	// failing.
	if fallbackModels[model] {
		body["fallbacks"] = "default"
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if fallbackModels[model] {
		httpReq.Header.Set("anthropic-beta", "server-side-fallback-2026-07-01")
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", nil, fmt.Errorf("claude API request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("claude API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		StopReason  string `json:"stop_reason"`
		StopDetails *struct {
			Category    string `json:"category"`
			Explanation string `json:"explanation"`
		} `json:"stop_details"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", nil, fmt.Errorf("parse response: %w", err)
	}

	switch result.StopReason {
	case "refusal":
		if result.StopDetails != nil {
			return "", nil, fmt.Errorf("%w (category %q): %s", ErrRefused,
				result.StopDetails.Category, result.StopDetails.Explanation)
		}
		return "", nil, ErrRefused
	case "max_tokens":
		return "", nil, fmt.Errorf("%w: %d tokens", errTruncatedJSON, maxTokens)
	}

	// The answer is the text blocks only. Models that think — every Claude 5.5
	// model does by default — put a thinking block first, whose text is empty
	// unless a summary of the reasoning was requested. Reading content[0]
	// stored that empty string as a completed summary.
	var text strings.Builder
	for _, b := range result.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	if text.Len() == 0 {
		return "", nil, fmt.Errorf("empty response from Claude (stop_reason %q)", result.StopReason)
	}

	return text.String(), &Usage{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}, nil
}

// maxOutputTokens is the output budget per detail level. A deep summary of a
// multi-hour episode asks for segment headers, quotes, key points and a
// reference list, which does not fit in the 4096 that medium needs.
func maxOutputTokens(level string) int {
	if level == "deep" {
		return 16000
	}
	return 4096
}

// claudeThinkingHeadroom is added to the answer budget for Claude. Thinking
// tokens count against max_tokens, and the 5.5 models think adaptively by
// default: on 2026-11-06 claude-sonnet-5-5 hit 4096 on a medium summary of a
// 62-minute German podcast and finished with 8192. Only generated tokens are
// billed, so the headroom costs nothing when it is not used.
const claudeThinkingHeadroom = 12000

func claudeMaxTokens(level string) int {
	return maxOutputTokens(level) + claudeThinkingHeadroom
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

// errTruncatedJSON reports a response whose JSON object never closed, which is
// what hitting the output token limit looks like.
var errTruncatedJSON = errors.New("model response ended inside the JSON object (output token limit reached)")

func parseSummaryJSON(text string) (*Summary, error) {
	// Try to extract JSON from the response (may be wrapped in markdown code blocks)
	cleaned := text
	start := findJSONStart(cleaned)
	if start >= 0 {
		cleaned = cleaned[start:]
	}
	end := findJSONEnd(cleaned)
	if end >= 0 {
		cleaned = cleaned[:end+1]
	} else if start >= 0 {
		// The object opened but never closed. Falling through to the raw-text
		// path here would store a cut-off summary as if it were complete, so
		// the caller has to see this as a failure.
		return nil, errTruncatedJSON
	}

	var s Summary
	if err := json.Unmarshal([]byte(cleaned), &s); err != nil {
		// No JSON at all — treat the whole response as the summary text.
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

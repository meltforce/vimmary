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

const mistralAPIURL = "https://api.mistral.ai/v1/chat/completions"

// MistralSummarizer uses the Mistral API for summary generation.
type MistralSummarizer struct {
	apiKey string
	http   *http.Client
}

// NewMistralSummarizer creates a Mistral-based summarizer.
func NewMistralSummarizer(apiKey string) *MistralSummarizer {
	return &MistralSummarizer{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (m *MistralSummarizer) Summarize(ctx context.Context, req Request) (*Summary, error) {
	template := promptFor(req.Source, req.Level)
	if req.CustomPrompt != "" {
		template = req.CustomPrompt
	}
	prompt := WithTopicReuseHint(
		BuildPrompt(template, req.Title, req.Language, truncateTranscript(req.Transcript)),
		req.ExistingTopics)

	text, usage, err := m.complete(ctx, req.Model, prompt, maxOutputTokens(req.Level), true)
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

// Complete sends one free-form prompt and returns the raw text answer.
func (m *MistralSummarizer) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	text, _, err := m.complete(ctx, "", prompt, maxTokens, false)
	return text, err
}

func (m *MistralSummarizer) complete(ctx context.Context, model, prompt string, maxTokens int, jsonMode bool) (string, *Usage, error) {
	if model == "" {
		model = "mistral-large-latest"
	}

	// max_tokens is set explicitly rather than left to the provider default, so
	// both providers cut off at the same point or not at all.
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
		"max_tokens":  maxTokens,
	}
	if jsonMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", mistralAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.http.Do(httpReq)
	if err != nil {
		return "", nil, fmt.Errorf("mistral API request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("mistral API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", nil, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", nil, fmt.Errorf("empty response from Mistral")
	}

	return result.Choices[0].Message.Content, &Usage{
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}, nil
}

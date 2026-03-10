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

func (m *MistralSummarizer) Summarize(ctx context.Context, title, transcript, level, language, customPrompt, model string) (*Summary, error) {
	template := promptForLevel(level)
	if customPrompt != "" {
		template = customPrompt
	}
	prompt := BuildPrompt(template, title, language, truncateTranscript(transcript))

	if model == "" {
		model = "mistral-large-latest"
	}

	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mistralAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mistral API request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mistral API error %d: %s", resp.StatusCode, string(respBody))
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
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response from Mistral")
	}

	sum, err := parseSummaryJSON(result.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	sum.Usage = Usage{
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}
	return sum, nil
}

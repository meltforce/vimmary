package summary

import "context"

// Usage tracks token consumption for a summarization call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Summary holds the generated summary and extracted metadata.
type Summary struct {
	Text        string   `json:"text"`
	Topics      []string `json:"topics"`
	KeyPoints   []string `json:"key_points"`
	ActionItems []string `json:"action_items"`
	Usage       Usage    `json:"usage"`
}

// Summarizer generates summaries from video transcripts.
// The model parameter selects the model; empty string means use provider default.
type Summarizer interface {
	Summarize(ctx context.Context, title, transcript, level, language, customPrompt, model string) (*Summary, error)
}

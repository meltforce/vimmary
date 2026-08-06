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

// Request is one summarization call.
type Request struct {
	// Source is "youtube" or "podcast" and selects the built-in prompt.
	Source string
	Title  string
	// Transcript is the full text; the summarizer truncates it if needed.
	Transcript string
	// Level is "medium" or "deep" and also drives the output token budget.
	Level    string
	Language string
	// CustomPrompt overrides the built-in prompt when non-empty.
	CustomPrompt string
	// Model is empty to use the provider default.
	Model string
}

// Summarizer generates summaries from transcripts.
type Summarizer interface {
	Summarize(ctx context.Context, req Request) (*Summary, error)
}

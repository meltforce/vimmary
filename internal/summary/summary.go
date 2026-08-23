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
	// ExistingTopics are the library's current topic tags; the prompt asks
	// the model to reuse them where they fit, so the tag set converges
	// instead of growing a new near-synonym per video.
	ExistingTopics []string
}

// Summarizer generates summaries from transcripts.
type Summarizer interface {
	Summarize(ctx context.Context, req Request) (*Summary, error)
	// Complete sends one free-form prompt and returns the raw text answer.
	// It exists for maintenance calls (topic consolidation) that need the
	// configured provider without the summary JSON contract.
	Complete(ctx context.Context, prompt string, maxTokens int) (string, error)
}

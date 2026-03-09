package summary

import "context"

// Summary holds the generated summary and extracted metadata.
type Summary struct {
	Text        string   `json:"text"`
	Topics      []string `json:"topics"`
	KeyPoints   []string `json:"key_points"`
	ActionItems []string `json:"action_items"`
}

// Summarizer generates summaries from video transcripts.
type Summarizer interface {
	Summarize(ctx context.Context, title, transcript, level string) (*Summary, error)
}

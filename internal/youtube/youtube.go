package youtube

import (
	yt_transcript "github.com/horiagug/youtube-transcript-api-go/pkg/yt_transcript"
)

// Transcript holds the extracted transcript and its source.
type Transcript struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Source   string `json:"source"` // "manual", "auto"
}

// Metadata holds video metadata from YouTube.
type Metadata struct {
	Title           string `json:"title"`
	Channel         string `json:"channel"`
	DurationSeconds int    `json:"duration_seconds"`
	Language        string `json:"language"`
}

// Client wraps YouTube transcript and metadata extraction.
type Client struct {
	subLangs         []string
	transcriptClient *yt_transcript.YtTranscriptClient
}

// NewClient creates a YouTube client.
func NewClient(subLangs []string) *Client {
	if len(subLangs) == 0 {
		subLangs = []string{"en", "de"}
	}
	return &Client{
		subLangs:         subLangs,
		transcriptClient: yt_transcript.NewClient(),
	}
}

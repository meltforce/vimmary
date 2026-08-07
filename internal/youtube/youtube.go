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

// ThumbnailURL is the poster image for a video ID.
//
// It is derived rather than fetched: InnerTube's metadata response carries no
// thumbnail, and the path is stable for every video YouTube serves. `hqdefault`
// is the variant that always exists — `maxresdefault` and `sddefault` 404 on
// plenty of videos, and a broken image is worse than a soft one. It is 480x360
// with letterbox bars on a 16:9 upload, and those bars are exactly the 45px the
// feed's `object-fit: cover` crops off a 16:9 frame, so the displayed image is
// the 480x270 picture itself.
//
// Returns "" for an empty ID, so a podcast row never gets a YouTube URL.
func ThumbnailURL(videoID string) string {
	if videoID == "" {
		return ""
	}
	return "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"
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

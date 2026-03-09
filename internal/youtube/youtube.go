package youtube

// Transcript holds the extracted transcript and its source.
type Transcript struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Source   string `json:"source"` // "manual", "auto", "whisper"
}

// Metadata holds video metadata from YouTube.
type Metadata struct {
	Title           string `json:"title"`
	Channel         string `json:"channel"`
	DurationSeconds int    `json:"duration_seconds"`
	Language        string `json:"language"`
}

// Client wraps yt-dlp for transcript and metadata extraction.
type Client struct {
	ytdlpPath string
	subLangs  []string
}

// NewClient creates a YouTube client.
func NewClient(ytdlpPath string, subLangs []string) *Client {
	if ytdlpPath == "" {
		ytdlpPath = "yt-dlp"
	}
	if len(subLangs) == 0 {
		subLangs = []string{"en", "de"}
	}
	return &Client{ytdlpPath: ytdlpPath, subLangs: subLangs}
}

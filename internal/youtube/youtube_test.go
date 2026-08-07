package youtube

import (
	"testing"
)

func TestNewClient_DefaultLanguages(t *testing.T) {
	c := NewClient(nil)
	if len(c.subLangs) != 2 || c.subLangs[0] != "en" || c.subLangs[1] != "de" {
		t.Errorf("default subLangs = %v, want [en de]", c.subLangs)
	}
}

func TestNewClient_CustomLanguages(t *testing.T) {
	c := NewClient([]string{"fr", "es"})
	if len(c.subLangs) != 2 || c.subLangs[0] != "fr" || c.subLangs[1] != "es" {
		t.Errorf("custom subLangs = %v, want [fr es]", c.subLangs)
	}
}

func TestParsePlayerResponse(t *testing.T) {
	// Test the regex used for metadata extraction
	html := []byte(`some stuff var ytInitialPlayerResponse = {"videoDetails":{"title":"Test Video","author":"Test Channel","lengthSeconds":"360"}}; more stuff`)

	matches := playerResponseRe.FindSubmatch(html)
	if matches == nil {
		t.Fatal("playerResponseRe did not match")
	}

	want := `{"videoDetails":{"title":"Test Video","author":"Test Channel","lengthSeconds":"360"}}`
	if string(matches[1]) != want {
		t.Errorf("regex captured:\n%s\nwant:\n%s", string(matches[1]), want)
	}
}

func TestParsePlayerResponse_NoMatch(t *testing.T) {
	html := []byte(`<html><body>no player response here</body></html>`)

	matches := playerResponseRe.FindSubmatch(html)
	if matches != nil {
		t.Error("expected no match for page without player response")
	}
}

func TestThumbnailURL(t *testing.T) {
	tests := []struct {
		name    string
		videoID string
		want    string
	}{
		{
			name:    "video ID",
			videoID: "dQw4w9WgXcQ",
			want:    "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg",
		},
		{
			// A podcast row carries no youtube_id, and must not be handed a
			// YouTube URL built from an empty string.
			name:    "empty ID",
			videoID: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ThumbnailURL(tt.videoID); got != tt.want {
				t.Errorf("ThumbnailURL(%q) = %q, want %q", tt.videoID, got, tt.want)
			}
		})
	}
}

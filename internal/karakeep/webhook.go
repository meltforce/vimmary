package karakeep

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// WebhookPayload is the payload sent by Karakeep webhooks.
type WebhookPayload struct {
	BookmarkID string `json:"bookmarkId"`
	URL        string `json:"url"`
	Type       string `json:"type"`
	Operation  string `json:"operation"`
}

// VideoProcessor is called when a YouTube video bookmark is received.
type VideoProcessor interface {
	ProcessVideoAsync(userID int, youtubeID, bookmarkID string)
}

var youtubeIDRegex = regexp.MustCompile(`(?:youtube\.com/watch\?v=|youtu\.be/)([a-zA-Z0-9_-]{11})`)

// ExtractYouTubeID extracts the YouTube video ID from a URL.
func ExtractYouTubeID(url string) string {
	matches := youtubeIDRegex.FindStringSubmatch(url)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// WebhookHandler returns an HTTP handler for Karakeep webhooks.
func WebhookHandler(processor VideoProcessor, webhookToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify webhook token if configured
		if webhookToken != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != webhookToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		// Only process new bookmarks
		if payload.Operation != "created" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Extract YouTube ID
		youtubeID := ExtractYouTubeID(payload.URL)
		if youtubeID == "" {
			// Not a YouTube URL, ignore
			w.WriteHeader(http.StatusOK)
			return
		}

		// Process asynchronously (user_id=1 for webhook-triggered processing)
		processor.ProcessVideoAsync(1, youtubeID, payload.BookmarkID)

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}
}

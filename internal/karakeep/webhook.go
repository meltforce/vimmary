package karakeep

import (
	"context"
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

// WebhookProcessor handles webhook events from Karakeep.
type WebhookProcessor interface {
	ProcessVideoAsync(userID int, youtubeID, bookmarkID string)
	DeleteByBookmarkID(ctx context.Context, userID int, bookmarkID string) error
}

// TokenResolver resolves a Bearer token to a user ID.
type TokenResolver func(ctx context.Context, token string) (int, error)

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
// The tokenResolver maps a Bearer token to a user ID.
func WebhookHandler(processor WebhookProcessor, tokenResolver TokenResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")

		userID, err := tokenResolver(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		switch payload.Operation {
		case "created":
			youtubeID := ExtractYouTubeID(payload.URL)
			if youtubeID == "" {
				w.WriteHeader(http.StatusOK)
				return
			}
			processor.ProcessVideoAsync(userID, youtubeID, payload.BookmarkID)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"accepted"}`))

		case "deleted":
			if payload.BookmarkID == "" {
				w.WriteHeader(http.StatusOK)
				return
			}
			// Best-effort delete — ignore errors if bookmark wasn't tracked
			_ = processor.DeleteByBookmarkID(r.Context(), userID, payload.BookmarkID)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"deleted"}`))

		default:
			w.WriteHeader(http.StatusOK)
		}
	}
}

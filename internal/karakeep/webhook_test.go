package karakeep

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractYouTubeID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"standard watch URL", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"short URL", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"with extra params", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=120", "dQw4w9WgXcQ"},
		{"with playlist", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "dQw4w9WgXcQ"},
		{"no protocol", "youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"non-youtube URL", "https://example.com/watch?v=dQw4w9WgXcQ", ""},
		{"empty string", "", ""},
		{"random text", "hello world", ""},
		{"ID with hyphens/underscores", "https://youtu.be/abc-_def123", "abc-_def123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractYouTubeID(tt.url)
			if got != tt.want {
				t.Errorf("ExtractYouTubeID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// mockProcessor records calls to ProcessVideoAsync and DeleteByBookmarkID.
type mockProcessor struct {
	processedCalls []processCall
	deletedCalls   []deleteCall
}

type processCall struct {
	userID     int
	youtubeID  string
	bookmarkID string
}

type deleteCall struct {
	userID     int
	bookmarkID string
}

func (m *mockProcessor) ProcessVideoAsync(userID int, youtubeID, bookmarkID string) {
	m.processedCalls = append(m.processedCalls, processCall{userID, youtubeID, bookmarkID})
}

func (m *mockProcessor) DeleteByBookmarkID(_ context.Context, userID int, bookmarkID string) error {
	m.deletedCalls = append(m.deletedCalls, deleteCall{userID, bookmarkID})
	return nil
}

func testTokenResolver(token string) TokenResolver {
	return func(_ context.Context, t string) (int, error) {
		if t == token {
			return 1, nil
		}
		return 0, http.ErrNoCookie // any error
	}
}

func TestWebhookHandler_CreatedWithYouTubeURL(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	payload := WebhookPayload{
		BookmarkID: "bm-123",
		URL:        "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Type:       "link",
		Operation:  "created",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if len(proc.processedCalls) != 1 {
		t.Fatalf("expected 1 process call, got %d", len(proc.processedCalls))
	}
	if proc.processedCalls[0].youtubeID != "dQw4w9WgXcQ" {
		t.Errorf("youtubeID = %q, want %q", proc.processedCalls[0].youtubeID, "dQw4w9WgXcQ")
	}
	if proc.processedCalls[0].bookmarkID != "bm-123" {
		t.Errorf("bookmarkID = %q, want %q", proc.processedCalls[0].bookmarkID, "bm-123")
	}
}

func TestWebhookHandler_CreatedNonYouTubeURL(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	payload := WebhookPayload{
		BookmarkID: "bm-456",
		URL:        "https://example.com/article",
		Operation:  "created",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(proc.processedCalls) != 0 {
		t.Error("expected no process calls for non-YouTube URL")
	}
}

func TestWebhookHandler_Deleted(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	payload := WebhookPayload{
		BookmarkID: "bm-789",
		Operation:  "deleted",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(proc.deletedCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(proc.deletedCalls))
	}
	if proc.deletedCalls[0].bookmarkID != "bm-789" {
		t.Errorf("bookmarkID = %q, want %q", proc.deletedCalls[0].bookmarkID, "bm-789")
	}
}

func TestWebhookHandler_DeletedEmptyBookmarkID(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	payload := WebhookPayload{
		BookmarkID: "",
		Operation:  "deleted",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(proc.deletedCalls) != 0 {
		t.Error("expected no delete calls for empty bookmark ID")
	}
}

func TestWebhookHandler_MissingAuth(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	req := httptest.NewRequest("POST", "/webhook", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHandler_InvalidToken(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	payload := WebhookPayload{Operation: "created", URL: "https://youtube.com/watch?v=abc12345678"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHandler_InvalidPayload(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte("not json")))
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWebhookHandler_UnknownOperation(t *testing.T) {
	proc := &mockProcessor{}
	handler := WebhookHandler(proc, testTokenResolver("valid-token"))

	payload := WebhookPayload{Operation: "updated"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(proc.processedCalls) != 0 || len(proc.deletedCalls) != 0 {
		t.Error("expected no calls for unknown operation")
	}
}

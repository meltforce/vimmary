package storage_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
)

// completedRow inserts a row and brings it to `completed` the way the ingest
// path does: transcript first, then summary with an embedding.
func completedRow(t *testing.T, store *storage.DB, userID int, ytID string, embedding []float32) *storage.Video {
	t.Helper()
	ctx := context.Background()

	v := &storage.Video{
		ID:          uuid.New(),
		UserID:      userID,
		YouTubeID:   ytID,
		DetailLevel: "medium",
		Status:      "processing",
	}
	if err := store.InsertVideo(ctx, v); err != nil {
		t.Fatalf("insert video: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, userID, v.ID) })

	if err := store.UpdateVideoTranscript(ctx, v.ID, "the transcript", "The Title",
		"The Channel", "en", 600, json.RawMessage(`[{"s":0,"d":2,"t":"hello"}]`)); err != nil {
		t.Fatalf("update transcript: %v", err)
	}
	if err := store.UpdateVideoSummary(ctx, v.ID, "the summary", "medium", "claude",
		"claude-opus-5", 100, 200, embedding, json.RawMessage(`{"topics":["go"]}`)); err != nil {
		t.Fatalf("update summary: %v", err)
	}
	return v
}

// secondUser returns a user other than 1, creating it if the test database has
// only the seeded local user.
func secondUser(t *testing.T, store *storage.DB) int {
	t.Helper()
	id, err := store.GetOrCreateUser(context.Background(), "second@example.com", "Second User")
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}
	return id
}

// TestAdoptSharedContent covers the ingest short-circuit: a second user's row
// takes over the first user's finished work instead of re-fetching and
// re-summarizing it, embedding included.
func TestAdoptSharedContent(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	other := secondUser(t, store)

	ytID := "shared_" + uuid.NewString()[:8]
	embedding := make([]float32, 1024)
	embedding[0] = 1

	completedRow(t, store, 1, ytID, embedding)

	second := &storage.Video{
		ID:          uuid.New(),
		UserID:      other,
		YouTubeID:   ytID,
		DetailLevel: "deep",
		Status:      "pending",
	}
	if err := store.InsertVideo(ctx, second); err != nil {
		t.Fatalf("insert second row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, other, second.ID) })

	adopted, err := store.AdoptSharedContent(ctx, second.ID)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if !adopted {
		t.Fatal("adopt reported no source row, want the first user's completed row")
	}

	got, err := store.GetVideo(ctx, other, second.ID)
	if err != nil {
		t.Fatalf("get adopted row: %v", err)
	}
	if got.Transcript != "the transcript" {
		t.Errorf("transcript = %q, want the first row's", got.Transcript)
	}
	if got.Summary != "the summary" {
		t.Errorf("summary = %q, want the first row's", got.Summary)
	}
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.Title != "The Title" {
		t.Errorf("title = %q, want the first row's", got.Title)
	}
	// The embedding has no field on Video, so the search index is what proves
	// the vector came across — this is the regression the SQL-level copy exists
	// to prevent.
	matches, err := store.SearchVideos(ctx, other, embedding, 0.9, 10, storage.SourceYouTube)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var found bool
	for _, m := range matches {
		if m.ID == second.ID {
			found = true
		}
	}
	if !found {
		t.Error("adopted row is not in the second user's search index, embedding was not copied")
	}

	// A row whose content nobody else has finished stays untouched.
	lone := &storage.Video{
		ID:          uuid.New(),
		UserID:      1,
		YouTubeID:   "lone_" + uuid.NewString()[:8],
		DetailLevel: "medium",
		Status:      "pending",
	}
	if err := store.InsertVideo(ctx, lone); err != nil {
		t.Fatalf("insert lone row: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteVideo(ctx, 1, lone.ID) })
	if adopted, err := store.AdoptSharedContent(ctx, lone.ID); err != nil {
		t.Fatalf("adopt lone: %v", err)
	} else if adopted {
		t.Error("adopt reported a source row for content nobody has finished")
	}
}

// TestSharedContentWritesFanOut covers last-write-wins: a regeneration started
// by one user is the version the other user sees.
func TestSharedContentWritesFanOut(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	other := secondUser(t, store)

	ytID := "fanout_" + uuid.NewString()[:8]
	embedding := make([]float32, 1024)
	embedding[0] = 1

	first := completedRow(t, store, 1, ytID, embedding)
	second := completedRow(t, store, other, ytID, embedding)

	// The second user regenerates at a different level.
	if err := store.UpdateVideoSummary(ctx, second.ID, "regenerated", "deep", "mistral",
		"mistral-large", 10, 20, embedding, json.RawMessage(`{"topics":["rust"]}`)); err != nil {
		t.Fatalf("regenerate: %v", err)
	}

	got, err := store.GetVideo(ctx, 1, first.ID)
	if err != nil {
		t.Fatalf("get first row: %v", err)
	}
	if got.Summary != "regenerated" {
		t.Errorf("first user's summary = %q, want the regenerated one", got.Summary)
	}
	if got.DetailLevel != "deep" {
		t.Errorf("first user's detail level = %q, want deep", got.DetailLevel)
	}

	// A transcript rewrite reaches the other row the same way.
	if err := store.UpdateVideoTranscript(ctx, first.ID, "corrected", "New Title",
		"The Channel", "de", 700, nil); err != nil {
		t.Fatalf("update transcript: %v", err)
	}
	got, err = store.GetVideo(ctx, other, second.ID)
	if err != nil {
		t.Fatalf("get second row: %v", err)
	}
	if got.Transcript != "corrected" || got.Title != "New Title" {
		t.Errorf("second row = (%q, %q), want the corrected transcript and title",
			got.Transcript, got.Title)
	}

	// A failure belongs to one attempt and must not fan out.
	if err := store.UpdateVideoStatus(ctx, second.ID, "failed", "boom"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, err = store.GetVideo(ctx, 1, first.ID)
	if err != nil {
		t.Fatalf("get first row: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("first user's status = %q, want completed — a failure fanned out", got.Status)
	}
}

// TestSummaryPromptIsShared covers the settings half: one user's prompt is
// every user's prompt, including a user created after the change.
func TestSummaryPromptIsShared(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	other := secondUser(t, store)

	t.Cleanup(func() {
		_ = store.SetUserPrompt(ctx, 1, storage.SourceYouTube, "deep", "")
	})

	if err := store.SetUserPrompt(ctx, 1, storage.SourceYouTube, "deep", "shared prompt"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}

	got, err := store.GetUserPrompt(ctx, other, storage.SourceYouTube, "deep")
	if err != nil {
		t.Fatalf("get prompt as other user: %v", err)
	}
	if got != "shared prompt" {
		t.Errorf("other user's prompt = %q, want the shared one", got)
	}

	prompts, err := store.GetUserPrompts(ctx, other, storage.SourceYouTube)
	if err != nil {
		t.Fatalf("get prompts as other user: %v", err)
	}
	if prompts["deep"] != "shared prompt" {
		t.Errorf("other user's prompt map = %v, want deep to carry the shared prompt", prompts)
	}

	// Resetting clears it for everyone.
	if err := store.SetUserPrompt(ctx, other, storage.SourceYouTube, "deep", ""); err != nil {
		t.Fatalf("reset prompt: %v", err)
	}
	got, err = store.GetUserPrompt(ctx, 1, storage.SourceYouTube, "deep")
	if err != nil {
		t.Fatalf("get prompt after reset: %v", err)
	}
	if got != "" {
		t.Errorf("prompt after reset = %q, want empty", got)
	}
}

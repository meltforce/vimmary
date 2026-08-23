package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
)

func TestVideoSegmentsRoundtrip(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	video := &storage.Video{
		ID:          uuid.New(),
		UserID:      1,
		YouTubeID:   "test_segments_" + uuid.NewString()[:8],
		DetailLevel: "medium",
		Status:      "completed",
	}
	if err := store.InsertVideo(ctx, video); err != nil {
		t.Fatalf("insert video: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteVideo(ctx, video.UserID, video.ID)
	})

	// A fresh row carries NULL, which reads back as nil.
	got, err := store.GetVideoSegments(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("GetVideoSegments on fresh row: %v", err)
	}
	if got != nil {
		t.Errorf("fresh row segments = %q, want nil", got)
	}

	// The negative cache: '[]' is stored and read back as-is, not as nil.
	if err := store.UpdateVideoSegments(ctx, video.ID, json.RawMessage("[]")); err != nil {
		t.Fatalf("UpdateVideoSegments with []: %v", err)
	}
	got, err = store.GetVideoSegments(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("GetVideoSegments after []: %v", err)
	}
	if string(got) != "[]" {
		t.Errorf("segments = %q, want []", got)
	}

	// A real payload round-trips.
	payload := `[{"s": 0, "d": 2.5, "t": "hello"}, {"s": 12.4, "d": 3.1, "t": "world"}]`
	if err := store.UpdateVideoSegments(ctx, video.ID, json.RawMessage(payload)); err != nil {
		t.Fatalf("UpdateVideoSegments with payload: %v", err)
	}
	got, err = store.GetVideoSegments(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("GetVideoSegments after payload: %v", err)
	}
	var segments []storage.TranscriptSegment
	if err := json.Unmarshal(got, &segments); err != nil {
		t.Fatalf("unmarshal stored segments: %v", err)
	}
	if len(segments) != 2 || segments[1].Start != 12.4 || segments[1].Text != "world" {
		t.Errorf("stored segments = %+v, want two entries ending at 12.4/world", segments)
	}

	// Another user's read is ErrNotFound, not an empty answer.
	if _, err := store.GetVideoSegments(ctx, video.UserID+1, video.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("foreign-user read err = %v, want ErrNotFound", err)
	}

	// UpdateVideoTranscript with nil segments must not clear a stored payload…
	// it does clear it — the parameter is authoritative. Assert that contract
	// so a change to it is a conscious one: the InnerTube path always passes
	// the fresh segments, and the podcast/Voxtral paths pass nil, resetting
	// the column to NULL alongside their timing-free transcript.
	if err := store.UpdateVideoTranscript(ctx, video.ID, "text", "t", "c", "en", 1, nil); err != nil {
		t.Fatalf("UpdateVideoTranscript: %v", err)
	}
	got, err = store.GetVideoSegments(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("GetVideoSegments after transcript update: %v", err)
	}
	if got != nil {
		t.Errorf("segments after nil transcript update = %q, want nil", got)
	}
}

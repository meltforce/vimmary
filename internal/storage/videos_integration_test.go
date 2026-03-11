package storage_test

import (
	"context"
	"io/fs"
	"os"
	"testing"

	"github.com/google/uuid"
	vimmary "github.com/meltforce/vimmary"
	"github.com/meltforce/meltkit/pkg/db"
	"github.com/meltforce/vimmary/internal/storage"
)

const defaultDSN = "postgres://vimmary:vimmary@localhost:5434/vimmary?sslmode=disable"

func setupTestDB(t *testing.T) *storage.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
	}

	ctx := context.Background()

	// Run migrations
	migrationsFS, err := fs.Sub(vimmary.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if err := db.RunMigrations(dsn, migrationsFS); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	d, err := db.New(ctx, dsn, db.WithPgvector())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	return storage.NewDB(d)
}

func TestUpdateVideoMetadata(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Insert a minimal video (simulates what ProcessVideo does before transcript fetch)
	video := &storage.Video{
		ID:          uuid.New(),
		UserID:      1,
		YouTubeID:   "test_metadata_" + uuid.NewString()[:8],
		DetailLevel: "medium",
		Status:      "processing",
	}
	if err := store.InsertVideo(ctx, video); err != nil {
		t.Fatalf("insert video: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteVideo(ctx, video.UserID, video.ID)
	})

	// Verify initial state: no title/channel
	got, err := store.GetVideo(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("get video: %v", err)
	}
	if got.Title != "" || got.Channel != "" {
		t.Fatalf("expected empty title/channel, got %q / %q", got.Title, got.Channel)
	}

	// Save metadata early (the new method)
	if err := store.UpdateVideoMetadata(ctx, video.ID, "Test Title", "Test Channel", "en", 300); err != nil {
		t.Fatalf("UpdateVideoMetadata: %v", err)
	}

	// Verify metadata is persisted
	got, err = store.GetVideo(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("get video after metadata update: %v", err)
	}
	if got.Title != "Test Title" {
		t.Errorf("title = %q, want %q", got.Title, "Test Title")
	}
	if got.Channel != "Test Channel" {
		t.Errorf("channel = %q, want %q", got.Channel, "Test Channel")
	}
	if got.Language != "en" {
		t.Errorf("language = %q, want %q", got.Language, "en")
	}
	if got.DurationSeconds != 300 {
		t.Errorf("duration = %d, want %d", got.DurationSeconds, 300)
	}
	// Status should be unchanged
	if got.Status != "processing" {
		t.Errorf("status = %q, want %q", got.Status, "processing")
	}
	// Transcript should still be empty
	if got.Transcript != "" {
		t.Errorf("transcript should be empty, got %q", got.Transcript)
	}
}

func TestUpdateVideoMetadata_ThenTranscript(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	video := &storage.Video{
		ID:          uuid.New(),
		UserID:      1,
		YouTubeID:   "test_meta_tx_" + uuid.NewString()[:8],
		DetailLevel: "medium",
		Status:      "processing",
	}
	if err := store.InsertVideo(ctx, video); err != nil {
		t.Fatalf("insert video: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteVideo(ctx, video.UserID, video.ID)
	})

	// Step 1: Save metadata early
	if err := store.UpdateVideoMetadata(ctx, video.ID, "Early Title", "Early Channel", "en", 120); err != nil {
		t.Fatalf("UpdateVideoMetadata: %v", err)
	}

	// Step 2: Save transcript + metadata (as the existing flow does)
	if err := store.UpdateVideoTranscript(ctx, video.ID, "Hello world transcript", "Early Title", "Early Channel", "en", 120); err != nil {
		t.Fatalf("UpdateVideoTranscript: %v", err)
	}

	// Verify both metadata and transcript are present
	got, err := store.GetVideo(ctx, video.UserID, video.ID)
	if err != nil {
		t.Fatalf("get video: %v", err)
	}
	if got.Title != "Early Title" {
		t.Errorf("title = %q, want %q", got.Title, "Early Title")
	}
	if got.Transcript != "Hello world transcript" {
		t.Errorf("transcript = %q, want %q", got.Transcript, "Hello world transcript")
	}
}

package storage_test

import (
	"context"
	"io/fs"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/meltforce/meltkit/pkg/db"
	vimmary "github.com/meltforce/vimmary"
	"github.com/meltforce/vimmary/internal/storage"
)

func TestLoadFixtures(t *testing.T) {
	f, err := storage.LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string]int{}
	for _, v := range f.Videos {
		bySource[v.Source]++
		if v.Title == "" || v.Summary == "" || v.Transcript == "" || len(v.Metadata) == 0 {
			t.Errorf("fixture %q lacks title, summary, transcript or metadata", v.Title)
		}
		if v.Source == storage.SourcePodcast && (v.YouTubeID != "" || v.ExternalID == "") {
			t.Errorf("podcast fixture %q: youtube_id %q, external_id %q", v.Title, v.YouTubeID, v.ExternalID)
		}
	}
	if bySource[storage.SourceYouTube] == 0 || bySource[storage.SourcePodcast] == 0 {
		t.Errorf("fixtures by source = %v, want both sources", bySource)
	}
	if len(f.Channels) == 0 || len(f.PodcastSubscriptions) == 0 {
		t.Errorf("fixtures have %d channels and %d podcast subscriptions, want at least one each",
			len(f.Channels), len(f.PodcastSubscriptions))
	}
}

// emptyTestDB creates a database of its own on the test server, because the
// seed acts only on an empty videos table and the shared test database holds
// rows of the other tests.
func emptyTestDB(t *testing.T) *storage.DB {
	t.Helper()
	ctx := context.Background()
	admin := setupTestDB(t)

	name := "seed_" + uuid.NewString()[:8]
	if _, err := admin.Pool.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Pool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})

	u, err := url.Parse(admin.Pool.Config().ConnString())
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()

	migrationsFS, err := fs.Sub(vimmary.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(dsn, migrationsFS); err != nil {
		t.Fatalf("migrate %s: %v", name, err)
	}
	d, err := db.New(ctx, dsn, db.WithPgvector())
	if err != nil {
		t.Fatalf("connect %s: %v", name, err)
	}
	t.Cleanup(d.Close)
	return storage.NewDB(d)
}

func TestSeedFixtures(t *testing.T) {
	store := emptyTestDB(t)
	ctx := context.Background()

	seeded, err := store.SeedFixtures(ctx, 1)
	if err != nil {
		t.Fatalf("SeedFixtures: %v", err)
	}
	if !seeded {
		t.Fatal("SeedFixtures on an empty table reported nothing seeded")
	}

	f, err := storage.LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{}
	for _, v := range f.Videos {
		want[v.Source]++
	}
	for source, n := range want {
		rows, total, err := store.ListRecent(ctx, 1, storage.ListFilters{Source: source}, 100, 0)
		if err != nil {
			t.Fatalf("ListRecent(%s): %v", source, err)
		}
		if total != n {
			t.Errorf("%s rows = %d, want %d", source, total, n)
		}
		for _, v := range rows {
			if v.Status != "completed" || v.Summary == "" {
				t.Errorf("%s row %q: status %q, summary empty %v", source, v.Title, v.Status, v.Summary == "")
			}
		}
	}

	// Podcast rows carry NULL youtube_id, so two of them can coexist under
	// UNIQUE (user_id, youtube_id).
	var nullIDs int
	if err := store.Pool.QueryRow(ctx,
		`SELECT count(*) FROM videos WHERE source = 'podcast' AND youtube_id IS NULL`).Scan(&nullIDs); err != nil {
		t.Fatal(err)
	}
	if nullIDs != want[storage.SourcePodcast] {
		t.Errorf("podcast rows with NULL youtube_id = %d, want %d", nullIDs, want[storage.SourcePodcast])
	}

	channels, err := store.ListChannelSubscriptions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != len(f.Channels) {
		t.Errorf("channels = %d, want %d", len(channels), len(f.Channels))
	}
	items, _, err := store.ListInboxItems(ctx, 1, "new", 0, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(f.Channels[0].Inbox) {
		t.Errorf("inbox items = %d, want %d", len(items), len(f.Channels[0].Inbox))
	}
	unprobed, err := store.ListUnprobedInboxItems(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(unprobed) != 0 {
		t.Errorf("%d seeded inbox items wait for a Shorts probe, want 0", len(unprobed))
	}
	subs, err := store.ListSubscriptions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != len(f.PodcastSubscriptions) || !subs[0].Initialized {
		t.Errorf("podcast subscriptions = %+v, want %d initialized", subs, len(f.PodcastSubscriptions))
	}

	// A second start finds rows and writes nothing.
	again, err := store.SeedFixtures(ctx, 1)
	if err != nil {
		t.Fatalf("second SeedFixtures: %v", err)
	}
	if again {
		t.Error("second SeedFixtures reported seeded, want no-op")
	}
	var count int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM videos`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(f.Videos) {
		t.Errorf("videos after second seed = %d, want %d", count, len(f.Videos))
	}
}

// TestSeedFixtures_NonEmpty: one row of any user keeps the seed out.
func TestSeedFixtures_NonEmpty(t *testing.T) {
	store := emptyTestDB(t)
	ctx := context.Background()

	if err := store.InsertVideo(ctx, &storage.Video{
		ID: uuid.New(), UserID: 1, YouTubeID: "existing001", DetailLevel: "medium", Status: "completed",
	}); err != nil {
		t.Fatal(err)
	}
	seeded, err := store.SeedFixtures(ctx, 1)
	if err != nil {
		t.Fatalf("SeedFixtures: %v", err)
	}
	if seeded {
		t.Error("SeedFixtures seeded a non-empty table")
	}
	var count int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM videos`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("videos = %d, want 1", count)
	}
}

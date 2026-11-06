package storage_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Search compares only vectors of the query's model, and the stale list is
// what the re-embed works off: after a model change every row is stale and
// out of semantic search until it is redone.
func TestEmbeddingModel_SearchFilterAndReembed(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	embedding := make([]float32, 1024)
	embedding[0] = 1
	v := completedRow(t, store, 1, "emb_"+uuid.NewString()[:8], embedding) // embedded as "test-embed"

	hits := func(model string) int {
		t.Helper()
		matches, err := store.SearchVideos(ctx, 1, embedding, model, 0.9, 50, "")
		if err != nil {
			t.Fatalf("SearchVideos(%s): %v", model, err)
		}
		n := 0
		for _, m := range matches {
			if m.ID == v.ID {
				n++
			}
		}
		return n
	}
	isStale := func(model string) bool {
		t.Helper()
		rows, err := store.ListStaleEmbeddings(ctx, model, 1000)
		if err != nil {
			t.Fatalf("ListStaleEmbeddings: %v", err)
		}
		for _, r := range rows {
			if r.ID == v.ID {
				return true
			}
		}
		return false
	}

	if hits("test-embed") != 1 || hits("other-embed") != 0 {
		t.Fatalf("search by model: test-embed %d, other-embed %d; want 1 and 0",
			hits("test-embed"), hits("other-embed"))
	}
	if isStale("test-embed") {
		t.Error("row listed stale for the model that produced it")
	}
	if !isStale("other-embed") {
		t.Fatal("row not listed stale for a different model")
	}

	if err := store.UpdateVideoEmbedding(ctx, v.ID, embedding, "other-embed"); err != nil {
		t.Fatalf("UpdateVideoEmbedding: %v", err)
	}
	if hits("other-embed") != 1 || hits("test-embed") != 0 {
		t.Error("after re-embedding, the row is searchable under the new model only")
	}
	if isStale("other-embed") {
		t.Error("row still stale after re-embedding")
	}
}

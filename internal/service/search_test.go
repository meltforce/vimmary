package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
)

// mockDB implements the DB methods used by the service layer under test.
// We can't use storage.DB directly (requires pgx pool), so we test via
// the exported Service methods with a mockable approach.

// mockEmbedder returns a fixed embedding vector.
type mockEmbedder struct {
	embedding []float32
	err       error
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.embedding, m.err
}

// mockSummarizer returns a fixed summary.
type mockSummarizer struct {
	summary *summary.Summary
	err     error
}

func (m *mockSummarizer) Summarize(_ context.Context, _, _, _, _, _, _ string) (*summary.Summary, error) {
	return m.summary, m.err
}

// Since Service.Search depends on storage.DB methods that require a real
// pgx pool, we test the RRF merging logic in isolation.

func TestRRFMerge(t *testing.T) {
	// Simulate the RRF merge logic from Search()
	const K = 60

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	textResults := []storage.VideoMatch{
		{ID: id1, Title: "Video A"},
		{ID: id2, Title: "Video B"},
	}
	semanticResults := []storage.VideoMatch{
		{ID: id2, Title: "Video B"},
		{ID: id3, Title: "Video C"},
	}

	type entry struct {
		HybridMatch
		hasText     bool
		hasSemantic bool
	}
	merged := make(map[uuid.UUID]*entry)

	for rank, m := range textResults {
		e, ok := merged[m.ID]
		if !ok {
			e = &entry{HybridMatch: HybridMatch{
				ID: m.ID, Title: m.Title,
			}}
			merged[m.ID] = e
		}
		e.Score += 1.0 / float64(K+rank+1)
		e.hasText = true
	}

	for rank, m := range semanticResults {
		e, ok := merged[m.ID]
		if !ok {
			e = &entry{HybridMatch: HybridMatch{
				ID: m.ID, Title: m.Title,
			}}
			merged[m.ID] = e
		}
		e.Score += 1.0 / float64(K+rank+1)
		e.hasSemantic = true
	}

	// id2 should have highest score (appears in both)
	if merged[id2].Score <= merged[id1].Score {
		t.Errorf("id2 (both) score %.6f should be > id1 (text-only) score %.6f", merged[id2].Score, merged[id1].Score)
	}
	if merged[id2].Score <= merged[id3].Score {
		t.Errorf("id2 (both) score %.6f should be > id3 (semantic-only) score %.6f", merged[id2].Score, merged[id3].Score)
	}

	// Check match types
	if !merged[id1].hasText || merged[id1].hasSemantic {
		t.Error("id1 should be text-only")
	}
	if !merged[id2].hasText || !merged[id2].hasSemantic {
		t.Error("id2 should be both")
	}
	if merged[id3].hasText || !merged[id3].hasSemantic {
		t.Error("id3 should be semantic-only")
	}
}

func TestRRFScoring_RankOrder(t *testing.T) {
	const K = 60

	// First-ranked item should score higher than second-ranked
	score1 := 1.0 / float64(K+0+1) // rank 0
	score2 := 1.0 / float64(K+1+1) // rank 1

	if score1 <= score2 {
		t.Errorf("rank 0 score %.6f should be > rank 1 score %.6f", score1, score2)
	}

	// Item appearing in both lists at rank 0 should score ~2x a single rank-0 item
	bothScore := score1 + score1
	if bothScore <= score1 {
		t.Error("double-match should score higher than single match")
	}
}

func TestHybridMatch_MatchType(t *testing.T) {
	tests := []struct {
		hasText     bool
		hasSemantic bool
		want        string
	}{
		{true, true, "both"},
		{true, false, "keyword"},
		{false, true, "semantic"},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("text=%v,semantic=%v", tt.hasText, tt.hasSemantic)
		t.Run(name, func(t *testing.T) {
			var matchType string
			switch {
			case tt.hasText && tt.hasSemantic:
				matchType = "both"
			case tt.hasText:
				matchType = "keyword"
			default:
				matchType = "semantic"
			}
			if matchType != tt.want {
				t.Errorf("matchType = %q, want %q", matchType, tt.want)
			}
		})
	}
}

// TestHybridMatchJSON verifies JSON serialization of search results.
func TestHybridMatchJSON(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	m := HybridMatch{
		ID:        id,
		YouTubeID: "abc123",
		Title:     "Test Video",
		Channel:   "Test Channel",
		Summary:   "A summary",
		Score:     0.5,
		MatchType: "both",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	var decoded HybridMatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.YouTubeID != m.YouTubeID || decoded.Title != m.Title || decoded.MatchType != m.MatchType {
		t.Errorf("round-trip mismatch: got %+v", decoded)
	}
}

// Verify that the service constructor works with nil-safe defaults.
func TestNewService(t *testing.T) {
	embedder := &mockEmbedder{embedding: []float32{0.1, 0.2}}
	summarizer := &mockSummarizer{summary: &summary.Summary{Text: "test"}}

	svc := New(
		nil, // db
		map[string]summary.Summarizer{"claude": summarizer},
		"claude",
		nil, // registry
		nil, // yt client
		"https://karakeep.example.com",
		"https://vimmary.example.com",
		embedder,
		nil, // transcriber
		config.SearchConfig{DefaultThreshold: 0.3, DefaultLimit: 10, ScoreCutoffRatio: 0.5},
		config.SummaryConfig{DefaultLevel: "medium"},
		slog.Default(),
	)

	if svc.karakeepBaseURL != "https://karakeep.example.com" {
		t.Errorf("karakeepBaseURL = %q, want %q", svc.karakeepBaseURL, "https://karakeep.example.com")
	}
}

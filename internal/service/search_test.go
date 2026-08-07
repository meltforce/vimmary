package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/storage"
)

// mockEmbedder returns a fixed embedding vector.
type mockEmbedder struct {
	embedding []float32
	err       error
	calls     int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	m.calls++
	return m.embedding, m.err
}

// There is no mockSummarizer any more. getSummarizer builds the summarizer from
// the API key in app_settings on every call, so a fake can no longer be injected
// through the constructor — summarizer_test.go replaces the settingsSource and
// summarizerFactory seams instead.

// fakeSearch stands in for storage.DB on the search path, the way fakeSettings
// does on the summarizer path. It records the arguments each query received, so
// a test can assert what Search passed down without a database.
type fakeSearch struct {
	text    []storage.VideoMatch
	textErr error
	sem     []storage.VideoMatch
	semErr  error

	gotTextLimit  int
	gotSemLimit   int
	gotThreshold  float64
	gotSource     string
	gotEmbedding  []float32
	textCalls     int
	semanticCalls int
}

func (f *fakeSearch) TextSearchVideos(_ context.Context, _ int, _ string, limit int, source string) ([]storage.VideoMatch, error) {
	f.textCalls++
	f.gotTextLimit = limit
	f.gotSource = source
	return f.text, f.textErr
}

func (f *fakeSearch) SearchVideos(_ context.Context, _ int, embedding []float32, threshold float64, limit int, _ string) ([]storage.VideoMatch, error) {
	f.semanticCalls++
	f.gotSemLimit = limit
	f.gotThreshold = threshold
	f.gotEmbedding = embedding
	return f.sem, f.semErr
}

// newSearchService builds a Service with the search seam and the embedder
// replaced and no database. ScoreCutoffRatio is 0 unless a test sets it, so the
// cutoff does not silently drop results a test expects to see.
func newSearchService(search *fakeSearch, emb *mockEmbedder, cfg config.SearchConfig) *Service {
	return &Service{
		search:    search,
		embedder:  emb,
		searchCfg: cfg,
		log:       slog.New(slog.DiscardHandler),
	}
}

func match(id uuid.UUID, title string) storage.VideoMatch {
	return storage.VideoMatch{ID: id, Title: title, Source: storage.SourceYouTube}
}

func ids(results []HybridMatch) []uuid.UUID {
	out := make([]uuid.UUID, len(results))
	for i, r := range results {
		out[i] = r.ID
	}
	return out
}

// A document found by both searches outranks one found by either alone, and the
// match type reports which searches contributed. This is the RRF fusion running
// inside Search, not a copy of it in the test.
func TestSearch_FusesBothResultSets(t *testing.T) {
	textOnly, both, semOnly := uuid.New(), uuid.New(), uuid.New()

	f := &fakeSearch{
		text: []storage.VideoMatch{match(textOnly, "A"), match(both, "B")},
		sem:  []storage.VideoMatch{match(both, "B"), match(semOnly, "C")},
	}
	svc := newSearchService(f, &mockEmbedder{embedding: []float32{0.1, 0.2}},
		config.SearchConfig{DefaultLimit: 10, DefaultThreshold: 0.3})

	results, warnings, err := svc.Search(context.Background(), 1, "query", 0, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].ID != both {
		t.Errorf("top result = %v, want the document found by both (%v)", results[0].ID, both)
	}

	byID := map[uuid.UUID]string{}
	for _, r := range results {
		byID[r.ID] = r.MatchType
	}
	for id, want := range map[uuid.UUID]string{textOnly: "keyword", both: "both", semOnly: "semantic"} {
		if byID[id] != want {
			t.Errorf("match type for %v = %q, want %q", id, byID[id], want)
		}
	}
}

// A higher-ranked hit scores above a lower-ranked one from the same result set.
func TestSearch_RanksByPosition(t *testing.T) {
	first, second := uuid.New(), uuid.New()

	f := &fakeSearch{text: []storage.VideoMatch{match(first, "A"), match(second, "B")}}
	svc := newSearchService(f, &mockEmbedder{err: errors.New("embedder down")},
		config.SearchConfig{DefaultLimit: 10})

	results, _, err := svc.Search(context.Background(), 1, "query", 0, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got := ids(results); !slices.Equal(got, []uuid.UUID{first, second}) {
		t.Errorf("order = %v, want %v", got, []uuid.UUID{first, second})
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("rank 0 score %.6f should exceed rank 1 score %.6f", results[0].Score, results[1].Score)
	}
}

// One failing search degrades to the other and says so. Both branches matter:
// a warning that never reaches the caller is a silent half-result.
func TestSearch_DegradesToOneSource(t *testing.T) {
	tests := []struct {
		name        string
		semErr      bool
		textErr     bool
		wantWarning string
	}{
		{"semantic fails", true, false, "Semantic search temporarily unavailable. Results from keyword search only."},
		{"text fails", false, true, "Keyword search failed. Results from semantic search only."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			f := &fakeSearch{}
			if tt.semErr {
				f.text = []storage.VideoMatch{match(id, "A")}
				f.semErr = errors.New("pgvector unavailable")
			} else {
				f.sem = []storage.VideoMatch{match(id, "A")}
				f.textErr = errors.New("tsquery failed")
			}
			svc := newSearchService(f, &mockEmbedder{embedding: []float32{0.1}},
				config.SearchConfig{DefaultLimit: 10})

			results, warnings, err := svc.Search(context.Background(), 1, "query", 0, "")
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(results) != 1 || results[0].ID != id {
				t.Errorf("results = %v, want the one surviving match %v", ids(results), id)
			}
			if len(warnings) != 1 || warnings[0] != tt.wantWarning {
				t.Errorf("warnings = %v, want [%q]", warnings, tt.wantWarning)
			}
		})
	}
}

// A failing embedder is the same case as a failing semantic query: degrade,
// warn, keep the keyword results. It reaches that branch without SearchVideos
// ever being called, which is the part worth pinning down.
func TestSearch_EmbedderFailureDegrades(t *testing.T) {
	id := uuid.New()
	f := &fakeSearch{text: []storage.VideoMatch{match(id, "A")}}
	emb := &mockEmbedder{err: errors.New("mistral 503")}
	svc := newSearchService(f, emb, config.SearchConfig{DefaultLimit: 10})

	results, warnings, err := svc.Search(context.Background(), 1, "query", 0, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want the keyword hit", len(results))
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want the semantic-unavailable warning", warnings)
	}
	if f.semanticCalls != 0 {
		t.Errorf("SearchVideos called %d times, want 0 — there is no embedding to query with", f.semanticCalls)
	}
}

// Both searches failing is an error, not an empty result set. An empty list
// would read to the caller as "nothing matched".
func TestSearch_BothFailingIsAnError(t *testing.T) {
	f := &fakeSearch{textErr: errors.New("tsquery failed"), semErr: errors.New("pgvector unavailable")}
	svc := newSearchService(f, &mockEmbedder{embedding: []float32{0.1}},
		config.SearchConfig{DefaultLimit: 10})

	results, _, err := svc.Search(context.Background(), 1, "query", 0, "")
	if err == nil {
		t.Fatalf("Search returned %d results and no error, want an error", len(results))
	}
	if !errors.Is(err, f.textErr) {
		t.Errorf("error = %v, want it to wrap the text search failure", err)
	}
}

// limit 0 takes the configured default, and the fetch limit is three times the
// effective limit with a floor of 30 — the over-fetch the fusion needs to have
// anything to merge.
func TestSearch_LimitDefaultsAndFetchLimit(t *testing.T) {
	tests := []struct {
		name           string
		limit          int
		defaultLimit   int
		wantFetchLimit int
	}{
		{"zero takes the configured default", 0, 20, 60},
		{"small limit hits the floor", 5, 20, 30},
		{"large limit triples", 40, 20, 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSearch{}
			svc := newSearchService(f, &mockEmbedder{embedding: []float32{0.1}},
				config.SearchConfig{DefaultLimit: tt.defaultLimit, DefaultThreshold: 0.42})

			if _, _, err := svc.Search(context.Background(), 1, "query", tt.limit, "podcast"); err != nil {
				t.Fatalf("Search: %v", err)
			}
			if f.gotTextLimit != tt.wantFetchLimit || f.gotSemLimit != tt.wantFetchLimit {
				t.Errorf("fetch limits = text %d, semantic %d, want %d for both",
					f.gotTextLimit, f.gotSemLimit, tt.wantFetchLimit)
			}
			if f.gotSource != "podcast" {
				t.Errorf("source = %q, want it passed through unchanged", f.gotSource)
			}
			if f.gotThreshold != 0.42 {
				t.Errorf("threshold = %v, want the configured 0.42", f.gotThreshold)
			}
		})
	}
}

// The result limit applies after fusion, so a document that only the over-fetch
// surfaced can still make the final list — and the list is cut to the limit.
func TestSearch_TruncatesToLimit(t *testing.T) {
	f := &fakeSearch{}
	for i := 0; i < 8; i++ {
		f.text = append(f.text, match(uuid.New(), "T"))
	}
	svc := newSearchService(f, &mockEmbedder{err: errors.New("no embedder")},
		config.SearchConfig{DefaultLimit: 10})

	results, _, err := svc.Search(context.Background(), 1, "query", 3, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("got %d results, want 3", len(results))
	}
}

// ScoreCutoffRatio drops the tail relative to the top score. At 0 nothing is
// dropped, which is why the other tests leave it unset.
func TestSearch_ScoreCutoff(t *testing.T) {
	f := &fakeSearch{}
	for i := 0; i < 10; i++ {
		f.text = append(f.text, match(uuid.New(), "T"))
	}
	svc := newSearchService(f, &mockEmbedder{err: errors.New("no embedder")},
		config.SearchConfig{DefaultLimit: 10, ScoreCutoffRatio: 0.99})

	results, _, err := svc.Search(context.Background(), 1, "query", 0, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// With K = 60, rank 1 scores 61/62 of rank 0 — below a 0.99 cutoff.
	if len(results) != 1 {
		t.Errorf("got %d results, want only the top one to clear a 0.99 cutoff", len(results))
	}
}

// No matches is an empty slice, never nil: the REST handler marshals this
// straight to JSON, and nil would serialize as null rather than [].
func TestSearch_EmptyResultIsNotNil(t *testing.T) {
	f := &fakeSearch{}
	svc := newSearchService(f, &mockEmbedder{embedding: []float32{0.1}},
		config.SearchConfig{DefaultLimit: 10})

	results, _, err := svc.Search(context.Background(), 1, "query", 0, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results == nil {
		t.Fatal("results is nil, want an empty slice")
	}
	data, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Errorf("marshalled = %s, want []", data)
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

	svc := New(
		nil, // db
		nil, // registry
		nil, // yt client
		nil, // cast2md client
		config.Cast2MDConfig{},
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

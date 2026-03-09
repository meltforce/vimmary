package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
)

// HybridMatch is a search result from hybrid (keyword + semantic) search.
type HybridMatch struct {
	ID        uuid.UUID       `json:"id"`
	YouTubeID string          `json:"youtube_id"`
	Title     string          `json:"title"`
	Channel   string          `json:"channel"`
	Summary   string          `json:"summary"`
	Metadata  json.RawMessage `json:"metadata"`
	Score     float64         `json:"score"`
	MatchType string          `json:"match_type"`
	CreatedAt time.Time       `json:"created_at"`
}

// Search runs hybrid (keyword + semantic) search with RRF merging.
func (s *Service) Search(ctx context.Context, userID int, query string, limit int) ([]HybridMatch, []string, error) {
	if limit == 0 {
		limit = s.searchCfg.DefaultLimit
	}
	fetchLimit := limit * 3
	if fetchLimit < 30 {
		fetchLimit = 30
	}

	type textResult struct {
		matches []storage.VideoMatch
		err     error
	}
	type semanticResult struct {
		matches []storage.VideoMatch
		err     error
	}

	textCh := make(chan textResult, 1)
	semCh := make(chan semanticResult, 1)

	go func() {
		matches, err := s.db.TextSearchVideos(ctx, userID, query, fetchLimit)
		textCh <- textResult{matches, err}
	}()

	go func() {
		embedding, err := s.embedder.Embed(ctx, query)
		if err != nil {
			semCh <- semanticResult{nil, err}
			return
		}
		matches, err := s.db.SearchVideos(ctx, userID, embedding, s.searchCfg.DefaultThreshold, fetchLimit)
		semCh <- semanticResult{matches, err}
	}()

	textRes := <-textCh
	semRes := <-semCh

	if textRes.err != nil && semRes.err != nil {
		return nil, nil, fmt.Errorf("both searches failed: text=%w, semantic=%v", textRes.err, semRes.err)
	}

	var warnings []string
	if semRes.err != nil {
		s.log.Warn("semantic search unavailable", "error", semRes.err)
		warnings = append(warnings, "Semantic search temporarily unavailable. Results from keyword search only.")
	}
	if textRes.err != nil {
		s.log.Warn("text search failed", "error", textRes.err)
		warnings = append(warnings, "Keyword search failed. Results from semantic search only.")
	}

	// RRF merge
	const K = 60
	type entry struct {
		HybridMatch
		hasText     bool
		hasSemantic bool
	}
	merged := make(map[uuid.UUID]*entry)

	if textRes.err == nil {
		for rank, m := range textRes.matches {
			e, ok := merged[m.ID]
			if !ok {
				e = &entry{HybridMatch: HybridMatch{
					ID: m.ID, YouTubeID: m.YouTubeID, Title: m.Title,
					Channel: m.Channel, Summary: m.Summary, Metadata: m.Metadata,
					CreatedAt: m.CreatedAt,
				}}
				merged[m.ID] = e
			}
			e.Score += 1.0 / float64(K+rank+1)
			e.hasText = true
		}
	}

	if semRes.err == nil {
		for rank, m := range semRes.matches {
			e, ok := merged[m.ID]
			if !ok {
				e = &entry{HybridMatch: HybridMatch{
					ID: m.ID, YouTubeID: m.YouTubeID, Title: m.Title,
					Channel: m.Channel, Summary: m.Summary, Metadata: m.Metadata,
					CreatedAt: m.CreatedAt,
				}}
				merged[m.ID] = e
			}
			e.Score += 1.0 / float64(K+rank+1)
			e.hasSemantic = true
		}
	}

	results := make([]HybridMatch, 0, len(merged))
	for _, e := range merged {
		switch {
		case e.hasText && e.hasSemantic:
			e.MatchType = "both"
		case e.hasText:
			e.MatchType = "keyword"
		default:
			e.MatchType = "semantic"
		}
		results = append(results, e.HybridMatch)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	if results == nil {
		results = []HybridMatch{}
	}

	return results, warnings, nil
}

// ListRecent returns recent completed videos with optional filters.
func (s *Service) ListRecent(ctx context.Context, userID int, filters storage.ListFilters, limit, offset int) ([]storage.Video, int, error) {
	if limit == 0 {
		limit = 20
	}
	return s.db.ListRecent(ctx, userID, filters, limit, offset)
}

// GetVideo returns a single video by ID.
func (s *Service) GetVideo(ctx context.Context, userID int, id uuid.UUID) (*storage.Video, error) {
	return s.db.GetVideo(ctx, userID, id)
}

// Stats returns aggregate statistics.
func (s *Service) Stats(ctx context.Context, userID int) (*storage.VideoStats, error) {
	return s.db.GetStats(ctx, userID)
}

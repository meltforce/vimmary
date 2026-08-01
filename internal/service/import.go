package service

import (
	"context"
	"fmt"

	"github.com/meltforce/vimmary/internal/karakeep"
)

// ImportResult holds the result of a Karakeep import.
type ImportResult struct {
	Total    int `json:"total"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// ImportKarakeepBookmarks imports all YouTube bookmarks from a user's Karakeep account.
func (s *Service) ImportKarakeepBookmarks(ctx context.Context, userID int) (*ImportResult, error) {
	apiKey, err := s.db.GetKarakeepAPIKey(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get karakeep API key: %w", err)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("karakeep API key not configured")
	}

	client := karakeep.NewClient(s.karakeepBaseURL, apiKey)
	bookmarks, err := client.ListBookmarks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}

	result := &ImportResult{}
	var toProcess []struct {
		youtubeID  string
		bookmarkID string
	}

	for _, bm := range bookmarks {
		youtubeID := karakeep.ExtractYouTubeID(bm.Content.URL)
		if youtubeID == "" {
			continue
		}
		result.Total++

		existing, err := s.db.GetByYouTubeID(ctx, userID, youtubeID)
		if err == nil && existing.Status == "completed" {
			// Already processed — backfill bookmark ID if missing
			if existing.KarakeepBookmarkID == "" {
				if err := s.db.UpdateBookmarkID(ctx, existing.ID, bm.ID); err != nil {
					s.log.Warn("failed to backfill bookmark ID", "video_id", existing.ID, "error", err)
				}
			}
			result.Skipped++
			continue
		}

		toProcess = append(toProcess, struct {
			youtubeID  string
			bookmarkID string
		}{youtubeID, bm.ID})
	}

	result.Imported = len(toProcess)

	for _, item := range toProcess {
		s.ProcessVideoAsync(userID, item.youtubeID, item.bookmarkID)
	}

	return result, nil
}

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Resummarize regenerates the summary for a video with a new detail level.
func (s *Service) Resummarize(ctx context.Context, userID int, videoID uuid.UUID, level string) error {
	if level == "" {
		level = "deep"
	}

	video, err := s.db.GetVideo(ctx, userID, videoID)
	if err != nil {
		return fmt.Errorf("get video: %w", err)
	}

	if video.Transcript == "" {
		return fmt.Errorf("no transcript available for video %s", video.YouTubeID)
	}

	// Generate new summary
	sum, err := s.summarizer.Summarize(ctx, video.Title, video.Transcript, level)
	if err != nil {
		return fmt.Errorf("generate summary: %w", err)
	}

	// Generate embedding
	embeddingText := video.Title + "\n\n" + sum.Text
	embedding, err := s.embedder.Embed(ctx, embeddingText)
	if err != nil {
		s.log.Warn("embedding failed during resummarize", "video_id", videoID, "error", err)
	}

	metadata := map[string]any{
		"topics":       sum.Topics,
		"key_points":   sum.KeyPoints,
		"action_items": sum.ActionItems,
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := s.db.UpdateVideoSummary(ctx, videoID, sum.Text, level, embedding, metaJSON); err != nil {
		return fmt.Errorf("update summary: %w", err)
	}

	s.log.Info("video resummarized", "video_id", videoID, "level", level)

	// Update Karakeep if applicable
	if s.karakeep != nil && video.KarakeepBookmarkID != "" {
		s.writeBackToKarakeep(ctx, video.KarakeepBookmarkID, video.Title, sum.Text)
	}

	return nil
}

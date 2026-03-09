package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ResummarizeAsync starts resummarization in a background goroutine.
func (s *Service) ResummarizeAsync(userID int, videoID uuid.UUID, level, language string) error {
	ctx := context.Background()

	// Validate video exists and has transcript before going async
	video, err := s.db.GetVideo(ctx, userID, videoID)
	if err != nil {
		return fmt.Errorf("get video: %w", err)
	}
	if video.Transcript == "" {
		return fmt.Errorf("no transcript available for video %s", video.YouTubeID)
	}

	// Set status to processing so the UI can track it
	if err := s.db.UpdateVideoStatus(ctx, videoID, "processing", ""); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	go func() {
		if err := s.Resummarize(context.Background(), userID, videoID, level, language); err != nil {
			s.log.Error("resummarize failed", "video_id", videoID, "error", err)
			_ = s.db.UpdateVideoStatus(context.Background(), videoID, "failed", err.Error())
		}
	}()
	return nil
}

// Resummarize regenerates the summary for a video with a new detail level.
func (s *Service) Resummarize(ctx context.Context, userID int, videoID uuid.UUID, level, language string) error {
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

	// Use override language if provided, otherwise keep video's detected language
	lang := video.Language
	if language != "" {
		lang = language
	}

	// Generate new summary
	sum, err := s.summarizer.Summarize(ctx, video.Title, video.Transcript, level, lang)
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
	if video.KarakeepBookmarkID != "" {
		s.writeBackToKarakeep(ctx, userID, video.KarakeepBookmarkID, videoID, video.Title, sum.Text)
	}

	return nil
}

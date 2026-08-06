package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ResummarizeAsync starts resummarization in a background goroutine.
func (s *Service) ResummarizeAsync(userID int, videoID uuid.UUID, level, language, provider string) error {
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
		if err := s.Resummarize(context.Background(), userID, videoID, level, language, provider); err != nil {
			s.log.Error("resummarize failed", "video_id", videoID, "error", err)
			_ = s.db.UpdateVideoStatus(context.Background(), videoID, "failed", err.Error())
		}
	}()
	return nil
}

// Resummarize regenerates the summary for a video with a new detail level.
func (s *Service) Resummarize(ctx context.Context, userID int, videoID uuid.UUID, level, language, provider string) error {
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

	summaryText, err := s.summarizeAndStore(ctx, summarizeRequest{
		userID:     userID,
		video:      video,
		transcript: video.Transcript,
		title:      video.Title,
		language:   lang,
		level:      level,
		provider:   provider,
	})
	if err != nil {
		return err
	}

	s.log.Info("video resummarized", "video_id", videoID, "source", video.Source, "level", level)

	// Update Karakeep if applicable
	if video.KarakeepBookmarkID != "" {
		s.writeBackToKarakeep(ctx, userID, video.KarakeepBookmarkID, videoID, video.Title, summaryText)
	}

	return nil
}

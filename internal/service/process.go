package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/karakeep"
	"github.com/meltforce/vimmary/internal/storage"
)

var (
	mdHeaderRe = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdBoldRe   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdItalicRe = regexp.MustCompile(`\*(.+?)\*`)
	mdLinkRe   = regexp.MustCompile(`\[(.+?)\]\(.+?\)`)
	mdBulletRe = regexp.MustCompile(`(?m)^[\t ]*[-*]\s+`)
)

// stripMarkdown converts markdown text to plain text for Karakeep notes.
func stripMarkdown(s string) string {
	s = mdHeaderRe.ReplaceAllString(s, "")
	s = mdBoldRe.ReplaceAllString(s, "$1")
	s = mdItalicRe.ReplaceAllString(s, "$1")
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = mdBulletRe.ReplaceAllString(s, "- ")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

// ProcessVideoAsync starts video processing in a background goroutine.
func (s *Service) ProcessVideoAsync(userID int, youtubeID, bookmarkID string) {
	go func() {
		ctx := context.Background()
		if err := s.ProcessVideo(ctx, userID, youtubeID, bookmarkID); err != nil {
			s.log.Error("video processing failed", "youtube_id", youtubeID, "error", err)
		}
	}()
}

// ProcessVideo fetches transcript, generates summary, creates embedding, stores in DB,
// and writes back to Karakeep.
func (s *Service) ProcessVideo(ctx context.Context, userID int, youtubeID, bookmarkID string) error {
	s.log.Info("processing video", "youtube_id", youtubeID, "bookmark_id", bookmarkID)

	// Check if already processed
	existing, err := s.db.GetByYouTubeID(ctx, userID, youtubeID)
	if err == nil && existing.Status == "completed" {
		// Video exists — update bookmark ID if webhook provides one, then do writeback
		if bookmarkID != "" && existing.KarakeepBookmarkID != bookmarkID {
			if err := s.db.UpdateBookmarkID(ctx, existing.ID, bookmarkID); err != nil {
				s.log.Warn("failed to update bookmark ID", "video_id", existing.ID, "error", err)
			}
			s.writeBackToKarakeep(ctx, userID, bookmarkID, existing.ID, existing.Title, existing.Summary)
		}
		s.log.Info("video already processed", "youtube_id", youtubeID)
		return nil
	}

	// Create or update video record
	var video *storage.Video
	if existing != nil {
		video = existing
	} else {
		video = &storage.Video{
			ID:                 uuid.New(),
			UserID:             userID,
			KarakeepBookmarkID: bookmarkID,
			YouTubeID:          youtubeID,
			DetailLevel:        s.summaryCfg.DefaultLevel,
			Status:             "pending",
		}
		if err := s.db.InsertVideo(ctx, video); err != nil {
			return fmt.Errorf("insert video: %w", err)
		}
	}

	// Update status to processing
	if err := s.db.UpdateVideoStatus(ctx, video.ID, "processing", ""); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Fetch metadata
	meta, err := s.yt.FetchMetadata(ctx, youtubeID)
	if err != nil {
		s.log.Warn("metadata fetch failed, continuing", "youtube_id", youtubeID, "error", err)
	}

	// Fetch transcript
	transcript, err := s.yt.FetchTranscript(ctx, youtubeID)
	if err != nil {
		errMsg := fmt.Sprintf("transcript fetch failed: %v", err)
		_ = s.db.UpdateVideoStatus(ctx, video.ID, "failed", errMsg)
		return fmt.Errorf("fetch transcript: %w", err)
	}

	// Update video with transcript and metadata
	title := ""
	channel := ""
	language := transcript.Language
	duration := 0
	if meta != nil {
		title = meta.Title
		channel = meta.Channel
		duration = meta.DurationSeconds
		if meta.Language != "" {
			language = meta.Language
		}
	}

	if err := s.db.UpdateVideoTranscript(ctx, video.ID, transcript.Text, title, channel, language, duration); err != nil {
		return fmt.Errorf("update transcript: %w", err)
	}

	// Generate summary
	sum, err := s.summarizer.Summarize(ctx, title, transcript.Text, video.DetailLevel, language)
	if err != nil {
		errMsg := fmt.Sprintf("summary generation failed: %v", err)
		_ = s.db.UpdateVideoStatus(ctx, video.ID, "failed", errMsg)
		return fmt.Errorf("generate summary: %w", err)
	}

	// Generate embedding from summary + title
	embeddingText := title + "\n\n" + sum.Text
	embedding, err := s.embedder.Embed(ctx, embeddingText)
	if err != nil {
		s.log.Warn("embedding generation failed, saving without embedding", "youtube_id", youtubeID, "error", err)
	}

	// Build metadata JSON
	metadata := map[string]any{
		"topics":       sum.Topics,
		"key_points":   sum.KeyPoints,
		"action_items": sum.ActionItems,
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Store summary + embedding
	if err := s.db.UpdateVideoSummary(ctx, video.ID, sum.Text, video.DetailLevel, embedding, metaJSON); err != nil {
		return fmt.Errorf("update summary: %w", err)
	}

	s.log.Info("video processed successfully", "youtube_id", youtubeID, "title", title)

	// Write back to Karakeep (best-effort, per-user client)
	if bookmarkID != "" {
		s.writeBackToKarakeep(ctx, userID, bookmarkID, video.ID, title, sum.Text)
	}

	return nil
}

func (s *Service) writeBackToKarakeep(ctx context.Context, userID int, bookmarkID string, videoID uuid.UUID, title, summaryText string) {
	if s.karakeepBaseURL == "" {
		return
	}

	apiKey, err := s.db.GetKarakeepAPIKey(ctx, userID)
	if err != nil || apiKey == "" {
		s.log.Debug("no karakeep API key for user, skipping writeback", "user_id", userID)
		return
	}

	client := karakeep.NewClient(s.karakeepBaseURL, apiKey)

	plain := stripMarkdown(summaryText)
	var note string
	if s.externalURL != "" {
		note = s.externalURL + "/video/" + videoID.String() + "\n\n"
	}
	if title != "" {
		note += title + "\n\n"
	}
	note += plain

	if err := client.UpdateNote(ctx, bookmarkID, note); err != nil {
		s.log.Warn("karakeep note update failed", "bookmark_id", bookmarkID, "error", err)
	}

	// Delay before tagging so Karakeep's AI tagging finishes first.
	// AddTag reads existing tags and appends, so we need AI tags to be present.
	time.Sleep(10 * time.Second)

	if err := client.AddTag(ctx, bookmarkID, "video-summarized"); err != nil {
		s.log.Warn("karakeep tag update failed", "bookmark_id", bookmarkID, "error", err)
	}
}

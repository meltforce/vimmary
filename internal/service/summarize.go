package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
)

// summarizeRequest carries everything the generic summary tail needs. The
// caller has already produced a transcript by whatever route its source
// requires.
type summarizeRequest struct {
	userID     int
	video      *storage.Video
	transcript string
	title      string
	language   string
	level      string
	provider   string // empty selects the default provider
}

// summarizeAndStore generates the summary and embedding for an existing row and
// writes both back. It is the part of the pipeline that does not depend on
// where the transcript came from, so ProcessVideo, Resummarize and
// ProcessEpisode all end here.
func (s *Service) summarizeAndStore(ctx context.Context, req summarizeRequest) (string, error) {
	summarizer, providerName, err := s.getSummarizer(ctx, req.provider)
	if err != nil {
		return "", fmt.Errorf("get summarizer: %w", err)
	}

	model := s.getModelForProvider(ctx, req.userID, providerName)
	customPrompt := s.getUserPrompt(ctx, req.userID, req.video.Source, req.level)

	sum, err := summarizer.Summarize(ctx, summary.Request{
		Source:       req.video.Source,
		Title:        req.title,
		Transcript:   req.transcript,
		Level:        req.level,
		Language:     req.language,
		CustomPrompt: customPrompt,
		Model:        model,
		// The existing vocabulary steers new tags onto old ones, so the tag
		// set converges instead of growing a near-synonym per video.
		ExistingTopics: s.existingTopics(ctx, req.userID),
	})
	if err != nil {
		return "", fmt.Errorf("generate summary: %w", err)
	}

	embeddingText := req.title + "\n\n" + sum.Text
	embedding, err := s.embedder.Embed(ctx, embeddingText)
	if err != nil {
		s.log.Warn("embedding generation failed, saving without embedding",
			"video_id", req.video.ID, "source", req.video.Source, "error", err)
	}

	metadata := map[string]any{
		"topics":       sum.Topics,
		"key_points":   sum.KeyPoints,
		"action_items": sum.ActionItems,
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	if err := s.db.UpdateVideoSummary(ctx, req.video.ID, sum.Text, req.level, providerName, model,
		sum.Usage.InputTokens, sum.Usage.OutputTokens, embedding, metaJSON); err != nil {
		return "", fmt.Errorf("update summary: %w", err)
	}

	return sum.Text, nil
}

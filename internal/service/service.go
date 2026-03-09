package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
	"github.com/meltforce/vimmary/internal/youtube"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Service contains all business logic for vimmary.
type Service struct {
	db              *storage.DB
	summarizer      summary.Summarizer
	yt              *youtube.Client
	karakeepBaseURL string
	externalURL     string
	embedder        Embedder
	searchCfg       config.SearchConfig
	summaryCfg      config.SummaryConfig
	log             *slog.Logger
}

// New creates a new Service.
func New(
	db *storage.DB,
	summarizer summary.Summarizer,
	yt *youtube.Client,
	karakeepBaseURL string,
	externalURL string,
	embedder Embedder,
	searchCfg config.SearchConfig,
	summaryCfg config.SummaryConfig,
	log *slog.Logger,
) *Service {
	return &Service{
		db:              db,
		summarizer:      summarizer,
		yt:              yt,
		karakeepBaseURL: karakeepBaseURL,
		externalURL:     externalURL,
		embedder:        embedder,
		searchCfg:       searchCfg,
		summaryCfg:      summaryCfg,
		log:             log,
	}
}

// DeleteVideo removes a video from the database.
func (s *Service) DeleteVideo(ctx context.Context, userID int, id uuid.UUID) error {
	return s.db.DeleteVideo(ctx, userID, id)
}

// DeleteByBookmarkID removes a video by its Karakeep bookmark ID.
func (s *Service) DeleteByBookmarkID(ctx context.Context, userID int, bookmarkID string) error {
	return s.db.DeleteByBookmarkID(ctx, userID, bookmarkID)
}

// RetryVideo resets a failed video and re-processes it.
func (s *Service) RetryVideo(ctx context.Context, userID int, id uuid.UUID) error {
	video, err := s.db.GetVideo(ctx, userID, id)
	if err != nil {
		return err
	}
	if video.Status != "failed" {
		return fmt.Errorf("video is not in failed state (status: %s)", video.Status)
	}
	if err := s.db.UpdateVideoStatus(ctx, id, "pending", ""); err != nil {
		return fmt.Errorf("reset status: %w", err)
	}
	s.ProcessVideoAsync(userID, video.YouTubeID, video.KarakeepBookmarkID)
	return nil
}

// GetWebhookInfo returns the webhook URL and token for a user.
func (s *Service) GetWebhookInfo(ctx context.Context, userID int) (token string, err error) {
	token, err = s.db.GetOrCreateWebhookToken(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get webhook token: %w", err)
	}
	return token, nil
}

// SetKarakeepAPIKey stores a Karakeep API key for a user.
func (s *Service) SetKarakeepAPIKey(ctx context.Context, userID int, apiKey string) error {
	return s.db.SetKarakeepAPIKey(ctx, userID, apiKey)
}

// HasKarakeepAPIKey returns whether a user has a Karakeep API key set.
func (s *Service) HasKarakeepAPIKey(ctx context.Context, userID int) (bool, error) {
	key, err := s.db.GetKarakeepAPIKey(ctx, userID)
	if err != nil {
		return false, err
	}
	return key != "", nil
}

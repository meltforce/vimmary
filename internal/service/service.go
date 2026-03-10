package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/models"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
	"github.com/meltforce/vimmary/internal/youtube"
)

// SummaryPromptsInfo holds the current and default prompts for the API.
type SummaryPromptsInfo struct {
	Medium        string `json:"medium"`
	Deep          string `json:"deep"`
	DefaultMedium string `json:"default_medium"`
	DefaultDeep   string `json:"default_deep"`
}

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// processJob represents a video processing request queued for rate-limited execution.
type processJob struct {
	userID     int
	youtubeID  string
	bookmarkID string
}

// Service contains all business logic for vimmary.
type Service struct {
	db              *storage.DB
	summarizers     map[string]summary.Summarizer
	defaultProvider string
	registry        *models.Registry
	yt              *youtube.Client
	karakeepBaseURL string
	externalURL     string
	embedder        Embedder
	searchCfg       config.SearchConfig
	summaryCfg      config.SummaryConfig
	log             *slog.Logger
	queue           chan processJob
}

// New creates a new Service.
func New(
	db *storage.DB,
	summarizers map[string]summary.Summarizer,
	defaultProvider string,
	registry *models.Registry,
	yt *youtube.Client,
	karakeepBaseURL string,
	externalURL string,
	embedder Embedder,
	searchCfg config.SearchConfig,
	summaryCfg config.SummaryConfig,
	log *slog.Logger,
) *Service {
	s := &Service{
		db:              db,
		summarizers:     summarizers,
		defaultProvider: defaultProvider,
		registry:        registry,
		yt:              yt,
		karakeepBaseURL: karakeepBaseURL,
		externalURL:     externalURL,
		embedder:        embedder,
		searchCfg:       searchCfg,
		summaryCfg:      summaryCfg,
		log:             log,
		queue:           make(chan processJob, 100),
	}
	go s.processWorker()
	return s
}

// processWorker drains the queue sequentially with rate limiting to avoid YouTube 429s.
func (s *Service) processWorker() {
	var last time.Time
	for job := range s.queue {
		if since := time.Since(last); since < 10*time.Second {
			time.Sleep(10*time.Second - since)
		}
		if err := s.ProcessVideo(context.Background(), job.userID, job.youtubeID, job.bookmarkID); err != nil {
			s.log.Error("video processing failed", "youtube_id", job.youtubeID, "error", err)
		}
		last = time.Now()
	}
}

// getSummarizer returns the summarizer for the given provider name.
// If provider is empty, the default provider is used.
func (s *Service) getSummarizer(provider string) (summary.Summarizer, string, error) {
	if provider == "" {
		provider = s.defaultProvider
	}
	sum, ok := s.summarizers[provider]
	if !ok {
		return nil, "", fmt.Errorf("unknown provider: %q", provider)
	}
	return sum, provider, nil
}

// AvailableProviders returns the names of all configured summarizer providers.
func (s *Service) AvailableProviders() []string {
	providers := make([]string, 0, len(s.summarizers))
	for name := range s.summarizers {
		providers = append(providers, name)
	}
	return providers
}

// DefaultProvider returns the name of the default summarizer provider.
func (s *Service) DefaultProvider() string {
	return s.defaultProvider
}

// KarakeepBaseURL returns the configured Karakeep base URL.
func (s *Service) KarakeepBaseURL() string {
	return s.karakeepBaseURL
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

// getUserPrompt returns the user's custom prompt for the given level, or empty string for default.
func (s *Service) getUserPrompt(ctx context.Context, userID int, level string) string {
	medium, deep, err := s.db.GetSummaryPrompts(ctx, userID)
	if err != nil {
		s.log.Warn("failed to load custom prompts, using defaults", "user_id", userID, "error", err)
		return ""
	}
	if level == "deep" && deep != nil {
		return *deep
	}
	if level != "deep" && medium != nil {
		return *medium
	}
	return ""
}

// GetSummaryPrompts returns the user's current and default prompts.
func (s *Service) GetSummaryPrompts(ctx context.Context, userID int) (*SummaryPromptsInfo, error) {
	medium, deep, err := s.db.GetSummaryPrompts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get prompts: %w", err)
	}

	info := &SummaryPromptsInfo{
		DefaultMedium: summary.DefaultPrompt("medium"),
		DefaultDeep:   summary.DefaultPrompt("deep"),
	}
	if medium != nil {
		info.Medium = *medium
	} else {
		info.Medium = info.DefaultMedium
	}
	if deep != nil {
		info.Deep = *deep
	} else {
		info.Deep = info.DefaultDeep
	}
	return info, nil
}

// SetSummaryPrompt sets a custom prompt for the given level. Empty string resets to default.
func (s *Service) SetSummaryPrompt(ctx context.Context, userID int, level, prompt string) error {
	if level != "medium" && level != "deep" {
		return fmt.Errorf("invalid level: %q (must be medium or deep)", level)
	}
	return s.db.SetSummaryPrompt(ctx, userID, level, prompt)
}

// ListModels returns available models for a provider.
func (s *Service) ListModels(ctx context.Context, provider string) ([]models.Model, error) {
	return s.registry.ListModels(ctx, provider)
}

// GetModelPreferences returns the user's preferred models for all providers.
func (s *Service) GetModelPreferences(ctx context.Context, userID int) (claude, mistral string, err error) {
	return s.db.GetModelPreferences(ctx, userID)
}

// SetModelPreference sets the user's preferred model for a provider.
func (s *Service) SetModelPreference(ctx context.Context, userID int, provider, model string) error {
	if provider != "claude" && provider != "mistral" {
		return fmt.Errorf("invalid provider: %q (must be claude or mistral)", provider)
	}
	return s.db.SetModelPreference(ctx, userID, provider, model)
}

// getModelForProvider resolves the model to use: user preference → config fallback → empty (provider default).
func (s *Service) getModelForProvider(ctx context.Context, userID int, provider string) string {
	model, err := s.db.GetModelPreference(ctx, userID, provider)
	if err != nil {
		s.log.Warn("failed to load model preference, using config fallback", "user_id", userID, "provider", provider, "error", err)
	}
	if model != "" {
		return model
	}
	// Config fallback
	switch provider {
	case "claude":
		return s.summaryCfg.ClaudeModel
	case "mistral":
		return s.summaryCfg.MistralModel
	}
	return ""
}

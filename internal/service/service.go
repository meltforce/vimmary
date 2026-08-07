package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/models"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
	"github.com/meltforce/vimmary/internal/youtube"
)

// SummaryPromptsInfo holds the current and default prompts for the API.
type SummaryPromptsInfo struct {
	Source        string `json:"source"`
	Medium        string `json:"medium"`
	Deep          string `json:"deep"`
	DefaultMedium string `json:"default_medium"`
	DefaultDeep   string `json:"default_deep"`
}

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Transcriber transcribes audio files to text.
type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

// processJob represents a video processing request queued for rate-limited execution.
type processJob struct {
	userID       int
	youtubeID    string
	bookmarkID   string
	retryCount   int
	forceVoxtral bool
}

// episodeJob represents a podcast episode queued for summarization.
type episodeJob struct {
	userID     int
	episodeID  int
	level      string
	retryCount int
}

// Service contains all business logic for vimmary.
type Service struct {
	db *storage.DB
	// settings and newSummarizer are the seams for the summarizer path; New
	// wires both to the real implementations. Everything else reaches storage
	// through db directly.
	settings        settingsSource
	newSummarizer   summarizerFactory
	registry        *models.Registry
	yt              *youtube.Client
	cast2md         PodcastSource
	cast2mdCfg      config.Cast2MDConfig
	karakeepBaseURL string
	externalURL     string
	embedder        Embedder
	transcriber     Transcriber
	searchCfg       config.SearchConfig
	summaryCfg      config.SummaryConfig
	log             *slog.Logger
	queue           chan processJob
	episodeQueue    chan episodeJob
}

// New creates a new Service.
func New(
	db *storage.DB,
	registry *models.Registry,
	yt *youtube.Client,
	c2m PodcastSource,
	cast2mdCfg config.Cast2MDConfig,
	karakeepBaseURL string,
	externalURL string,
	embedder Embedder,
	transcriber Transcriber,
	searchCfg config.SearchConfig,
	summaryCfg config.SummaryConfig,
	log *slog.Logger,
) *Service {
	s := &Service{
		db:              db,
		settings:        db,
		newSummarizer:   newSummarizer,
		registry:        registry,
		yt:              yt,
		cast2md:         c2m,
		cast2mdCfg:      cast2mdCfg,
		karakeepBaseURL: karakeepBaseURL,
		externalURL:     externalURL,
		embedder:        embedder,
		transcriber:     transcriber,
		searchCfg:       searchCfg,
		summaryCfg:      summaryCfg,
		log:             log,
		queue:           make(chan processJob, 100),
		episodeQueue:    make(chan episodeJob, 200),
	}
	go s.processWorker()
	go s.episodeWorker()
	return s
}

// adaptiveDelay returns the rate-limit delay based on current queue depth.
func (s *Service) adaptiveDelay() time.Duration {
	depth := len(s.queue)
	switch {
	case depth >= 26:
		return 45 * time.Second
	case depth >= 11:
		return 30 * time.Second
	case depth >= 4:
		return 20 * time.Second
	default:
		return 10 * time.Second
	}
}

// retryBackoff returns the backoff duration for the given retry attempt (1-indexed).
func retryBackoff(retry int) time.Duration {
	switch retry {
	case 1:
		return 2 * time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 10 * time.Minute
	}
}

const maxRetries = 3

// noCaptionsPatterns are error substrings indicating a video has no captions available.
var noCaptionsPatterns = []string{
	"no transcripts available",
	"error getting transcript from track",
	"no transcript found",
}

// isNoCaptionsError returns true if the error indicates a permanent no-captions condition.
func isNoCaptionsError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, pattern := range noCaptionsPatterns {
		if strings.Contains(msg, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// processWorker drains the queue sequentially with adaptive rate limiting to avoid YouTube 429s.
func (s *Service) processWorker() {
	var last time.Time
	for job := range s.queue {
		delay := s.adaptiveDelay()
		if since := time.Since(last); since < delay {
			time.Sleep(delay - since)
		}
		if err := s.ProcessVideo(context.Background(), job.userID, job.youtubeID, job.bookmarkID, job.forceVoxtral); err != nil {
			s.log.Error("video processing failed", "youtube_id", job.youtubeID, "retry", job.retryCount, "error", err)

			// Don't retry no-captions errors — they're permanent
			if isNoCaptionsError(err) {
				continue
			}

			// Auto-retry transcript fetch failures with backoff
			if job.retryCount < maxRetries && strings.Contains(err.Error(), "transcript fetch failed") {
				nextRetry := job.retryCount + 1
				backoff := retryBackoff(nextRetry)
				s.log.Info("scheduling retry for video", "youtube_id", job.youtubeID, "retry", nextRetry, "delay", backoff)
				retryJob := processJob{
					userID:     job.userID,
					youtubeID:  job.youtubeID,
					bookmarkID: job.bookmarkID,
					retryCount: nextRetry,
				}
				time.AfterFunc(backoff, func() {
					select {
					case s.queue <- retryJob:
					default:
						s.log.Warn("queue full, dropping retry", "youtube_id", retryJob.youtubeID, "retry", retryJob.retryCount)
					}
				})
			}
		}
		last = time.Now()
	}
}

// episodeWorker drains the podcast queue. It is separate from processWorker
// because adaptiveDelay compensates for YouTube's rate limit, which does not
// apply to cast2md, and because a three-hour episode would otherwise block
// every YouTube job for the duration of its LLM call.
func (s *Service) episodeWorker() {
	for job := range s.episodeQueue {
		err := s.ProcessEpisode(context.Background(), job.userID, job.episodeID, job.level)
		if err == nil {
			continue
		}

		s.log.Error("episode processing failed",
			"episode_id", job.episodeID, "retry", job.retryCount, "error", err)

		// A transcript that is not ready yet, or a cast2md that is briefly
		// unreachable, is worth retrying; a failed summary is not.
		retryable := IsEpisodeNotReady(err) || strings.Contains(err.Error(), "transcript fetch failed")
		if !retryable || job.retryCount >= maxRetries {
			continue
		}

		nextRetry := job.retryCount + 1
		backoff := retryBackoff(nextRetry)
		s.log.Info("scheduling retry for episode",
			"episode_id", job.episodeID, "retry", nextRetry, "delay", backoff)
		retryJob := episodeJob{
			userID:     job.userID,
			episodeID:  job.episodeID,
			level:      job.level,
			retryCount: nextRetry,
		}
		time.AfterFunc(backoff, func() { s.enqueueEpisode(retryJob) })
	}
}

// settingsSource is the part of storage.DB that the summarizer path reads. It
// exists as a seam: getSummarizer builds its summarizer from a key in the
// database, so without this a test of provider selection would need a live
// database and a live API key. storage.DB satisfies it.
type settingsSource interface {
	GetLLMKey(ctx context.Context, provider string) (string, error)
	GetAppSetting(ctx context.Context, key string) (string, error)
}

// summarizerFactory builds a summarizer for a provider from its API key. The
// second seam: it is what a test replaces to exercise the path without calling
// a provider API.
type summarizerFactory func(provider, apiKey string) (summary.Summarizer, error)

// newSummarizer is the production factory. Provider names are matched here and
// in models.Providers; a name in one and not the other is a bug.
func newSummarizer(provider, apiKey string) (summary.Summarizer, error) {
	switch provider {
	case "claude":
		return summary.NewClaudeSummarizer(apiKey, "", nil), nil
	case "mistral":
		return summary.NewMistralSummarizer(apiKey), nil
	}
	return nil, fmt.Errorf("unknown provider: %q", provider)
}

// LLMKey returns a provider's API key, empty when it is not configured.
func (s *Service) LLMKey(ctx context.Context, provider string) (string, error) {
	return s.settings.GetLLMKey(ctx, provider)
}

// LLMSettings is what the Settings page shows. It reports whether each key is
// set, never the key itself — the same contract as the Karakeep status
// endpoint, so a key that goes in never comes back out.
type LLMSettings struct {
	MistralConfigured   bool   `json:"mistral_configured"`
	AnthropicConfigured bool   `json:"anthropic_configured"`
	Provider            string `json:"provider"`
}

// GetLLMSettings reports the service-wide LLM configuration.
func (s *Service) GetLLMSettings(ctx context.Context) (LLMSettings, error) {
	mistral, err := s.LLMKey(ctx, "mistral")
	if err != nil {
		return LLMSettings{}, err
	}
	anthropic, err := s.LLMKey(ctx, "claude")
	if err != nil {
		return LLMSettings{}, err
	}
	provider, err := s.SummaryProvider(ctx)
	if err != nil {
		return LLMSettings{}, err
	}
	return LLMSettings{
		MistralConfigured:   mistral != "",
		AnthropicConfigured: anthropic != "",
		Provider:            provider,
	}, nil
}

// SetLLMKey stores a provider's API key. An empty key clears it, which is how
// a provider is switched off — Anthropic is expected to stay empty here.
func (s *Service) SetLLMKey(ctx context.Context, provider, key string) error {
	switch provider {
	case "claude":
		return s.db.SetAppSetting(ctx, storage.SettingAnthropicAPIKey, key)
	case "mistral":
		return s.db.SetAppSetting(ctx, storage.SettingMistralAPIKey, key)
	}
	return fmt.Errorf("unknown provider: %q", provider)
}

// SetSummaryProvider stores the provider used when a request names none. It is
// validated against the providers that have a key, so the selection cannot name
// something that would fail on every summary. Storing an empty string restores
// the config file as the source.
func (s *Service) SetSummaryProvider(ctx context.Context, provider string) error {
	if provider != "" && !slices.Contains(s.AvailableProviders(ctx), provider) {
		return fmt.Errorf("provider %q has no API key configured", provider)
	}
	return s.db.SetAppSetting(ctx, storage.SettingSummaryProvider, provider)
}

// IsAdmin reports whether the user may change the service-wide settings.
//
// The rule is "you are the primary user" — the first login containing `@`, by
// creation time. That is not a new notion of owner: meltkit's identity
// middleware already resolves every tagged device to exactly this user, so
// reusing it keeps one answer to who owns the instance instead of two that can
// disagree.
//
// When no primary user exists, the caller is admin. That state means no
// personal Tailscale login has ever reached this instance, so there is no owner
// to protect the settings from — it is a local run against the seeded `local`
// user, where the alternative is a deployment whose keys cannot be configured
// at all. It cannot occur behind Tailscale: the middleware rejects a tagged
// device with "no registered user yet" until a personal device has logged in,
// and that login creates the primary user. The rule therefore tightens by
// itself the moment there is someone to tighten it for.
func (s *Service) IsAdmin(ctx context.Context, userID int) (bool, error) {
	primaryID, _, err := s.db.GetPrimaryUser(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return userID == primaryID, nil
}

// getSummarizer builds the summarizer for the given provider name. If provider
// is empty, the configured default is used.
//
// Built per call, not held: the API keys are service-wide settings maintained
// in the Settings page, so a key entered there has to take effect without a
// restart. This follows writeBackToKarakeep, which reads its key from the
// database on every use for the same reason. The summarizer structs are
// stateless, so the cost is one http.Client allocation per summary.
func (s *Service) getSummarizer(ctx context.Context, provider string) (summary.Summarizer, string, error) {
	if provider == "" {
		var err error
		provider, err = s.SummaryProvider(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("resolve default provider: %w", err)
		}
	}

	apiKey, err := s.LLMKey(ctx, provider)
	if err != nil {
		return nil, "", fmt.Errorf("api key for %q: %w", provider, err)
	}
	if apiKey == "" {
		return nil, "", fmt.Errorf("provider %q has no API key configured — set one under Settings", provider)
	}

	sum, err := s.newSummarizer(provider, apiKey)
	if err != nil {
		return nil, "", err
	}
	return sum, provider, nil
}

// SummaryProvider returns the provider used when a request names none: the
// app_settings value, falling back to the config file. The config keeps working
// as the initial value for an unattended deployment, and the Settings page
// overrides it — the same shape as user_prompts over the built-in defaults.
func (s *Service) SummaryProvider(ctx context.Context) (string, error) {
	provider, err := s.settings.GetAppSetting(ctx, storage.SettingSummaryProvider)
	if err != nil {
		return "", err
	}
	if provider != "" {
		return provider, nil
	}
	return s.summaryCfg.Provider, nil
}

// AvailableProviders returns the providers that have an API key configured.
func (s *Service) AvailableProviders(ctx context.Context) []string {
	providers := make([]string, 0, len(models.Providers))
	for _, provider := range models.Providers {
		key, err := s.LLMKey(ctx, provider)
		if err != nil {
			s.log.Warn("failed to read provider key", "provider", provider, "error", err)
			continue
		}
		if key != "" {
			providers = append(providers, provider)
		}
	}
	return providers
}

// DefaultProvider returns the name of the default summarizer provider. It
// reports the configured value even when that provider has no key, so the
// Settings page can show what is selected rather than silently nothing.
func (s *Service) DefaultProvider(ctx context.Context) string {
	provider, err := s.SummaryProvider(ctx)
	if err != nil {
		s.log.Warn("failed to read summary provider", "error", err)
		return s.summaryCfg.Provider
	}
	return provider
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

// RetryVideo resets a failed row and re-processes it. Podcast rows go back to
// the episode worker, not to the YouTube pipeline, which has no ID to work with.
func (s *Service) RetryVideo(ctx context.Context, userID int, id uuid.UUID) error {
	video, err := s.db.GetVideo(ctx, userID, id)
	if err != nil {
		return err
	}
	if video.Status != "failed" && video.Status != "no_captions" {
		return fmt.Errorf("video is not in failed state (status: %s)", video.Status)
	}
	if err := s.db.UpdateVideoStatus(ctx, id, "pending", ""); err != nil {
		return fmt.Errorf("reset status: %w", err)
	}
	if video.Source == storage.SourcePodcast {
		episodeID, convErr := strconv.Atoi(video.ExternalID)
		if convErr != nil {
			return fmt.Errorf("podcast row %s has a non-numeric episode ID %q", id, video.ExternalID)
		}
		s.ProcessEpisodeAsync(userID, episodeID, video.DetailLevel)
		return nil
	}
	s.ProcessVideoAsync(userID, video.YouTubeID, video.KarakeepBookmarkID)
	return nil
}

// TranscribeVideo queues a no_captions video for Voxtral audio transcription.
func (s *Service) TranscribeVideo(ctx context.Context, userID int, id uuid.UUID) error {
	video, err := s.db.GetVideo(ctx, userID, id)
	if err != nil {
		return err
	}
	if video.Source != storage.SourceYouTube {
		return fmt.Errorf("voxtral transcription applies to YouTube videos only (source: %s)", video.Source)
	}
	if video.Status != "no_captions" && video.Status != "failed" {
		return fmt.Errorf("video is not in no_captions or failed state (status: %s)", video.Status)
	}
	if err := s.db.UpdateVideoStatus(ctx, id, "pending", ""); err != nil {
		return fmt.Errorf("reset status: %w", err)
	}
	select {
	case s.queue <- processJob{userID: userID, youtubeID: video.YouTubeID, bookmarkID: video.KarakeepBookmarkID, forceVoxtral: true}:
	default:
		s.log.Warn("processing queue full, dropping transcribe job", "youtube_id", video.YouTubeID)
	}
	return nil
}

// TranscribeAllNoCaptions queues all no_captions videos for Voxtral audio transcription.
func (s *Service) TranscribeAllNoCaptions(ctx context.Context, userID int) (int, error) {
	videos, err := s.db.ListNoCaptionsVideos(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("list no_captions videos: %w", err)
	}
	for _, v := range videos {
		if err := s.db.UpdateVideoStatus(ctx, v.ID, "pending", ""); err != nil {
			s.log.Warn("failed to reset video status", "video_id", v.ID, "error", err)
			continue
		}
		select {
		case s.queue <- processJob{userID: userID, youtubeID: v.YouTubeID, bookmarkID: v.KarakeepBookmarkID, forceVoxtral: true}:
		default:
			s.log.Warn("processing queue full, dropping transcribe job", "youtube_id", v.YouTubeID)
		}
	}
	return len(videos), nil
}

// RetryAllFailed resets all failed videos for a user and re-enqueues them.
func (s *Service) RetryAllFailed(ctx context.Context, userID int) (int, error) {
	videos, err := s.db.ListFailedVideos(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("list failed videos: %w", err)
	}
	for _, v := range videos {
		if err := s.db.UpdateVideoStatus(ctx, v.ID, "pending", ""); err != nil {
			s.log.Warn("failed to reset video status", "video_id", v.ID, "error", err)
			continue
		}
		s.ProcessVideoAsync(userID, v.YouTubeID, v.KarakeepBookmarkID)
	}
	return len(videos), nil
}

// GetWebhookInfo returns the webhook URL and token for a user.
func (s *Service) GetWebhookInfo(ctx context.Context, userID int) (token string, err error) {
	token, err = s.db.GetOrCreateWebhookToken(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get webhook token: %w", err)
	}
	return token, nil
}

// GetFeedInfo returns the feed token for a user, generating one if needed.
func (s *Service) GetFeedInfo(ctx context.Context, userID int) (string, error) {
	token, err := s.db.GetOrCreateFeedToken(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get feed token: %w", err)
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

// getUserPrompt returns the user's custom prompt for one source and level, or
// an empty string when the built-in default applies.
func (s *Service) getUserPrompt(ctx context.Context, userID int, source, level string) string {
	if level != "deep" {
		level = "medium"
	}
	prompt, err := s.db.GetUserPrompt(ctx, userID, normalizeSource(source), level)
	if err != nil {
		s.log.Warn("failed to load custom prompt, using default",
			"user_id", userID, "source", source, "level", level, "error", err)
		return ""
	}
	return prompt
}

// normalizeSource maps an empty or unknown source onto youtube, so a caller
// that never learned about sources keeps working.
func normalizeSource(source string) string {
	if source == storage.SourcePodcast {
		return storage.SourcePodcast
	}
	return storage.SourceYouTube
}

// GetSummaryPrompts returns the user's current and default prompts for one source.
func (s *Service) GetSummaryPrompts(ctx context.Context, userID int, source string) (*SummaryPromptsInfo, error) {
	source = normalizeSource(source)
	prompts, err := s.db.GetUserPrompts(ctx, userID, source)
	if err != nil {
		return nil, fmt.Errorf("get prompts: %w", err)
	}

	info := &SummaryPromptsInfo{
		Source:        source,
		DefaultMedium: summary.DefaultPromptFor(source, "medium"),
		DefaultDeep:   summary.DefaultPromptFor(source, "deep"),
	}
	if p, ok := prompts["medium"]; ok {
		info.Medium = p
	} else {
		info.Medium = info.DefaultMedium
	}
	if p, ok := prompts["deep"]; ok {
		info.Deep = p
	} else {
		info.Deep = info.DefaultDeep
	}
	return info, nil
}

// SetSummaryPrompt sets a custom prompt for one source and level. An empty
// prompt resets to the default.
func (s *Service) SetSummaryPrompt(ctx context.Context, userID int, source, level, prompt string) error {
	if level != "medium" && level != "deep" {
		return fmt.Errorf("invalid level: %q (must be medium or deep)", level)
	}
	if source != "" && source != storage.SourceYouTube && source != storage.SourcePodcast {
		return fmt.Errorf("invalid source: %q (must be youtube or podcast)", source)
	}
	return s.db.SetUserPrompt(ctx, userID, normalizeSource(source), level, prompt)
}

// ListAllModels returns available models from all configured providers.
func (s *Service) ListAllModels(ctx context.Context) []models.Model {
	return s.registry.ListAllModels(ctx)
}

// GetModelPreference returns the user's preferred summary model.
func (s *Service) GetModelPreference(ctx context.Context, userID int) (provider, model string, err error) {
	return s.db.GetModelPreference(ctx, userID)
}

// SetModelPreference sets the user's preferred summary model. The provider is
// validated against the ones that have a key, so a preference cannot be stored
// for a provider that could never serve it.
func (s *Service) SetModelPreference(ctx context.Context, userID int, provider, model string) error {
	if provider != "" && !slices.Contains(s.AvailableProviders(ctx), provider) {
		return fmt.Errorf("invalid provider: %q", provider)
	}
	return s.db.SetModelPreference(ctx, userID, provider, model)
}

// getModelForProvider resolves the model to use: user preference → config fallback → empty (provider default).
func (s *Service) getModelForProvider(ctx context.Context, userID int, provider string) string {
	prefProvider, prefModel, err := s.db.GetModelPreference(ctx, userID)
	if err != nil {
		s.log.Warn("failed to load model preference, using config fallback", "user_id", userID, "provider", provider, "error", err)
	}
	// Use stored preference if it matches the active provider
	if prefProvider == provider && prefModel != "" {
		return prefModel
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

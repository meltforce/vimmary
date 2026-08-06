package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	mkserver "github.com/meltforce/meltkit/pkg/server"
	"github.com/meltforce/vimmary/internal/feed"
	"github.com/meltforce/vimmary/internal/karakeep"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
)

type Server struct {
	*mkserver.Server
	svc     *service.Service
	store   *storage.DB
	version string
	log     *slog.Logger
}

func New(svc *service.Service, store *storage.DB, version string, log *slog.Logger) *Server {
	s := &Server{
		Server:  mkserver.New(mkserver.WithLogger(log)),
		svc:     svc,
		store:   store,
		version: version,
		log:     log,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	r := s.Router()

	// Version route — no Tailscale auth, same as /healthz. It reports which
	// build is serving, which is what lets a deploy gate tell a real rollout
	// from a green pipeline over an unchanged image. meltkit owns /healthz and
	// answers a fixed "ok", so this is a route of its own rather than an
	// override.
	r.Get("/version", s.handleVersion)

	// Webhook route — no Tailscale auth, uses per-user Bearer token
	r.Post("/webhook/karakeep", karakeep.WebhookHandler(s.svc, s.store.GetUserByWebhookToken))

	// Feed routes — no Tailscale auth, token in URL path is the access control.
	// The bare path stays videos-only so existing subscriptions keep their
	// contents when podcast rows appear.
	r.Get("/feed/atom/{token}", feed.HandleVideoFeed(s.svc, s.store))
	r.Get("/feed/atom/{token}/podcasts", feed.HandlePodcastFeed(s.svc, s.store))
	r.Get("/feed/atom/{token}/all", feed.HandleCombinedFeed(s.svc, s.store))

	r.Group(func(r chi.Router) {
		r.Use(s.IdentityMiddleware())

		r.Get("/api/v1/videos", s.handleListVideos)
		r.Post("/api/v1/videos", s.handleSubmitVideo)
		r.Post("/api/v1/videos/backfill-metadata", s.handleBackfillMetadata)
		r.Post("/api/v1/videos/retry-all", s.handleRetryAllFailed)
		r.Post("/api/v1/videos/transcribe-all", s.handleTranscribeAll)
		r.Get("/api/v1/videos/{id}", s.handleGetVideo)
		r.Delete("/api/v1/videos/{id}", s.handleDeleteVideo)
		r.Post("/api/v1/videos/{id}/resummarize", s.handleResummarize)
		r.Post("/api/v1/videos/{id}/retry", s.handleRetryVideo)
		r.Post("/api/v1/videos/{id}/transcribe", s.handleTranscribeVideo)
		r.Get("/api/v1/config/features", s.handleGetFeatures)
		r.Get("/api/v1/config/providers", s.handleGetProviders)
		r.Get("/api/v1/config/models", s.handleListModels)
		r.Get("/api/v1/search", s.handleSearch)
		r.Get("/api/v1/stats", s.handleStats)

		// Podcasts
		r.Get("/api/v1/podcasts/feeds", s.handleListPodcastFeeds)
		r.Put("/api/v1/podcasts/feeds/{feedID}", s.handleSetPodcastSubscription)
		r.Post("/api/v1/podcasts/feeds/{feedID}/backfill", s.handleBackfillPodcastFeed)
		// Whole-feed actions. summarize-all spends LLM calls here;
		// transcribe-all queues download and Whisper work in cast2md.
		r.Post("/api/v1/podcasts/feeds/{feedID}/summarize-all", s.handleSummarizeAllPodcastFeed)
		r.Post("/api/v1/podcasts/feeds/{feedID}/transcribe-all", s.handleTranscribeAllPodcastFeed)
		r.Get("/api/v1/podcasts/episodes/{episodeID}", s.handleGetEpisodePreview)
		r.Post("/api/v1/podcasts/episodes", s.handleSubmitEpisode)

		// Settings
		r.Get("/api/v1/settings/feed", s.handleGetFeed)
		r.Get("/api/v1/settings/webhook", s.handleGetWebhook)
		r.Get("/api/v1/settings/karakeep", s.handleGetKarakeepStatus)
		r.Put("/api/v1/settings/karakeep", s.handleSetKarakeepKey)
		r.Post("/api/v1/settings/karakeep/import", s.handleImportKarakeep)
		r.Get("/api/v1/settings/models", s.handleGetModelPreferences)
		r.Put("/api/v1/settings/model", s.handleSetModel)
		r.Get("/api/v1/settings/prompts", s.handleGetPrompts)
		r.Put("/api/v1/settings/prompts", s.handleSetPrompt)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Server.ServeHTTP(w, r)
}

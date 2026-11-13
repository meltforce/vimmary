package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/meltkit/pkg/middleware"
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
	// auth is nil without an `auth:` block; the identity group then uses
	// Tailscale WhoIs alone, as before application login existed.
	auth *auth.Auth
	// tailnet is set by SetTailscale. The WhoIs fallback behind a missing
	// session exists only then: without it meltkit's identity middleware
	// would hand every request DevIdentity (user 1).
	tailnet bool
	// isDisabled answers refuseDisabled; a field so a test can replace it.
	isDisabled func(ctx context.Context, userID int) (bool, error)
}

// newBase builds the meltkit server, whose /healthz answers
// {"status":"ok","sha":"<sha>"} (homelab STANDARDS.md § Image registry &
// tagging).
func newBase(sha string, log *slog.Logger) *mkserver.Server {
	return mkserver.New(mkserver.WithLogger(log), mkserver.WithBuildSHA(sha))
}

// New builds the router. sha is the commit /healthz reports. authn is nil
// when login is not configured.
func New(svc *service.Service, store *storage.DB, version, sha string, log *slog.Logger, authn *auth.Auth) *Server {
	s := &Server{
		Server:  newBase(sha, log),
		svc:     svc,
		store:   store,
		version: version,
		log:     log,
		auth:    authn,
	}
	s.isDisabled = svc.Accounts().IsDisabled
	s.routes()
	return s
}

// SetTailscale enables WhoIs identity. Call it before serving.
func (s *Server) SetTailscale(lc middleware.WhoisClient, us middleware.UserStore) {
	s.Server.SetTailscale(lc, us)
	s.tailnet = true
}

func (s *Server) routes() {
	r := s.Router()

	// Version route — no Tailscale auth, same as /healthz. It reports which
	// build is serving and whether the database answers; pipeline-deploy waits
	// on it. meltkit owns /healthz, which reports status and the build SHA
	// without touching the database, so this is a route of its own.
	r.Get("/version", s.handleVersion)

	// Webhook route — no Tailscale auth, uses per-user Bearer token
	r.Post("/webhook/karakeep", karakeep.WebhookHandler(s.svc, s.store.GetUserByWebhookToken))

	// Feed routes — no Tailscale auth, token in URL path is the access control.
	// The bare path stays videos-only so existing subscriptions keep their
	// contents when podcast rows appear.
	r.Get("/feed/atom/{token}", feed.HandleVideoFeed(s.svc, s.store))
	r.Get("/feed/atom/{token}/podcasts", feed.HandlePodcastFeed(s.svc, s.store))
	r.Get("/feed/atom/{token}/all", feed.HandleCombinedFeed(s.svc, s.store))

	// Login endpoints of pkg/auth. Without an auth block they answer 404, so
	// the login page can tell that login is not configured.
	if s.auth != nil {
		r.Handle("/auth/*", s.auth.Handler())
	} else {
		r.Handle("/auth/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "login is not configured"})
		}))
	}

	// Share-sheet target. Not under /api/v1 because it answers a redirect for
	// a browser tab rather than JSON for a client, which is also why a missing
	// login redirects to the login page instead of answering 401.
	r.With(s.identity(http.HandlerFunc(redirectToLogin))).Get("/submit", s.handleShareSubmit)

	r.Group(func(r chi.Router) {
		r.Use(s.identity(http.HandlerFunc(unauthenticated)))

		if s.auth != nil {
			s.accountRoutes(r)
		}

		r.Get("/api/v1/videos", s.handleListVideos)
		r.Get("/api/v1/videos/facets", s.handleVideoFacets)
		r.Post("/api/v1/videos", s.handleSubmitVideo)
		r.Post("/api/v1/videos/backfill-metadata", s.handleBackfillMetadata)
		r.Post("/api/v1/videos/retry-all", s.handleRetryAllFailed)
		r.Post("/api/v1/videos/transcribe-all", s.handleTranscribeAll)
		r.Post("/api/v1/videos/consolidate-topics", s.handleConsolidateTopics)
		r.Get("/api/v1/videos/{id}", s.handleGetVideo)
		r.Get("/api/v1/videos/{id}/segments", s.handleGetVideoSegments)
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

		// Channels inbox
		r.Get("/api/v1/channels", s.handleListChannels)
		r.Post("/api/v1/channels", s.handleSubscribeChannel)
		r.Post("/api/v1/channels/import", s.handleImportChannels)
		r.Put("/api/v1/channels/{id}", s.handleSetChannelEnabled)
		r.Delete("/api/v1/channels/{id}", s.handleDeleteChannel)
		r.Get("/api/v1/inbox", s.handleListInbox)
		r.Post("/api/v1/inbox/dismiss-all", s.handleDismissAllInbox)
		r.Post("/api/v1/inbox/{id}/summarize", s.handleSummarizeInboxItem)
		r.Post("/api/v1/inbox/{id}/dismiss", s.handleDismissInboxItem)

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

		// Service-wide, primary user only. These are the LLM API keys and the
		// summary provider, which used to come from setec at startup.
		r.Get("/api/v1/settings/llm", s.handleGetLLMSettings)
		r.Put("/api/v1/settings/llm", s.handleSetLLMSettings)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Server.ServeHTTP(w, r)
}

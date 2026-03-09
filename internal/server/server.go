package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meltforce/vimmary/internal/karakeep"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
	mkserver "github.com/meltforce/meltkit/pkg/server"
)

type Server struct {
	*mkserver.Server
	svc   *service.Service
	store *storage.DB
	log   *slog.Logger
}

func New(svc *service.Service, store *storage.DB, log *slog.Logger) *Server {
	s := &Server{
		Server: mkserver.New(mkserver.WithLogger(log)),
		svc:    svc,
		store:  store,
		log:    log,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	r := s.Router()

	// Webhook route — no Tailscale auth, uses per-user Bearer token
	r.Post("/webhook/karakeep", karakeep.WebhookHandler(s.svc, s.store.GetUserByWebhookToken))

	r.Group(func(r chi.Router) {
		r.Use(s.IdentityMiddleware())

		r.Get("/api/v1/videos", s.handleListVideos)
		r.Post("/api/v1/videos", s.handleSubmitVideo)
		r.Get("/api/v1/videos/{id}", s.handleGetVideo)
		r.Delete("/api/v1/videos/{id}", s.handleDeleteVideo)
		r.Post("/api/v1/videos/{id}/resummarize", s.handleResummarize)
		r.Post("/api/v1/videos/{id}/retry", s.handleRetryVideo)
		r.Get("/api/v1/search", s.handleSearch)
		r.Get("/api/v1/stats", s.handleStats)

		// Settings
		r.Get("/api/v1/settings/webhook", s.handleGetWebhook)
		r.Get("/api/v1/settings/karakeep", s.handleGetKarakeepStatus)
		r.Put("/api/v1/settings/karakeep", s.handleSetKarakeepKey)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Server.ServeHTTP(w, r)
}

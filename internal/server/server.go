package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/meltforce/vimmary/internal/karakeep"
	"github.com/meltforce/vimmary/internal/service"
	mkserver "github.com/meltforce/meltkit/pkg/server"
)

type Server struct {
	*mkserver.Server
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service, webhookToken string, log *slog.Logger) *Server {
	s := &Server{
		Server: mkserver.New(mkserver.WithLogger(log)),
		svc:    svc,
		log:    log,
	}
	s.routes(webhookToken)
	return s
}

func (s *Server) routes(webhookToken string) {
	r := s.Router()

	// Webhook route — no Tailscale auth, uses its own Bearer token
	r.Post("/webhook/karakeep", karakeep.WebhookHandler(s.svc, webhookToken))

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
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Server.ServeHTTP(w, r)
}

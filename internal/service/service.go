package service

import (
	"context"
	"log/slog"

	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/karakeep"
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
	db         *storage.DB
	summarizer summary.Summarizer
	yt         *youtube.Client
	karakeep   *karakeep.Client
	embedder   Embedder
	searchCfg  config.SearchConfig
	summaryCfg config.SummaryConfig
	log        *slog.Logger
}

// New creates a new Service.
func New(
	db *storage.DB,
	summarizer summary.Summarizer,
	yt *youtube.Client,
	kk *karakeep.Client,
	embedder Embedder,
	searchCfg config.SearchConfig,
	summaryCfg config.SummaryConfig,
	log *slog.Logger,
) *Service {
	return &Service{
		db:         db,
		summarizer: summarizer,
		yt:         yt,
		karakeep:   kk,
		embedder:   embedder,
		searchCfg:  searchCfg,
		summaryCfg: summaryCfg,
		log:        log,
	}
}

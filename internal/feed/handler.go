package feed

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
)

// The three feeds a user can subscribe to. The video feed keeps the original
// path and its videos-only contents, so existing subscriptions are unaffected
// by podcast rows appearing.
var (
	videoFeed = FeedOptions{
		Source:   storage.SourceYouTube,
		Slug:     "videos",
		Title:    "vimmary — Video Summaries",
		Subtitle: "AI-generated summaries of YouTube videos",
	}
	podcastFeed = FeedOptions{
		Source:   storage.SourcePodcast,
		Slug:     "podcasts",
		Title:    "vimmary — Podcast Summaries",
		Subtitle: "AI-generated summaries of podcast episodes",
	}
	combinedFeed = FeedOptions{
		Source:   "",
		Slug:     "all",
		Title:    "vimmary — Summaries",
		Subtitle: "AI-generated summaries of videos and podcast episodes",
	}
)

// HandleVideoFeed serves the YouTube-only Atom feed.
func HandleVideoFeed(svc *service.Service, store *storage.DB) http.HandlerFunc {
	return handleFeed(svc, store, videoFeed)
}

// HandlePodcastFeed serves the podcast-only Atom feed.
func HandlePodcastFeed(svc *service.Service, store *storage.DB) http.HandlerFunc {
	return handleFeed(svc, store, podcastFeed)
}

// HandleCombinedFeed serves both kinds in one Atom feed. Entries carry their
// type as the first category.
func HandleCombinedFeed(svc *service.Service, store *storage.DB) http.HandlerFunc {
	return handleFeed(svc, store, combinedFeed)
}

func handleFeed(svc *service.Service, store *storage.DB, opts FeedOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if token == "" {
			http.NotFound(w, r)
			return
		}

		userID, err := store.GetUserByFeedToken(r.Context(), token)
		if err != nil {
			if err == pgx.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		limit := 50
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
			limit = l
		}
		if limit > 200 {
			limit = 200
		}

		filters := storage.ListFilters{Status: "completed", Source: opts.Source}
		videos, _, err := svc.ListRecent(r.Context(), userID, filters, limit, 0)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		scheme := "https"
		baseURL := scheme + "://" + r.Host

		data, err := BuildFeed(videos, baseURL, opts)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		_, _ = w.Write(data)
	}
}

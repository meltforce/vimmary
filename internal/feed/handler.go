package feed

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
)

// HandleAtomFeed returns an HTTP handler that serves an Atom feed for a user identified by feed token.
func HandleAtomFeed(svc *service.Service, store *storage.DB) http.HandlerFunc {
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

		filters := storage.ListFilters{Status: "completed"}
		videos, _, err := svc.ListRecent(r.Context(), userID, filters, limit, 0)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		scheme := "https"
		baseURL := scheme + "://" + r.Host

		data, err := BuildFeed(videos, baseURL)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		_, _ = w.Write(data)
	}
}

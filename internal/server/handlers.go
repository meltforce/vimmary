package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meltforce/meltkit/pkg/middleware"
	"github.com/meltforce/vimmary/internal/storage"
)

func mustUserID(w http.ResponseWriter, r *http.Request) (int, bool) {
	uid, ok := middleware.UserIDFromContext(r)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no authenticated user"})
		return 0, false
	}
	return uid, true
}

func (s *Server) handleListVideos(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	filters := storage.ListFilters{
		Channel:  q.Get("channel"),
		Language: q.Get("language"),
		Topic:    q.Get("topic"),
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	videos, total, err := s.svc.ListRecent(r.Context(), uid, filters, limit, offset)
	if err != nil {
		s.log.Error("list failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":  total,
		"count":  len(videos),
		"videos": videos,
	})
}

func (s *Server) handleGetVideo(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video ID"})
		return
	}

	video, err := s.svc.GetVideo(r.Context(), uid, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("get video failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get video failed"})
		return
	}

	writeJSON(w, http.StatusOK, video)
}

func (s *Server) handleResummarize(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video ID"})
		return
	}

	var body struct {
		Level string `json:"level"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body.Level == "" {
		body.Level = "deep"
	}

	if err := s.svc.Resummarize(r.Context(), uid, id, body.Level); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("resummarize failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resummarize failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "resummarized successfully", "level": body.Level})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q parameter is required"})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	matches, warnings, err := s.svc.Search(r.Context(), uid, query, limit)
	if err != nil {
		s.log.Error("search failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search failed"})
		return
	}

	resp := map[string]any{
		"count":   len(matches),
		"results": matches,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	stats, err := s.svc.Stats(r.Context(), uid)
	if err != nil {
		s.log.Error("stats failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stats failed"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

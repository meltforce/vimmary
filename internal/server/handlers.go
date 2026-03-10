package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meltforce/meltkit/pkg/middleware"
	"github.com/meltforce/vimmary/internal/karakeep"
	"github.com/meltforce/vimmary/internal/service"
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
		Status:   q.Get("status"),
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	videos, total, err := s.svc.ListRecent(r.Context(), uid, filters, limit, offset)
	if err != nil {
		s.log.Error("list failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
		return
	}

	if videos == nil {
		videos = []storage.Video{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":  total,
		"count":  len(videos),
		"videos": videos,
	})
}

func (s *Server) handleSubmitVideo(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	youtubeID := karakeep.ExtractYouTubeID(body.URL)
	if youtubeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid YouTube URL"})
		return
	}

	s.svc.ProcessVideoAsync(uid, youtubeID, "")
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "youtube_id": youtubeID})
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
		Level    string `json:"level"`
		Language string `json:"language"`
		Provider string `json:"provider"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body.Level == "" {
		body.Level = "deep"
	}

	if err := s.svc.ResummarizeAsync(uid, id, body.Level, body.Language, body.Provider); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("resummarize failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resummarize failed"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "processing", "level": body.Level})
}

func (s *Server) handleRetryVideo(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video ID"})
		return
	}

	if err := s.svc.RetryVideo(r.Context(), uid, id); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("retry failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "retrying"})
}

func (s *Server) handleDeleteVideo(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video ID"})
		return
	}

	if err := s.svc.DeleteVideo(r.Context(), uid, id); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("delete video failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRetryAllFailed(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	count, err := s.svc.RetryAllFailed(r.Context(), uid)
	if err != nil {
		s.log.Error("retry all failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "retry all failed"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]int{"retried": count})
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

	if matches == nil {
		matches = []service.HybridMatch{}
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

func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	token, err := s.svc.GetWebhookInfo(r.Context(), uid)
	if err != nil {
		s.log.Error("get webhook info failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get webhook info"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

func (s *Server) handleGetKarakeepStatus(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	hasKey, err := s.svc.HasKarakeepAPIKey(r.Context(), uid)
	if err != nil {
		s.log.Error("get karakeep status failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get status"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"configured": hasKey,
		"base_url":   s.svc.KarakeepBaseURL(),
	})
}

func (s *Server) handleImportKarakeep(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	result, err := s.svc.ImportKarakeepBookmarks(r.Context(), uid)
	if err != nil {
		s.log.Error("karakeep import failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleSetKarakeepKey(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "api_key is required"})
		return
	}

	if err := s.svc.SetKarakeepAPIKey(r.Context(), uid, body.APIKey); err != nil {
		s.log.Error("set karakeep key failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save key"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleGetPrompts(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	info, err := s.svc.GetSummaryPrompts(r.Context(), uid)
	if err != nil {
		s.log.Error("get prompts failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get prompts"})
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleSetPrompt(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		Level  string `json:"level"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Level == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level is required"})
		return
	}

	if err := s.svc.SetSummaryPrompt(r.Context(), uid, body.Level, body.Prompt); err != nil {
		s.log.Error("set prompt failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	prefProvider, prefModel, _ := s.svc.GetModelPreference(r.Context(), uid)

	writeJSON(w, http.StatusOK, map[string]any{
		"providers": s.svc.AvailableProviders(),
		"default":   s.svc.DefaultProvider(),
		"selected_provider": prefProvider,
		"selected_model":    prefModel,
	})
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	allModels := s.svc.ListAllModels(r.Context())
	prefProvider, prefModel, _ := s.svc.GetModelPreference(r.Context(), uid)

	writeJSON(w, http.StatusOK, map[string]any{
		"models":            allModels,
		"selected_provider": prefProvider,
		"selected_model":    prefModel,
	})
}

func (s *Server) handleGetModelPreferences(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	provider, model, err := s.svc.GetModelPreference(r.Context(), uid)
	if err != nil {
		s.log.Error("get model preferences failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get model preferences"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"provider": provider,
		"model":    model,
	})
}

func (s *Server) handleSetModel(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := s.svc.SetModelPreference(r.Context(), uid, body.Provider, body.Model); err != nil {
		s.log.Error("set model failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/meltforce/meltkit/pkg/middleware"
	"github.com/meltforce/vimmary/internal/karakeep"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
)

// sourceParam resolves the `source` query parameter. An empty value takes the
// given default; "all" means no filter and yields an empty string.
func sourceParam(value, fallback string) string {
	switch value {
	case "":
		return fallback
	case "all":
		return ""
	case storage.SourceYouTube, storage.SourcePodcast:
		return value
	default:
		return fallback
	}
}

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
		// The source default is youtube, not "everything". That is what keeps
		// the videos page, MCP list_recent and every older client video-only
		// once podcast rows exist. "all" opts out.
		Source: sourceParam(q.Get("source"), storage.SourceYouTube),
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
		if errors.Is(err, storage.ErrNotFound) {
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
		if errors.Is(err, storage.ErrNotFound) {
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
		if errors.Is(err, storage.ErrNotFound) {
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
		if errors.Is(err, storage.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("delete video failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTranscribeVideo(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid video ID"})
		return
	}

	if err := s.svc.TranscribeVideo(r.Context(), uid, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
			return
		}
		s.log.Error("transcribe failed", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "transcribing"})
}

func (s *Server) handleTranscribeAll(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	count, err := s.svc.TranscribeAllNoCaptions(r.Context(), uid)
	if err != nil {
		s.log.Error("transcribe all failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transcribe all failed"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]int{"transcribing": count})
}

func (s *Server) handleBackfillMetadata(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	count, err := s.svc.BackfillMetadata(r.Context(), uid)
	if err != nil {
		s.log.Error("backfill metadata failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "backfill metadata failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"updated": count})
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
	// Search defaults to both kinds; the results carry `source` so the caller
	// can tell them apart.
	source := sourceParam(r.URL.Query().Get("source"), "")

	matches, warnings, err := s.svc.Search(r.Context(), uid, query, limit, source)
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

	stats, err := s.svc.Stats(r.Context(), uid, sourceParam(r.URL.Query().Get("source"), ""))
	if err != nil {
		s.log.Error("stats failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stats failed"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleGetFeed(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	token, err := s.svc.GetFeedInfo(r.Context(), uid)
	if err != nil {
		s.log.Error("get feed info failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get feed info"})
		return
	}

	base := "https://" + r.Host + "/feed/atom/" + token
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"urls": map[string]string{
			"videos":   base,
			"podcasts": base + "/podcasts",
			"all":      base + "/all",
		},
	})
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

// mustAdmin resolves the caller and requires that they are the primary user.
// It answers 404 rather than 403 for a non-admin, so the response does not
// confirm that an admin-only surface exists — the same reasoning as the Atom
// feed token in server.go.
func (s *Server) mustAdmin(w http.ResponseWriter, r *http.Request) (int, bool) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return 0, false
	}
	admin, err := s.svc.IsAdmin(r.Context(), uid)
	if err != nil {
		s.log.Error("admin check failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check permissions"})
		return 0, false
	}
	if !admin {
		http.NotFound(w, r)
		return 0, false
	}
	return uid, true
}

func (s *Server) handleGetLLMSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}

	settings, err := s.svc.GetLLMSettings(r.Context())
	if err != nil {
		s.log.Error("get llm settings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get settings"})
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// handleSetLLMSettings updates the service-wide LLM configuration. Every field
// is a pointer, because absent and empty mean different things here: absent
// leaves the value alone, empty clears it. The Anthropic key is meant to be
// clearable, which is why this does not reject empty the way the Karakeep
// handler does.
//
// Keys are applied before the provider, so one request can supply a key and
// select the provider it belongs to.
func (s *Server) handleSetLLMSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}

	var body struct {
		MistralAPIKey   *string `json:"mistral_api_key"`
		AnthropicAPIKey *string `json:"anthropic_api_key"`
		Provider        *string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if body.MistralAPIKey != nil {
		if err := s.svc.SetLLMKey(r.Context(), "mistral", strings.TrimSpace(*body.MistralAPIKey)); err != nil {
			s.log.Error("set mistral key failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save key"})
			return
		}
	}
	if body.AnthropicAPIKey != nil {
		if err := s.svc.SetLLMKey(r.Context(), "claude", strings.TrimSpace(*body.AnthropicAPIKey)); err != nil {
			s.log.Error("set anthropic key failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save key"})
			return
		}
	}
	if body.Provider != nil {
		if err := s.svc.SetSummaryProvider(r.Context(), strings.TrimSpace(*body.Provider)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	settings, err := s.svc.GetLLMSettings(r.Context())
	if err != nil {
		s.log.Error("get llm settings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "saved, but failed to read back"})
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleGetPrompts(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	info, err := s.svc.GetSummaryPrompts(r.Context(), uid,
		sourceParam(r.URL.Query().Get("source"), storage.SourceYouTube))
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
		Source string `json:"source"`
		Level  string `json:"level"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Level == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level is required"})
		return
	}
	if body.Source == "" {
		body.Source = r.URL.Query().Get("source")
	}
	body.Source = sourceParam(body.Source, storage.SourceYouTube)

	if err := s.svc.SetSummaryPrompt(r.Context(), uid, body.Source, body.Level, body.Prompt); err != nil {
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
		"providers":         s.svc.AvailableProviders(r.Context()),
		"default":           s.svc.DefaultProvider(r.Context()),
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

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/meltforce/vimmary/internal/service"
)

// writePodcastError maps the service's typed podcast errors onto status codes.
func (s *Server) writePodcastError(w http.ResponseWriter, err error, logMsg string) {
	switch {
	case errors.Is(err, service.ErrPodcastDisabled):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case service.IsEpisodeNotReady(err):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		s.log.Error(logMsg, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
}

// handleGetFeatures reports which optional integrations this deployment has.
// The frontend hides everything belonging to an integration that is off, so an
// installation without cast2md shows no trace of podcasts anywhere.
func (s *Server) handleGetFeatures(w http.ResponseWriter, r *http.Request) {
	if _, ok := mustUserID(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"podcasts":    s.svc.PodcastEnabled(),
		"cast2md_url": s.svc.Cast2MDBaseURL(),
	})
}

func (s *Server) handleListPodcastFeeds(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	feeds, err := s.svc.ListPodcastFeeds(r.Context(), uid)
	if err != nil {
		s.writePodcastError(w, err, "list podcast feeds failed")
		return
	}
	if feeds == nil {
		feeds = []service.PodcastFeed{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":       len(feeds),
		"feeds":       feeds,
		"cast2md_url": s.svc.Cast2MDBaseURL(),
	})
}

func (s *Server) handleSetPodcastSubscription(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	feedID := chi.URLParam(r, "feedID")
	if feedID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feed ID is required"})
		return
	}

	// InitialBackfill is a pointer so that omitting it keeps the stored value
	// rather than silently resetting the feed to zero.
	var body struct {
		Enabled         bool   `json:"enabled"`
		DetailLevel     string `json:"detail_level"`
		InitialBackfill *int   `json:"initial_backfill"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	initialBackfill := service.DefaultInitialBackfill
	if body.InitialBackfill != nil {
		initialBackfill = *body.InitialBackfill
	} else if existing, err := s.svc.GetPodcastSubscription(r.Context(), uid, feedID); err == nil {
		initialBackfill = existing.InitialBackfill
	}

	sub, err := s.svc.SetPodcastSubscription(r.Context(), uid, feedID, body.Enabled, body.DetailLevel, initialBackfill)
	if err != nil {
		s.writePodcastError(w, err, "set podcast subscription failed")
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

func (s *Server) handleBackfillPodcastFeed(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	feedID := chi.URLParam(r, "feedID")
	if feedID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feed ID is required"})
		return
	}

	var body struct {
		Limit int `json:"limit"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	result, err := s.svc.BackfillFeed(r.Context(), uid, feedID, body.Limit)
	if err != nil {
		s.writePodcastError(w, err, "podcast backfill failed")
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// handleSummarizeAllPodcastFeed summarizes every episode cast2md already has a
// transcript for. Only LLM calls — cast2md is read, not driven.
func (s *Server) handleSummarizeAllPodcastFeed(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	feedID := chi.URLParam(r, "feedID")
	if feedID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feed ID is required"})
		return
	}

	result, err := s.svc.SummarizeAllCompleted(r.Context(), uid, feedID)
	if err != nil {
		s.writePodcastError(w, err, "summarize all failed")
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// handleTranscribeAllPodcastFeed asks cast2md to transcribe the rest of a feed.
// This queues download and Whisper work over there, not summaries here.
func (s *Server) handleTranscribeAllPodcastFeed(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	feedID := chi.URLParam(r, "feedID")
	if feedID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feed ID is required"})
		return
	}

	result, err := s.svc.TranscribeAllInFeed(r.Context(), uid, feedID)
	if err != nil {
		s.writePodcastError(w, err, "transcribe all failed")
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleGetEpisodePreview(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	episodeID, err := strconv.Atoi(chi.URLParam(r, "episodeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid episode ID"})
		return
	}

	preview, err := s.svc.GetEpisodePreview(r.Context(), uid, episodeID)
	if err != nil {
		s.writePodcastError(w, err, "episode preview failed")
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleSubmitEpisode(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		EpisodeID int    `json:"episode_id"`
		Level     string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EpisodeID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "episode_id is required"})
		return
	}

	// The row is created before this returns, so the frontend can navigate to
	// it right away rather than polling for it to appear.
	video, err := s.svc.SubmitEpisode(r.Context(), uid, body.EpisodeID, body.Level)
	if err != nil {
		s.writePodcastError(w, err, "submit episode failed")
		return
	}

	writeJSON(w, http.StatusAccepted, video)
}

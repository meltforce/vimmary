package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/meltforce/vimmary/internal/storage"
)

// idParam parses a numeric path parameter, answering 400 itself on failure.
func idParam(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, name))
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + name})
		return 0, false
	}
	return id, true
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	channels, err := s.svc.ListChannels(r.Context(), uid)
	if err != nil {
		s.log.Error("list channels failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list channels failed"})
		return
	}
	if channels == nil {
		channels = []storage.ChannelSubscription{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(channels),
		"channels": channels,
	})
}

func (s *Server) handleSubscribeChannel(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	sub, err := s.svc.SubscribeChannel(r.Context(), uid, body.URL)
	if err != nil {
		// A resolution failure is the user's input, not a server fault: the
		// message names what could not be resolved.
		if strings.Contains(err.Error(), "resolve channel") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.log.Error("subscribe channel failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "subscribe channel failed"})
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) handleSetChannelEnabled(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	if err := s.svc.SetChannelEnabled(r.Context(), uid, id, body.Enabled); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
			return
		}
		s.log.Error("set channel enabled failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "set channel enabled failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}

	if err := s.svc.DeleteChannel(r.Context(), uid, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
			return
		}
		s.log.Error("delete channel failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete channel failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListInbox(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	subscriptionID, _ := strconv.Atoi(q.Get("subscription_id"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	items, total, err := s.svc.ListInbox(r.Context(), uid, q.Get("state"), subscriptionID, limit, offset)
	if err != nil {
		s.log.Error("list inbox failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list inbox failed"})
		return
	}
	if items == nil {
		items = []storage.InboxItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total,
		"count": len(items),
		"items": items,
	})
}

func (s *Server) handleSummarizeInboxItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}

	video, err := s.svc.SummarizeInboxItem(r.Context(), uid, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "inbox item not found"})
			return
		}
		s.log.Error("summarize inbox item failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "summarize inbox item failed"})
		return
	}
	// 202: the row exists and is returned for navigation, the summary is
	// still in the queue.
	writeJSON(w, http.StatusAccepted, video)
}

func (s *Server) handleDismissInboxItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}

	if err := s.svc.DismissInboxItem(r.Context(), uid, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "inbox item not found"})
			return
		}
		s.log.Error("dismiss inbox item failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dismiss inbox item failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

func (s *Server) handleDismissAllInbox(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}

	dismissed, err := s.svc.DismissAllInbox(r.Context(), uid)
	if err != nil {
		s.log.Error("dismiss all inbox failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dismiss all failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dismissed": dismissed})
}

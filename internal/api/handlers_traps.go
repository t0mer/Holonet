package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/t0mer/holonet/internal/notify"
)

// ---- Traps ----

func (s *Server) listTraps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	traps, err := s.deps.Store.ListTraps(q.Get("sort"), q.Get("order"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, traps)
}

func (s *Server) getTrap(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	t, err := s.deps.Store.GetTrap(id)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) listTrapNotifications(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	n, err := s.deps.Store.ListNotificationsForTrap(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) replayTrap(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if s.deps.Replay == nil {
		writeErr(w, http.StatusNotImplemented, "replay not available")
		return
	}
	newID, err := s.deps.Replay(r.Context(), id)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"trap_id": newID})
}

// ---- Channel test ----

func testMessage() notify.Message {
	return notify.Message{
		Title:    "HoloNet test notification",
		Body:     "If you can read this, the channel is configured correctly.",
		Severity: "Info",
		Emoji:    "⚪",
	}
}

// testChannel sends a real test message using a saved channel's sealed config.
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	ch, err := s.deps.Store.GetChannel(id)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	configJSON, err := s.deps.Sealer.OpenString(ch.ConfigSealed)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unseal channel config")
		return
	}
	s.sendTest(w, r, ch.Kind, configJSON)
}

// testUnsaved sends a test using values the operator has entered but not saved.
func (s *Server) testUnsaved(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   string          `json:"kind"`
		Config json.RawMessage `json:"config"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	s.sendTest(w, r, body.Kind, string(body.Config))
}

func (s *Server) sendTest(w http.ResponseWriter, r *http.Request, kind, configJSON string) {
	notifier, err := notify.BuildNotifier(kind, configJSON)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := notifier.Send(ctx, testMessage()); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// ---- Sinks: v2c communities ----

type communityRequest struct {
	Community string `json:"community"`
	Enabled   bool   `json:"enabled"`
}

func (s *Server) listCommunities(w http.ResponseWriter, r *http.Request) {
	c, err := s.deps.Store.ListCommunities()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c) // Community.CommunitySealed is json:"-"
}

func (s *Server) createCommunity(w http.ResponseWriter, r *http.Request) {
	var req communityRequest
	if err := decodeJSON(r, &req); err != nil || req.Community == "" {
		writeErr(w, http.StatusBadRequest, "community is required")
		return
	}
	sealed, err := s.deps.Sealer.SealString(req.Community)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "seal community")
		return
	}
	id, err := s.deps.Store.AddCommunity(sealed, req.Enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "enabled": req.Enabled})
}

func (s *Server) updateCommunity(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.deps.Store.SetCommunityEnabled(id, body.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": body.Enabled})
}

func (s *Server) deleteCommunity(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteCommunity(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

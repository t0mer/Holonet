package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/holonet/internal/store"
)

// mountResources wires the authenticated /api/v1 resource routes.
func (s *Server) mountResources(r chi.Router) {
	// Settings
	r.Get("/settings", s.listSettings)
	r.Put("/settings", s.updateSettings)
	r.Get("/dashboard", s.dashboard)

	// Severities
	r.Get("/severities", s.listSeverities)
	r.Post("/severities", s.createSeverity)
	r.Put("/severities/{id}", s.updateSeverity)
	r.Delete("/severities/{id}", s.deleteSeverity)

	// OID map
	r.Get("/oidmap", s.listOIDMap)
	r.Post("/oidmap", s.createOID)
	r.Put("/oidmap/{id}", s.updateOID)
	r.Delete("/oidmap/{id}", s.deleteOID)

	// Devices
	r.Get("/devices", s.listDevices)
	r.Post("/devices", s.createDevice)
	r.Put("/devices/{id}", s.updateDevice)
	r.Delete("/devices/{id}", s.deleteDevice)

	// Channels
	r.Get("/channels", s.listChannels)
	r.Post("/channels", s.createChannel)
	r.Put("/channels/{id}", s.updateChannel)
	r.Delete("/channels/{id}", s.deleteChannel)

	// Rules
	r.Get("/rules", s.listRules)
	r.Post("/rules", s.createRule)
	r.Put("/rules/reorder", s.reorderRules)
	r.Post("/rules/test", s.testRule)
	r.Put("/rules/{id}", s.updateRule)
	r.Delete("/rules/{id}", s.deleteRule)

	// Default routes
	r.Get("/routes", s.listRoutes)
	r.Put("/routes/{severityID}", s.setRoutes)

	// Sinks (v2c communities)
	r.Get("/sinks/communities", s.listCommunities)
	r.Post("/sinks/communities", s.createCommunity)
	r.Put("/sinks/communities/{id}", s.updateCommunity)
	r.Delete("/sinks/communities/{id}", s.deleteCommunity)

	// Sinks (SNMPv3 users)
	r.Get("/sinks/v3users", s.listV3Users)
	r.Post("/sinks/v3users", s.createV3User)
	r.Put("/sinks/v3users/{id}", s.updateV3User)
	r.Delete("/sinks/v3users/{id}", s.deleteV3User)

	// Traps + notifications + actions
	r.Get("/traps", s.listTraps)
	r.Get("/traps/{id}", s.getTrap)
	r.Get("/traps/{id}/notifications", s.listTrapNotifications)
	r.Post("/replay/{id}", s.replayTrap)
	r.Post("/test/{id}", s.testChannel)
	r.Post("/test", s.testUnsaved)
}

// ---- Settings ----

func (s *Server) listSettings(w http.ResponseWriter, r *http.Request) {
	m, err := s.deps.Store.AllSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// writableSettings is the allow-list of keys the settings API may change. This
// keeps arbitrary keys out of the store; disabling auth via auth.enabled is a
// documented, auth-gated feature (design §5), not an unbounded write.
var writableSettings = map[string]bool{
	"snmp.bind_addr":              true,
	"flood.strategy":              true,
	"flood.dedupe_window":         true,
	"flood.rate_n":                true,
	"flood.rate_window":           true,
	"flood.digest_interval":       true,
	"unknown_default_severity_id": true,
	"auth.enabled":                true,
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var kv map[string]string
	if err := decodeJSON(r, &kv); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for k := range kv {
		if !writableSettings[k] {
			writeErr(w, http.StatusBadRequest, "unknown setting key: "+k)
			return
		}
	}
	for k, v := range kv {
		if err := s.deps.Store.SetSetting(k, v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if s.deps.ReloadFlood != nil {
		s.deps.ReloadFlood()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	total, _ := s.deps.Store.CountTraps()
	bySev, _ := s.deps.Store.CountTrapsBySeverity()
	notif, _ := s.deps.Store.CountNotifications()
	recent, _ := s.deps.Store.ListTraps("received_at", "desc", 10)
	writeJSON(w, http.StatusOK, map[string]any{
		"traps_total":       total,
		"traps_by_severity": bySev,
		"notifications":     notif,
		"recent":            recent,
	})
}

// ---- Severities ----

func (s *Server) listSeverities(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Store.ListSeverities()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createSeverity(w http.ResponseWriter, r *http.Request) {
	var v store.Severity
	if err := decodeJSON(r, &v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	id, err := s.deps.Store.CreateSeverity(v)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v.ID = id
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateSeverity(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var v store.Severity
	if err := decodeJSON(r, &v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v.ID = id
	if err := s.deps.Store.UpdateSeverity(v); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteSeverity(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteSeverity(id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- OID map ----

func (s *Server) listOIDMap(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Store.ListOIDMap()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createOID(w http.ResponseWriter, r *http.Request) {
	var e store.OIDEntry
	if err := decodeJSON(r, &e); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	id, err := s.deps.Store.UpsertOID(e)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	e.ID = id
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) updateOID(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	var e store.OIDEntry
	if err := decodeJSON(r, &e); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	e.ID = id
	if err := s.deps.Store.UpdateOID(e); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deleteOID(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteOID(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Devices ----

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Store.ListDevices()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var d store.Device
	if err := decodeJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	id, err := s.deps.Store.CreateDevice(d)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ID = id
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	var d store.Device
	if err := decodeJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.ID = id
	if err := s.deps.Store.UpdateDevice(d); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteDevice(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Channels ----

type channelRequest struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"` // provider-specific; sealed at rest
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	chans, err := s.deps.Store.ListChannels()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chans) // Channel.ConfigSealed is json:"-"
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	sealed, err := s.sealConfig(req.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.deps.Store.AddChannel(req.Name, req.Kind, sealed, req.Enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": req.Name, "kind": req.Kind, "enabled": req.Enabled})
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	var req channelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	c := store.Channel{ID: id, Name: req.Name, Kind: req.Kind, Enabled: req.Enabled}
	// Empty config → preserve existing sealed secret (write-only semantics).
	if len(req.Config) > 0 && string(req.Config) != "null" {
		sealed, err := s.sealConfig(req.Config)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		c.ConfigSealed = sealed
	}
	if err := s.deps.Store.UpdateChannel(c); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": req.Name, "kind": req.Kind, "enabled": req.Enabled})
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteChannel(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sealConfig(cfg json.RawMessage) (string, error) {
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	return s.deps.Sealer.SealString(string(cfg))
}

// ---- Rules ----

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Store.ListRules()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var rule store.Rule
	if err := decodeJSON(r, &rule); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	id, err := s.deps.Store.CreateRule(rule)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rule.ID = id
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	var rule store.Rule
	if err := decodeJSON(r, &rule); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	rule.ID = id
	if err := s.deps.Store.UpdateRule(rule); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteRule(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testRule(w http.ResponseWriter, r *http.Request) {
	if s.deps.RuleTest == nil {
		writeErr(w, http.StatusNotImplemented, "rule test not available")
		return
	}
	var in RuleTestInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	res, err := s.deps.RuleTest(r.Context(), in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) reorderRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderedIDs []int64 `json:"ordered_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.deps.Store.ReorderRules(body.OrderedIDs); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- Default routes ----

func (s *Server) listRoutes(w http.ResponseWriter, r *http.Request) {
	m, err := s.deps.Store.ListDefaultRoutes()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) setRoutes(w http.ResponseWriter, r *http.Request) {
	sevID, err := idParam(r, "severityID")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid severity id")
		return
	}
	var body struct {
		ChannelIDs []int64 `json:"channel_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.deps.Store.SetDefaultRoutes(sevID, body.ChannelIDs); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

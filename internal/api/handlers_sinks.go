package api

import (
	"net/http"

	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

type v3UserRequest struct {
	Username      string `json:"username"`
	SecurityLevel string `json:"security_level"`
	AuthProtocol  string `json:"auth_protocol"`
	AuthPass      string `json:"auth_pass"`
	PrivProtocol  string `json:"priv_protocol"`
	PrivPass      string `json:"priv_pass"`
	EngineID      string `json:"engine_id"`
	Enabled       bool   `json:"enabled"`
}

func (s *Server) listV3Users(w http.ResponseWriter, r *http.Request) {
	users, err := s.deps.Store.ListV3Users()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users) // sealed passwords are json:"-"
}

func (s *Server) createV3User(w http.ResponseWriter, r *http.Request) {
	var req v3UserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	// Enforce password protection (decision 8) before sealing.
	if err := (snmp.USMUser{
		Username: req.Username, SecurityLevel: req.SecurityLevel,
		AuthProtocol: req.AuthProtocol, AuthPass: req.AuthPass,
		PrivProtocol: req.PrivProtocol, PrivPass: req.PrivPass,
	}).Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u := store.SNMPv3User{
		Username: req.Username, SecurityLevel: req.SecurityLevel,
		AuthProtocol: req.AuthProtocol, PrivProtocol: req.PrivProtocol,
		EngineID: req.EngineID, Enabled: req.Enabled,
	}
	var err error
	if u.AuthPassSealed, err = s.deps.Sealer.SealString(req.AuthPass); err != nil {
		writeErr(w, http.StatusInternalServerError, "seal auth password")
		return
	}
	if req.PrivPass != "" {
		if u.PrivPassSealed, err = s.deps.Sealer.SealString(req.PrivPass); err != nil {
			writeErr(w, http.StatusInternalServerError, "seal privacy password")
			return
		}
	}
	id, err := s.deps.Store.CreateV3User(u)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "username": u.Username})
}

func (s *Server) updateV3User(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	var req v3UserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.SecurityLevel != snmp.SecurityAuthNoPriv && req.SecurityLevel != snmp.SecurityAuthPriv {
		writeErr(w, http.StatusBadRequest, "security level must be authNoPriv or authPriv")
		return
	}
	u := store.SNMPv3User{
		ID: id, Username: req.Username, SecurityLevel: req.SecurityLevel,
		AuthProtocol: req.AuthProtocol, PrivProtocol: req.PrivProtocol,
		EngineID: req.EngineID, Enabled: req.Enabled,
	}
	// Empty passwords preserve the stored secret.
	if req.AuthPass != "" {
		sealed, err := s.deps.Sealer.SealString(req.AuthPass)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "seal auth password")
			return
		}
		u.AuthPassSealed = sealed
	}
	if req.PrivPass != "" {
		sealed, err := s.deps.Sealer.SealString(req.PrivPass)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "seal privacy password")
			return
		}
		u.PrivPassSealed = sealed
	}
	if err := s.deps.Store.UpdateV3User(u); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "username": u.Username})
}

func (s *Server) deleteV3User(w http.ResponseWriter, r *http.Request) {
	id, _ := idParam(r, "id")
	if err := s.deps.Store.DeleteV3User(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

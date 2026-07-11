package api

import (
	"net/http"

	"github.com/t0mer/holonet/internal/auth"
)

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	_, authed := s.deps.Auth.Verify(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":    s.deps.Auth.IsConfigured(),
		"auth_enabled":  s.deps.Auth.AuthEnabled(),
		"authenticated": authed || !s.deps.Auth.AuthEnabled(),
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.deps.Auth.Setup(c.Username, c.Password); err != nil {
		if err == auth.ErrAlreadyConfigured {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.deps.Auth.IssueCookie(w, c.Username)
	writeJSON(w, http.StatusCreated, map[string]string{"username": c.Username})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !s.deps.Auth.Authenticate(c.Username, c.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.deps.Auth.IssueCookie(w, c.Username)
	writeJSON(w, http.StatusOK, map[string]string{"username": c.Username})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.deps.Auth.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

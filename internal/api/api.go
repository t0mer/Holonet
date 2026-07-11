// Package api exposes HoloNet's REST API (chi) and serves the embedded SPA
// (design §3.7). Every UI action maps to a documented /api/v1 endpoint. Sealed
// secrets are never returned in responses.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/t0mer/holonet/internal/auth"
	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/store"
)

// Replayer re-runs routing for a stored trap and returns the new trap id.
type Replayer func(ctx context.Context, trapID int64) (int64, error)

// RuleTestInput is a sample event for a rule dry-run.
type RuleTestInput struct {
	SourceIP string `json:"source_ip"`
	TrapOID  string `json:"trap_oid"`
	Message  string `json:"message"`
}

// RuleTestResult is the classification a sample event would receive, without
// persisting or dispatching anything.
type RuleTestResult struct {
	ResolvedName       string   `json:"resolved_name"`
	Unmapped           bool     `json:"unmapped"`
	SeverityID         *int64   `json:"severity_id"`
	SeverityName       string   `json:"severity_name"`
	Matched            bool     `json:"matched"`
	MatchedRuleID      *int64   `json:"matched_rule_id"`
	MatchedRuleName    string   `json:"matched_rule_name"`
	BypassFloodControl bool     `json:"bypass_flood_control"`
	ChannelIDs         []int64  `json:"channel_ids"`
	ChannelNames       []string `json:"channel_names"`
}

// RuleTester dry-runs the decoder + rule engine against a sample event.
type RuleTester func(ctx context.Context, in RuleTestInput) (RuleTestResult, error)

// FloodReloader re-reads flood settings and applies them at runtime.
type FloodReloader func()

// Deps are the API server's dependencies.
type Deps struct {
	Store       *store.Store
	Sealer      *crypto.Sealer
	Auth        *auth.Manager
	Dispatch    *notify.Dispatcher
	Replay      Replayer
	RuleTest    RuleTester
	ReloadFlood FloodReloader
	Metrics     http.Handler // Prometheus /metrics handler (optional)
	Version     string
	SPA         fs.FS
}

// Server holds the router and dependencies.
type Server struct {
	deps   Deps
	router chi.Router
}

// New builds the API server and wires all routes.
func New(deps Deps) *Server {
	s := &Server{deps: deps}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Get("/health", s.handleHealth)
	r.Get("/api/openapi.yaml", s.handleOpenAPI)
	r.Get("/api/docs", s.handleDocs)
	if deps.Metrics != nil {
		r.Handle("/metrics", deps.Metrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoints.
		r.Get("/auth/status", s.handleAuthStatus)
		r.Post("/auth/setup", s.handleAuthSetup)
		r.Post("/auth/login", s.handleAuthLogin)
		r.Post("/auth/logout", s.handleAuthLogout)

		// Protected resources.
		r.Group(func(r chi.Router) {
			r.Use(deps.Auth.Middleware)
			s.mountResources(r)
		})
	})

	// SPA (last, catch-all).
	r.NotFound(s.handleSPA)
	s.router = r
	return s
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.deps.Version})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// decodeJSON decodes the request body into dst, rejecting unknown fields.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// idParam parses a URL path integer parameter.
func idParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func notFoundOr(w http.ResponseWriter, err error) {
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
}

// handleSPA serves the embedded frontend, falling back to index.html for client
// routes (history API). API and health paths never reach here.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.deps.SPA.Open(p); err == nil {
		f.Close()
		http.FileServer(http.FS(s.deps.SPA)).ServeHTTP(w, r)
		return
	}
	// Client-side route: serve index.html.
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	http.FileServer(http.FS(s.deps.SPA)).ServeHTTP(w, r2)
}

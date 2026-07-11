package api

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t0mer/holonet/internal/auth"
	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	sealer, _ := crypto.New("master")
	authMgr := auth.New(st, auth.SigningKeyFromMaster("master"), time.Hour, false)
	return New(Deps{
		Store: st, Sealer: sealer, Auth: authMgr,
		Dispatch: notify.NewDispatcher(sealer, time.Second, 0),
		Version:  "test", SPA: emptyFS{},
	})
}

// emptyFS is a minimal fs.FS with just index.html.
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) { return nil, fs.ErrNotExist }

func do(t *testing.T, s *Server, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	return rec
}

func TestAuthGateAndSetup(t *testing.T) {
	s := testServer(t)

	// Status: not configured.
	rec := do(t, s, "GET", "/api/v1/auth/status", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status code %d", rec.Code)
	}
	var status map[string]any
	json.Unmarshal(rec.Body.Bytes(), &status)
	if status["configured"] != false {
		t.Errorf("expected not configured, got %v", status)
	}

	// Protected endpoint without auth → 401.
	if rec := do(t, s, "GET", "/api/v1/severities", "", nil); rec.Code != 401 {
		t.Fatalf("severities without auth = %d, want 401", rec.Code)
	}

	// Setup issues a cookie.
	rec = do(t, s, "POST", "/api/v1/auth/setup", `{"username":"admin","password":"hunter2!"}`, nil)
	if rec.Code != 201 {
		t.Fatalf("setup = %d: %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("setup did not set a cookie")
	}
	session := cookies[0]

	// Now severities is reachable and seeded.
	rec = do(t, s, "GET", "/api/v1/severities", "", session)
	if rec.Code != 200 {
		t.Fatalf("severities with auth = %d", rec.Code)
	}
	var sevs []store.Severity
	json.Unmarshal(rec.Body.Bytes(), &sevs)
	if len(sevs) != 5 {
		t.Errorf("severities = %d, want 5 builtins", len(sevs))
	}
}

func authed(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	rec := do(t, s, "POST", "/api/v1/auth/setup", `{"username":"admin","password":"hunter2!"}`, nil)
	return rec.Result().Cookies()[0]
}

func TestChannelSecretNeverReturned(t *testing.T) {
	s := testServer(t)
	c := authed(t, s)

	body := `{"name":"tg","kind":"shoutrrr","enabled":true,"config":{"url":"telegram://secret-token@telegram?chats=@c"}}`
	if rec := do(t, s, "POST", "/api/v1/channels", body, c); rec.Code != 201 {
		t.Fatalf("create channel = %d: %s", rec.Code, rec.Body)
	}
	rec := do(t, s, "GET", "/api/v1/channels", "", c)
	if strings.Contains(rec.Body.String(), "secret-token") || strings.Contains(rec.Body.String(), "config_sealed") {
		t.Fatalf("channel listing leaked the sealed secret: %s", rec.Body)
	}
}

func TestTrapsSortInjectionSafe(t *testing.T) {
	s := testServer(t)
	c := authed(t, s)
	rec := do(t, s, "GET", "/api/v1/traps?sort=%27%3B+DROP+TABLE+traps%3B--&order=asc&limit=5", "", c)
	if rec.Code != 200 {
		t.Fatalf("traps = %d: %s", rec.Code, rec.Body)
	}
}

func TestRuleCreateAndList(t *testing.T) {
	s := testServer(t)
	c := authed(t, s)
	body := `{"ord":1,"name":"links","enabled":true,"match_oid_glob":"1.3.6.1.6.3.1.1.5.*","severity_id":2}`
	if rec := do(t, s, "POST", "/api/v1/rules", body, c); rec.Code != 201 {
		t.Fatalf("create rule = %d: %s", rec.Code, rec.Body)
	}
	rec := do(t, s, "GET", "/api/v1/rules", "", c)
	var rules []store.Rule
	json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 1 || rules[0].Name != "links" {
		t.Errorf("rules = %+v", rules)
	}
}

func TestHealth(t *testing.T) {
	s := testServer(t)
	rec := do(t, s, "GET", "/health", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "test") {
		t.Errorf("health = %d %s", rec.Code, rec.Body)
	}
}

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// memStore is an in-memory dataStore for tests.
type memStore struct {
	user, hash string
	settings   map[string]string
}

func (m *memStore) GetAdmin() (string, string, error) {
	if m.user == "" {
		return "", "", errNoAdmin
	}
	return m.user, m.hash, nil
}
func (m *memStore) CreateAdmin(u, h string) error { m.user, m.hash = u, h; return nil }
func (m *memStore) GetSetting(k string) (string, error) {
	if v, ok := m.settings[k]; ok {
		return v, nil
	}
	return "", errNoSetting
}

var errNoAdmin = &stringErr{"no admin"}
var errNoSetting = &stringErr{"no setting"}

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }

func newManager() (*Manager, *memStore) {
	st := &memStore{settings: map[string]string{}}
	return New(st, []byte("signing-key"), time.Hour, false), st
}

func TestSetupAndAuthenticate(t *testing.T) {
	m, _ := newManager()
	if m.IsConfigured() {
		t.Fatal("should not be configured initially")
	}
	if err := m.Setup("admin", "hunter2!"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !m.IsConfigured() {
		t.Fatal("should be configured after setup")
	}
	if err := m.Setup("admin", "another8x"); err != ErrAlreadyConfigured {
		t.Fatalf("second setup should fail, got %v", err)
	}
	if !m.Authenticate("admin", "hunter2!") {
		t.Error("correct credentials should authenticate")
	}
	if m.Authenticate("admin", "wrong") {
		t.Error("wrong password must fail")
	}
	if m.Authenticate("root", "hunter2!") {
		t.Error("wrong username must fail")
	}
}

func TestSetupRejectsWeakPassword(t *testing.T) {
	m, _ := newManager()
	if err := m.Setup("admin", "short"); err == nil {
		t.Fatal("expected weak-password rejection")
	}
}

func TestCookieRoundTrip(t *testing.T) {
	m, _ := newManager()
	rec := httptest.NewRecorder()
	m.IssueCookie(rec, "admin")
	cookie := rec.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	user, ok := m.Verify(req)
	if !ok || user != "admin" {
		t.Fatalf("verify = %q,%v", user, ok)
	}
}

func TestTamperedCookieRejected(t *testing.T) {
	m, _ := newManager()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "forged-value"})
	if _, ok := m.Verify(req); ok {
		t.Fatal("forged cookie must be rejected")
	}
}

func TestMiddlewareEnforcesAndBypasses(t *testing.T) {
	m, st := newManager()
	protected := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Auth on, no cookie → 401.
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/rules", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: code = %d, want 401", rec.Code)
	}

	// Auth disabled → pass through.
	st.settings[SettingAuthEnabled] = "false"
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/rules", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("auth disabled: code = %d, want 200", rec.Code)
	}
}

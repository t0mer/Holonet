// Package auth provides single-admin authentication (design §5): a bcrypt admin
// credential, a stateless HMAC-signed session cookie, a first-run setup wizard,
// and an optional bypass for instances fronted by Cloudflare Access.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// CookieName is the session cookie name.
	CookieName = "holonet_session"
	// SettingAuthEnabled toggles app auth (default "true").
	SettingAuthEnabled = "auth.enabled"
)

// ErrAlreadyConfigured is returned when setup runs on a configured instance.
var ErrAlreadyConfigured = errors.New("auth: admin already configured")

// dataStore is the slice of the store auth needs.
type dataStore interface {
	GetAdmin() (username, passHash string, err error)
	CreateAdmin(username, passHash string) error
	GetSetting(key string) (string, error)
}

// Manager holds auth dependencies.
type Manager struct {
	store      dataStore
	signingKey []byte
	ttl        time.Duration
	secure     bool
}

// New builds a Manager. signingKey signs session cookies; ttl is the session
// lifetime; secure sets the cookie Secure flag (enable behind TLS).
func New(store dataStore, signingKey []byte, ttl time.Duration, secure bool) *Manager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{store: store, signingKey: signingKey, ttl: ttl, secure: secure}
}

// IsConfigured reports whether an admin account exists.
func (m *Manager) IsConfigured() bool {
	_, _, err := m.store.GetAdmin()
	return err == nil
}

// AuthEnabled reports whether app auth is on (default true).
func (m *Manager) AuthEnabled() bool {
	v, err := m.store.GetSetting(SettingAuthEnabled)
	if err != nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(v), "false")
}

// Setup creates the admin account on first run.
func (m *Manager) Setup(username, password string) error {
	if m.IsConfigured() {
		return ErrAlreadyConfigured
	}
	if strings.TrimSpace(username) == "" || len(password) < 8 {
		return errors.New("auth: username required and password must be at least 8 characters")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return m.store.CreateAdmin(username, hash)
}

// HashPassword bcrypt-hashes a password (shared by setup and reset flows).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
}

// Authenticate verifies credentials against the stored bcrypt hash.
func (m *Manager) Authenticate(username, password string) bool {
	storedUser, hash, err := m.store.GetAdmin()
	if err != nil {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(storedUser)) != 1 {
		// Still run bcrypt to keep timing uniform against user enumeration.
		bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IssueCookie sets a signed session cookie for username.
func (m *Manager) IssueCookie(w http.ResponseWriter, username string) {
	exp := time.Now().Add(m.ttl)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    m.sign(username, exp),
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie removes the session cookie.
func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode,
	})
}

// Verify checks the request's session cookie and returns the username.
func (m *Manager) Verify(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	return m.verifyToken(c.Value)
}

// Middleware enforces authentication. When auth is disabled it passes through;
// otherwise a missing/invalid session yields 401.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.AuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := m.Verify(r); !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sign builds a signed token: base64(username|expiryUnix|hmac).
func (m *Manager) sign(username string, exp time.Time) string {
	payload := fmt.Sprintf("%s|%d", username, exp.Unix())
	mac := m.hmac(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + mac))
}

func (m *Manager) verifyToken(token string) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", false
	}
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 {
		return "", false
	}
	username, expStr, sig := parts[0], parts[1], parts[2]
	if !hmac.Equal([]byte(sig), []byte(m.hmac(username+"|"+expStr))) {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return username, true
}

func (m *Manager) hmac(payload string) string {
	h := hmac.New(sha256.New, m.signingKey)
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// SigningKeyFromMaster derives a stable cookie-signing key from the master key.
func SigningKeyFromMaster(masterKey string) []byte {
	sum := sha256.Sum256([]byte("holonet-session:" + masterKey))
	return sum[:]
}

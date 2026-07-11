package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WhatsAppConfig configures delivery via a self-hosted
// go-whatsapp-web-multidevice gateway (design §3.5). The send path is a config
// field so it tracks whatever route the running gateway exposes.
type WhatsAppConfig struct {
	BaseURL   string `json:"base_url"`
	Endpoint  string `json:"endpoint"`  // default /send/message
	Recipient string `json:"recipient"` // JID or phone (e.g. 972501234567 or 1234@g.us)
	Username  string `json:"username"`  // optional basic auth
	Password  string `json:"password"`
	Token     string `json:"token"` // optional bearer token (alternative to basic auth)
}

// WhatsAppNotifier posts messages to the WhatsApp gateway.
type WhatsAppNotifier struct {
	cfg    WhatsAppConfig
	client *http.Client
}

// NewWhatsApp builds a notifier from unsealed JSON config.
func NewWhatsApp(configJSON string) (*WhatsAppNotifier, error) {
	var cfg WhatsAppConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("whatsapp: parse config: %w", err)
	}
	if cfg.BaseURL == "" || cfg.Recipient == "" {
		return nil, fmt.Errorf("whatsapp: base_url and recipient are required")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "/send/message"
	}
	return &WhatsAppNotifier{cfg: cfg, client: &http.Client{Timeout: 20 * time.Second}}, nil
}

// Kind implements Notifier.
func (n *WhatsAppNotifier) Kind() string { return KindWhatsApp }

// Send posts the rendered message to the gateway.
func (n *WhatsAppNotifier) Send(ctx context.Context, msg Message) error {
	body, err := json.Marshal(map[string]string{
		"phone":   n.cfg.Recipient,
		"message": renderPlain(msg),
	})
	if err != nil {
		return fmt.Errorf("whatsapp: marshal: %w", err)
	}
	url := strings.TrimRight(n.cfg.BaseURL, "/") + "/" + strings.TrimLeft(n.cfg.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("whatsapp: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if n.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.cfg.Token)
	} else if n.cfg.Username != "" {
		req.SetBasicAuth(n.cfg.Username, n.cfg.Password)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp: send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("whatsapp: gateway returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

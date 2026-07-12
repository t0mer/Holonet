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

// GreenAPIConfig configures delivery via the GreenAPI WhatsApp Cloud gateway
// (notifications guideline). All fields are trimmed on load: stray whitespace in
// InstanceID or Token corrupts the request URL and yields opaque 400s.
type GreenAPIConfig struct {
	InstanceID string `json:"instance_id"`
	Token      string `json:"token"`
	Recipient  string `json:"recipient"` // international phone, digits only (e.g. 972501234567), or a JID
	APIURL     string `json:"api_url"`   // optional; blank -> https://api.green-api.com
}

// defaultGreenAPIURL is used when no cluster URL is configured.
const defaultGreenAPIURL = "https://api.green-api.com"

// GreenAPINotifier posts messages to the GreenAPI sendMessage endpoint.
type GreenAPINotifier struct {
	cfg    GreenAPIConfig
	client *http.Client
}

// NewGreenAPI builds a notifier from unsealed JSON config.
func NewGreenAPI(configJSON string) (*GreenAPINotifier, error) {
	var cfg GreenAPIConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("greenapi: parse config: %w", err)
	}
	cfg.InstanceID = strings.TrimSpace(cfg.InstanceID)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.Recipient = strings.TrimSpace(cfg.Recipient)
	cfg.APIURL = strings.TrimSpace(cfg.APIURL)

	if cfg.InstanceID == "" || cfg.Token == "" || cfg.Recipient == "" {
		return nil, fmt.Errorf("greenapi: instance_id, token and recipient are required")
	}
	if cfg.APIURL == "" {
		cfg.APIURL = defaultGreenAPIURL
	}
	return &GreenAPINotifier{cfg: cfg, client: &http.Client{Timeout: 20 * time.Second}}, nil
}

// Kind implements Notifier.
func (n *GreenAPINotifier) Kind() string { return KindGreenAPI }

// Send posts the rendered message to GreenAPI's sendMessage endpoint:
//
//	POST {apiURL}/waInstance{instanceID}/sendMessage/{token}
//	{"chatId": "{recipient}@c.us", "message": "..."}
func (n *GreenAPINotifier) Send(ctx context.Context, msg Message) error {
	chatID := n.cfg.Recipient
	if !strings.Contains(chatID, "@") {
		chatID += "@c.us"
	}
	body, err := json.Marshal(map[string]string{
		"chatId":  chatID,
		"message": renderPlain(msg),
	})
	if err != nil {
		return fmt.Errorf("greenapi: marshal: %w", err)
	}
	url := fmt.Sprintf("%s/waInstance%s/sendMessage/%s",
		strings.TrimRight(n.cfg.APIURL, "/"), n.cfg.InstanceID, n.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("greenapi: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("greenapi: send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("greenapi: gateway returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

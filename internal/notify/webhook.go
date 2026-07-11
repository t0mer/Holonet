package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// WebhookConfig configures a generic HTTP webhook notifier (design §3.5).
type WebhookConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`        // default POST
	Headers      map[string]string `json:"headers"`       // optional extra headers
	BodyTemplate string            `json:"body_template"` // text/template over Message; default JSON
}

// WebhookNotifier POSTs a rendered body to a configurable URL.
type WebhookNotifier struct {
	cfg    WebhookConfig
	tmpl   *template.Template // nil → default JSON body
	client *http.Client
}

// NewWebhook builds a notifier from unsealed JSON config.
func NewWebhook(configJSON string) (*WebhookNotifier, error) {
	var cfg WebhookConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("webhook: parse config: %w", err)
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("webhook: url is required")
	}
	if cfg.Method == "" {
		cfg.Method = http.MethodPost
	}
	n := &WebhookNotifier{cfg: cfg, client: &http.Client{Timeout: 20 * time.Second}}
	if strings.TrimSpace(cfg.BodyTemplate) != "" {
		t, err := template.New("webhook").Parse(cfg.BodyTemplate)
		if err != nil {
			return nil, fmt.Errorf("webhook: parse body template: %w", err)
		}
		n.tmpl = t
	}
	return n, nil
}

// Kind implements Notifier.
func (n *WebhookNotifier) Kind() string { return KindWebhook }

// Send renders the body and dispatches the HTTP request.
func (n *WebhookNotifier) Send(ctx context.Context, msg Message) error {
	body, contentType, err := n.renderBody(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, n.cfg.Method, n.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range n.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook: %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

// renderBody produces the request body. With no template, a JSON object of the
// message is sent; with a template, its rendered text is sent as-is.
func (n *WebhookNotifier) renderBody(msg Message) ([]byte, string, error) {
	if n.tmpl == nil {
		b, err := json.Marshal(map[string]any{
			"title":    msg.Title,
			"body":     msg.Body,
			"severity": msg.Severity,
			"emoji":    msg.Emoji,
			"fields":   msg.Fields,
		})
		return b, "application/json", err
	}
	var buf bytes.Buffer
	if err := n.tmpl.Execute(&buf, msg); err != nil {
		return nil, "", fmt.Errorf("webhook: render body: %w", err)
	}
	return buf.Bytes(), "application/json", nil
}

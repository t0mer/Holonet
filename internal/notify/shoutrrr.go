package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containrrr/shoutrrr"
)

// ShoutrrrConfig is the stored config for a Shoutrrr channel. The URL carries
// its own credentials (bot token etc.) and is sealed at rest.
type ShoutrrrConfig struct {
	URL string `json:"url"`
}

// ShoutrrrNotifier delivers via containrrr/shoutrrr, whose single URL scheme
// covers Telegram, Discord, Slack, ntfy, Pushover, generic webhooks, email, …
type ShoutrrrNotifier struct {
	url string
}

// NewShoutrrr builds a notifier from unsealed JSON config.
func NewShoutrrr(configJSON string) (*ShoutrrrNotifier, error) {
	var cfg ShoutrrrConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("shoutrrr: parse config: %w", err)
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("shoutrrr: url is required")
	}
	return &ShoutrrrNotifier{url: cfg.URL}, nil
}

// Kind implements Notifier.
func (n *ShoutrrrNotifier) Kind() string { return KindShoutrrr }

// Send renders the message and dispatches it through shoutrrr. Shoutrrr's Send
// is blocking with no context hook; the Dispatcher enforces the timeout by
// abandoning the wait, so we honor ctx cancellation at the boundary.
func (n *ShoutrrrNotifier) Send(ctx context.Context, msg Message) error {
	text := renderPlain(msg)
	errCh := make(chan error, 1)
	go func() {
		if err := shoutrrr.Send(n.url, text); err != nil {
			errCh <- fmt.Errorf("shoutrrr: send: %w", err)
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

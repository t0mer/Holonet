package notify

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/t0mer/holonet/internal/store"
)

// Notification statuses recorded per attempt (design §3.6).
const (
	StatusSent   = "sent"
	StatusFailed = "failed"
	StatusHeld   = "held"
)

// Unsealer decrypts a channel's sealed config (satisfied by *crypto.Sealer).
type Unsealer interface {
	OpenString(sealed string) (string, error)
}

// Result is the outcome of dispatching to one channel.
type Result struct {
	ChannelID int64
	Status    string
	Attempts  int
	Err       error
}

// Dispatcher fans a Message out to channels concurrently. Each channel gets its
// own timeout and bounded retry; one channel failing never blocks the others
// (design §3.5).
type Dispatcher struct {
	unsealer Unsealer
	timeout  time.Duration
	retries  int
	build    func(kind, configJSON string) (Notifier, error)
}

// NewDispatcher builds a Dispatcher. timeout is per attempt; retries is the
// number of extra attempts after the first.
func NewDispatcher(u Unsealer, timeout time.Duration, retries int) *Dispatcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if retries < 0 {
		retries = 0
	}
	return &Dispatcher{unsealer: u, timeout: timeout, retries: retries, build: BuildNotifier}
}

// UseBuilder overrides how notifiers are constructed from a channel kind and
// config. Used by tests to inject fakes.
func (d *Dispatcher) UseBuilder(fn func(kind, configJSON string) (Notifier, error)) {
	d.build = fn
}

// Dispatch sends msg to every channel concurrently and returns one Result per
// channel (order not guaranteed).
func (d *Dispatcher) Dispatch(ctx context.Context, channels []store.Channel, msg Message) []Result {
	results := make([]Result, len(channels))
	var wg sync.WaitGroup
	for i, ch := range channels {
		wg.Add(1)
		go func(i int, ch store.Channel) {
			defer wg.Done()
			results[i] = d.dispatchOne(ctx, ch, msg)
		}(i, ch)
	}
	wg.Wait()
	return results
}

func (d *Dispatcher) dispatchOne(ctx context.Context, ch store.Channel, msg Message) Result {
	res := Result{ChannelID: ch.ID, Status: StatusFailed}

	configJSON, err := d.unsealer.OpenString(ch.ConfigSealed)
	if err != nil {
		res.Err = fmt.Errorf("unseal channel %d config: %w", ch.ID, err)
		return res
	}
	notifier, err := d.build(ch.Kind, configJSON)
	if err != nil {
		res.Err = err
		return res
	}

	var lastErr error
	for attempt := 1; attempt <= d.retries+1; attempt++ {
		res.Attempts = attempt
		if err := ctx.Err(); err != nil {
			res.Err = err
			return res
		}
		attemptCtx, cancel := context.WithTimeout(ctx, d.timeout)
		err := notifier.Send(attemptCtx, msg)
		cancel()
		if err == nil {
			res.Status = StatusSent
			res.Err = nil
			return res
		}
		lastErr = err
	}
	res.Err = lastErr
	return res
}

// BuildNotifier constructs a Notifier for a channel kind from its unsealed JSON
// config. WhatsApp and Webhook arrive in Slice 3.
func BuildNotifier(kind, configJSON string) (Notifier, error) {
	switch kind {
	case KindShoutrrr:
		return NewShoutrrr(configJSON)
	case KindWhatsApp:
		return NewWhatsApp(configJSON)
	case KindGreenAPI:
		return NewGreenAPI(configJSON)
	case KindWebhook:
		return NewWebhook(configJSON)
	default:
		return nil, fmt.Errorf("notify: unknown channel kind %q", kind)
	}
}

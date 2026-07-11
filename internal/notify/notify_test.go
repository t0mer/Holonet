package notify

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t0mer/holonet/internal/store"
)

func TestRenderPlain(t *testing.T) {
	got := renderPlain(Message{
		Title:    "linkDown from 192.0.2.10",
		Severity: "High",
		Emoji:    "🟠",
		Fields:   []Field{{Name: "Device", Value: "sfvh"}},
	})
	for _, want := range []string{"🟠", "[High]", "linkDown from 192.0.2.10", "Device: sfvh"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered text missing %q:\n%s", want, got)
		}
	}
}

func TestNewShoutrrrRequiresURL(t *testing.T) {
	if _, err := NewShoutrrr(`{}`); err == nil {
		t.Fatal("expected error when url missing")
	}
	if _, err := NewShoutrrr(`{"url":"telegram://tok@telegram?chats=@c"}`); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

// fakeUnsealer returns the sealed string unchanged (already plaintext JSON).
type fakeUnsealer struct{}

func (fakeUnsealer) OpenString(s string) (string, error) { return s, nil }

// fakeNotifier records sends and can be told to fail a number of times first.
type fakeNotifier struct {
	failFirst int32
	calls     int32
}

func (f *fakeNotifier) Kind() string { return "fake" }
func (f *fakeNotifier) Send(ctx context.Context, msg Message) error {
	n := atomic.AddInt32(&f.calls, 1)
	if n <= atomic.LoadInt32(&f.failFirst) {
		return errors.New("transient")
	}
	return nil
}

func TestDispatchConcurrentAndRecordsResults(t *testing.T) {
	d := NewDispatcher(fakeUnsealer{}, time.Second, 0)
	d.build = func(kind, cfg string) (Notifier, error) { return &fakeNotifier{}, nil }

	channels := []store.Channel{
		{ID: 1, Kind: "fake", ConfigSealed: "{}", Enabled: true},
		{ID: 2, Kind: "fake", ConfigSealed: "{}", Enabled: true},
	}
	results := d.Dispatch(context.Background(), channels, Message{Title: "hi"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Status != StatusSent || r.Err != nil {
			t.Errorf("channel %d: status=%s err=%v", r.ChannelID, r.Status, r.Err)
		}
	}
}

func TestDispatchRetriesThenSucceeds(t *testing.T) {
	fn := &fakeNotifier{failFirst: 1}
	d := NewDispatcher(fakeUnsealer{}, time.Second, 2)
	d.build = func(kind, cfg string) (Notifier, error) { return fn, nil }

	results := d.Dispatch(context.Background(),
		[]store.Channel{{ID: 5, Kind: "fake", ConfigSealed: "{}"}}, Message{Title: "x"})
	if results[0].Status != StatusSent {
		t.Fatalf("expected sent after retry, got %s (err %v)", results[0].Status, results[0].Err)
	}
	if results[0].Attempts != 2 {
		t.Errorf("attempts = %d, want 2", results[0].Attempts)
	}
}

func TestDispatchFailsAfterRetries(t *testing.T) {
	fn := &fakeNotifier{failFirst: 100}
	d := NewDispatcher(fakeUnsealer{}, time.Second, 1)
	d.build = func(kind, cfg string) (Notifier, error) { return fn, nil }

	results := d.Dispatch(context.Background(),
		[]store.Channel{{ID: 9, Kind: "fake", ConfigSealed: "{}"}}, Message{Title: "x"})
	if results[0].Status != StatusFailed || results[0].Err == nil {
		t.Fatalf("expected failed, got %s err=%v", results[0].Status, results[0].Err)
	}
	if results[0].Attempts != 2 {
		t.Errorf("attempts = %d, want 2", results[0].Attempts)
	}
}

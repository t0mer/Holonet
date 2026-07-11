package pipeline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

// captureNotifier records the messages it is asked to send.
type captureNotifier struct{ got []notify.Message }

func (c *captureNotifier) Kind() string { return "capture" }
func (c *captureNotifier) Send(ctx context.Context, msg notify.Message) error {
	c.got = append(c.got, msg)
	return nil
}

func TestProcessDecodesDispatchesAndPersists(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	sealer, _ := crypto.New("test-key")
	// One enabled channel with sealed (here plaintext-through) config.
	sealed, _ := sealer.SealString(`{"url":"noop://"}`)
	if _, err := s.AddChannel("tg", notify.KindShoutrrr, sealed, true); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	cap := &captureNotifier{}
	disp := notify.NewDispatcher(sealer, time.Second, 0)
	disp.UseBuilder(func(kind, cfg string) (notify.Notifier, error) { return cap, nil })

	p := New(s, decode.New(s), disp, nil)

	raw := snmp.RawTrap{
		ReceivedAt: time.Now(),
		SourceIP:   "192.0.2.9",
		Version:    "v2c",
		TrapOID:    "1.3.6.1.6.3.1.1.5.3", // linkDown, seeded High
		Varbinds: []snmp.Varbind{
			{OID: snmp.SnmpTrapOID, Type: "OID", Value: "1.3.6.1.6.3.1.1.5.3"},
		},
	}
	if err := p.process(context.Background(), raw); err != nil {
		t.Fatalf("process: %v", err)
	}

	// Notifier received a High-severity linkDown message.
	if len(cap.got) != 1 {
		t.Fatalf("notifier got %d messages, want 1", len(cap.got))
	}
	if cap.got[0].Severity != "High" || cap.got[0].Title != "linkDown" {
		t.Errorf("message = %+v", cap.got[0])
	}

	// Trap persisted.
	var trapCount int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM traps WHERE resolved_name='linkDown'`).Scan(&trapCount); err != nil {
		t.Fatal(err)
	}
	if trapCount != 1 {
		t.Errorf("trap rows = %d, want 1", trapCount)
	}

	// Notification recorded as sent.
	var status string
	if err := s.DB().QueryRow(`SELECT status FROM notifications LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != notify.StatusSent {
		t.Errorf("notification status = %q, want sent", status)
	}
}

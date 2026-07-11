package pipeline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/flood"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/rules"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

type captureNotifier struct{ got []notify.Message }

func (c *captureNotifier) Kind() string { return "capture" }
func (c *captureNotifier) Send(ctx context.Context, msg notify.Message) error {
	c.got = append(c.got, msg)
	return nil
}

func setup(t *testing.T, floodCfg flood.Config) (*store.Store, *Processor, *captureNotifier) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	sealer, _ := crypto.New("test-key")
	sealed, _ := sealer.SealString(`{"url":"noop://"}`)
	chID, _ := s.AddChannel("tg", notify.KindShoutrrr, sealed, true)
	// Route High (severity id 2) to the channel by default.
	s.SetDefaultRoutes(2, []int64{chID})

	cap := &captureNotifier{}
	disp := notify.NewDispatcher(sealer, time.Second, 0)
	disp.UseBuilder(func(kind, cfg string) (notify.Notifier, error) { return cap, nil })

	p := New(s, decode.New(s), rules.New(s), flood.New(floodCfg, nil), disp, nil)
	return s, p, cap
}

func linkDownTrap() snmp.RawTrap {
	return snmp.RawTrap{
		ReceivedAt: time.Now(),
		SourceIP:   "192.0.2.9",
		Version:    "v2c",
		TrapOID:    "1.3.6.1.6.3.1.1.5.3", // linkDown, seeded High (id 2)
		Varbinds:   []snmp.Varbind{{OID: snmp.SnmpTrapOID, Type: "OID", Value: "1.3.6.1.6.3.1.1.5.3"}},
	}
}

func TestProcessClassifiesRoutesAndPersists(t *testing.T) {
	s, p, cap := setup(t, flood.Config{Strategy: flood.StrategyNone})
	if _, err := p.Process(context.Background(), linkDownTrap()); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(cap.got) != 1 || cap.got[0].Severity != "High" || cap.got[0].Title != "linkDown" {
		t.Fatalf("message = %+v", cap.got)
	}
	var status string
	if err := s.DB().QueryRow(`SELECT status FROM notifications LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != notify.StatusSent {
		t.Errorf("status = %q, want sent", status)
	}
}

func TestProcessDedupeSuppressesSecond(t *testing.T) {
	s, p, cap := setup(t, flood.Config{Strategy: flood.StrategyDedupe, DedupeWindow: time.Minute})
	ctx := context.Background()
	p.Process(ctx, linkDownTrap())
	p.Process(ctx, linkDownTrap()) // duplicate within window

	if len(cap.got) != 1 {
		t.Errorf("dedupe: notifier got %d messages, want 1", len(cap.got))
	}
	// Both traps stored; second flagged suppressed.
	var total, suppressed int
	s.DB().QueryRow(`SELECT COUNT(*) FROM traps`).Scan(&total)
	s.DB().QueryRow(`SELECT COUNT(*) FROM traps WHERE suppressed=1`).Scan(&suppressed)
	if total != 2 || suppressed != 1 {
		t.Errorf("total=%d suppressed=%d, want 2/1", total, suppressed)
	}
}

func TestProcessUnmappedUsesDefaultSeverity(t *testing.T) {
	s, p, _ := setup(t, flood.Config{Strategy: flood.StrategyNone})
	raw := linkDownTrap()
	raw.TrapOID = "1.3.6.1.4.1.99999.1"
	raw.Varbinds = []snmp.Varbind{{OID: snmp.SnmpTrapOID, Type: "OID", Value: raw.TrapOID}}
	trapID, err := p.Process(context.Background(), raw)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var unmapped bool
	s.DB().QueryRow(`SELECT unmapped FROM traps WHERE id=?`, trapID).Scan(&unmapped)
	if !unmapped {
		t.Error("unknown OID trap should be flagged unmapped")
	}
}

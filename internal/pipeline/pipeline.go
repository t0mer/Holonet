// Package pipeline wires the trap-processing stages together:
// decode → classify → (Slice 1: send-everything) dispatch → persist
// (design §2). Rules and flood control slot in at Slice 2.
package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

// Processor consumes RawTraps, decodes and classifies them, dispatches
// notifications, and persists the results.
type Processor struct {
	store      *store.Store
	decoder    *decode.Decoder
	dispatcher *notify.Dispatcher
	log        *slog.Logger
}

// New builds a Processor.
func New(s *store.Store, d *decode.Decoder, disp *notify.Dispatcher, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	return &Processor{store: s, decoder: d, dispatcher: disp, log: log}
}

// Run reads RawTraps from in until the channel closes or ctx is cancelled,
// processing each one. It is intended to run in its own goroutine.
func (p *Processor) Run(ctx context.Context, in <-chan snmp.RawTrap) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-in:
			if !ok {
				return
			}
			if err := p.process(ctx, raw); err != nil {
				p.log.Error("processing trap", "source", raw.SourceIP, "err", err)
			}
		}
	}
}

func (p *Processor) process(ctx context.Context, raw snmp.RawTrap) error {
	ev, err := p.decoder.Decode(raw)
	if err != nil {
		return err
	}

	// Persist the trap first so it is recorded even if dispatch fails.
	trapID, err := p.persistTrap(ev)
	if err != nil {
		return err
	}

	p.log.Info("trap received",
		"source", ev.RawTrap.SourceIP, "oid", ev.TrapOID,
		"event", ev.ResolvedName, "unmapped", ev.Unmapped, "trap_id", trapID)

	channels, err := p.store.ListEnabledChannels()
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		p.log.Warn("no enabled channels; trap stored but not dispatched", "trap_id", trapID)
		return nil
	}

	msg := p.buildMessage(ev)
	results := p.dispatcher.Dispatch(ctx, channels, msg)
	for _, r := range results {
		p.recordNotification(trapID, r)
	}
	return nil
}

func (p *Processor) persistTrap(ev decode.Event) (int64, error) {
	varbindsJSON, err := json.Marshal(ev.Varbinds)
	if err != nil {
		varbindsJSON = []byte("[]")
	}
	return p.store.InsertTrap(store.Trap{
		ReceivedAt:   ev.RawTrap.ReceivedAt,
		SourceIP:     ev.RawTrap.SourceIP,
		SNMPVersion:  ev.RawTrap.Version,
		TrapOID:      ev.TrapOID,
		ResolvedName: ev.ResolvedName,
		SeverityID:   ev.SeverityID,
		VarbindsJSON: string(varbindsJSON),
		Unmapped:     ev.Unmapped,
	})
}

func (p *Processor) buildMessage(ev decode.Event) notify.Message {
	msg := notify.Message{
		Title: ev.ResolvedName,
		Body:  ev.Message,
		Fields: []notify.Field{
			{Name: "Source", Value: ev.RawTrap.SourceIP},
			{Name: "OID", Value: ev.TrapOID},
			{Name: "Time", Value: ev.RawTrap.ReceivedAt.Format(time.RFC3339)},
		},
	}
	if ev.SeverityID != nil {
		if sev, err := p.store.GetSeverity(*ev.SeverityID); err == nil {
			msg.Severity = sev.Name
			msg.Emoji = sev.Emoji
		}
	}
	return msg
}

func (p *Processor) recordNotification(trapID int64, r notify.Result) {
	chID := r.ChannelID
	n := store.Notification{
		TrapID:    trapID,
		ChannelID: &chID,
		Status:    r.Status,
		Attempts:  r.Attempts,
	}
	if r.Err != nil {
		n.LastError = r.Err.Error()
		p.log.Warn("channel dispatch failed", "trap_id", trapID, "channel_id", chID, "err", r.Err)
	} else {
		now := time.Now()
		n.SentAt = &now
	}
	if _, err := p.store.InsertNotification(n); err != nil {
		p.log.Error("recording notification", "trap_id", trapID, "err", err)
	}
}

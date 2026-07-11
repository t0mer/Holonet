// Package pipeline wires the trap-processing stages together:
// decode → classify (rules) → flood control → dispatch → persist (design §2).
package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/flood"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/rules"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

// Metrics is the optional counter surface the pipeline reports to.
type Metrics interface {
	TrapSuppressed(strategy string)
	NotificationRecorded(channelID int64, status string)
}

// Processor consumes RawTraps, classifies and flood-gates them, dispatches
// notifications, and persists the results.
type Processor struct {
	store      *store.Store
	decoder    *decode.Decoder
	engine     *rules.Engine
	flood      *flood.Controller
	dispatcher *notify.Dispatcher
	log        *slog.Logger
	metrics    Metrics
}

// SetMetrics attaches a metrics sink (optional).
func (p *Processor) SetMetrics(m Metrics) { p.metrics = m }

// New builds a Processor.
func New(s *store.Store, d *decode.Decoder, eng *rules.Engine, fc *flood.Controller, disp *notify.Dispatcher, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	return &Processor{store: s, decoder: d, engine: eng, flood: fc, dispatcher: disp, log: log}
}

// Run reads RawTraps from in until the channel closes or ctx is cancelled.
func (p *Processor) Run(ctx context.Context, in <-chan snmp.RawTrap) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-in:
			if !ok {
				return
			}
			if _, err := p.Process(ctx, raw); err != nil {
				p.log.Error("processing trap", "source", raw.SourceIP, "err", err)
			}
		}
	}
}

// Process runs one trap through the full pipeline and returns the stored trap id.
func (p *Processor) Process(ctx context.Context, raw snmp.RawTrap) (int64, error) {
	ev, err := p.decoder.Decode(raw)
	if err != nil {
		return 0, err
	}

	decision, err := p.engine.Classify(ev)
	if err != nil {
		return 0, err
	}

	aggKey := ev.RawTrap.SourceIP + "|" + ev.TrapOID
	sev := p.severity(decision.SeverityID)

	// Flood gate — bypass rules always send.
	outcome := flood.Send
	if !decision.BypassFloodControl {
		outcome = p.flood.Admit(floodEvent(ev, decision, aggKey, sev), ev.RawTrap.ReceivedAt)
	}
	suppressed := outcome != flood.Send

	trapID, err := p.persistTrap(ev, decision, aggKey, suppressed)
	if err != nil {
		return 0, err
	}

	p.log.Info("trap processed",
		"source", ev.RawTrap.SourceIP, "event", ev.ResolvedName,
		"severity", severityName(sev), "matched_rule", decision.MatchedRuleID,
		"outcome", outcomeName(outcome), "trap_id", trapID)

	switch outcome {
	case flood.Suppress:
		if p.metrics != nil {
			p.metrics.TrapSuppressed(p.flood.Strategy())
		}
		return trapID, nil // deduped: stored + counted, not notified
	case flood.Hold:
		if p.metrics != nil {
			p.metrics.TrapSuppressed(p.flood.Strategy())
		}
		p.recordHeld(trapID)
		return trapID, nil // represented later in a rollup
	}

	p.dispatch(ctx, trapID, ev, decision, sev)
	return trapID, nil
}

// dispatch resolves the decision's channels and fans the message out.
func (p *Processor) dispatch(ctx context.Context, trapID int64, ev decode.Event, decision rules.Decision, sev *store.Severity) {
	channels, err := p.store.ChannelsByIDs(decision.ChannelIDs)
	if err != nil {
		p.log.Error("resolving channels", "trap_id", trapID, "err", err)
		return
	}
	if len(channels) == 0 {
		p.log.Warn("trap stored but no enabled channels routed", "trap_id", trapID)
		return
	}
	msg := buildMessage(ev, sev)
	for _, r := range p.dispatcher.Dispatch(ctx, channels, msg) {
		p.recordNotification(trapID, r)
	}
}

func (p *Processor) severity(id *int64) *store.Severity {
	if id == nil {
		return nil
	}
	sev, err := p.store.GetSeverity(*id)
	if err != nil {
		return nil
	}
	return sev
}

func (p *Processor) persistTrap(ev decode.Event, decision rules.Decision, aggKey string, suppressed bool) (int64, error) {
	varbindsJSON, err := json.Marshal(ev.Varbinds)
	if err != nil {
		varbindsJSON = []byte("[]")
	}
	return p.store.InsertTrap(store.Trap{
		ReceivedAt:     ev.RawTrap.ReceivedAt,
		SourceIP:       ev.RawTrap.SourceIP,
		SNMPVersion:    ev.RawTrap.Version,
		TrapOID:        ev.TrapOID,
		ResolvedName:   ev.ResolvedName,
		SeverityID:     decision.SeverityID,
		MatchedRuleID:  decision.MatchedRuleID,
		VarbindsJSON:   string(varbindsJSON),
		AggregationKey: aggKey,
		Suppressed:     suppressed,
		Unmapped:       ev.Unmapped,
	})
}

func (p *Processor) recordHeld(trapID int64) {
	if _, err := p.store.InsertNotification(store.Notification{TrapID: trapID, Status: notify.StatusHeld}); err != nil {
		p.log.Error("recording held notification", "trap_id", trapID, "err", err)
	}
}

func (p *Processor) recordNotification(trapID int64, r notify.Result) {
	chID := r.ChannelID
	n := store.Notification{TrapID: trapID, ChannelID: &chID, Status: r.Status, Attempts: r.Attempts}
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
	if p.metrics != nil {
		p.metrics.NotificationRecorded(chID, r.Status)
	}
}

// FloodFlush returns a rollup emitter that dispatches summary/digest messages to
// the rollup's channels. Wire this as the flood.Controller's flush callback.
func (p *Processor) FloodFlush(ctx context.Context) func(flood.Rollup) {
	return func(r flood.Rollup) {
		channels, err := p.store.ChannelsByIDs(r.ChannelIDs)
		if err != nil || len(channels) == 0 {
			return
		}
		msg := notify.Message{Title: r.Title, Body: r.Body}
		p.dispatcher.Dispatch(ctx, channels, msg)
		p.log.Info("flood rollup dispatched", "title", r.Title, "count", r.Count)
	}
}

func floodEvent(ev decode.Event, decision rules.Decision, aggKey string, sev *store.Severity) flood.Event {
	fe := flood.Event{Key: aggKey, EventName: ev.ResolvedName, ChannelIDs: decision.ChannelIDs}
	if sev != nil {
		fe.SeverityName = sev.Name
		fe.SeverityRank = sev.Rank
	}
	return fe
}

func buildMessage(ev decode.Event, sev *store.Severity) notify.Message {
	msg := notify.Message{
		Title: ev.ResolvedName,
		Body:  ev.Message,
		Fields: []notify.Field{
			{Name: "Source", Value: ev.RawTrap.SourceIP},
			{Name: "OID", Value: ev.TrapOID},
			{Name: "Time", Value: ev.RawTrap.ReceivedAt.Format(time.RFC3339)},
		},
	}
	if sev != nil {
		msg.Severity = sev.Name
		msg.Emoji = sev.Emoji
	}
	return msg
}

func severityName(sev *store.Severity) string {
	if sev == nil {
		return ""
	}
	return sev.Name
}

func outcomeName(o flood.Outcome) string {
	switch o {
	case flood.Suppress:
		return "suppress"
	case flood.Hold:
		return "hold"
	default:
		return "send"
	}
}

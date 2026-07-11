package rules

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func event(oid, sourceIP, msg string, sevID int64) decode.Event {
	return decode.Event{
		RawTrap:      snmp.RawTrap{SourceIP: sourceIP},
		TrapOID:      oid,
		ResolvedName: "evt",
		Message:      msg,
		SeverityID:   &sevID,
	}
}

func ptr(i int64) *int64 { return &i }

func TestNoRulesUsesDefaultRoutes(t *testing.T) {
	s := newStore(t)
	ch, _ := s.AddChannel("c", "shoutrrr", "x", true)
	s.SetDefaultRoutes(2, []int64{ch}) // High = id 2

	e := New(s)
	d, err := e.Classify(event("1.3.6.1.6.3.1.1.5.3", "10.0.0.1", "msg", 2))
	if err != nil {
		t.Fatal(err)
	}
	if d.Matched {
		t.Error("no rule should have matched")
	}
	if len(d.ChannelIDs) != 1 || d.ChannelIDs[0] != ch {
		t.Errorf("channels = %v, want default route [%d]", d.ChannelIDs, ch)
	}
}

func TestFirstMatchWinsAndAssignsSeverity(t *testing.T) {
	s := newStore(t)
	ch, _ := s.AddChannel("c", "shoutrrr", "x", true)
	// Rule assigns Critical (id 1) and routes to ch, terminal.
	s.CreateRule(store.Rule{Ord: 1, Name: "crit links", Enabled: true,
		MatchOIDGlob: "1.3.6.1.6.3.1.1.5.*", SeverityID: ptr(1), ChannelIDs: []int64{ch}})
	// Later rule would assign Low but must not be reached.
	s.CreateRule(store.Rule{Ord: 2, Name: "all", Enabled: true,
		MatchOIDGlob: "*", SeverityID: ptr(4), ChannelIDs: []int64{ch}})

	e := New(s)
	d, _ := e.Classify(event("1.3.6.1.6.3.1.1.5.3", "10.0.0.1", "msg", 2))
	if !d.Matched || d.SeverityID == nil || *d.SeverityID != 1 {
		t.Errorf("expected Critical severity, got %+v", d)
	}
}

func TestContinueOnMatchUnionsChannels(t *testing.T) {
	s := newStore(t)
	ch1, _ := s.AddChannel("c1", "shoutrrr", "x", true)
	ch2, _ := s.AddChannel("c2", "shoutrrr", "y", true)
	s.CreateRule(store.Rule{Ord: 1, Name: "first", Enabled: true, ContinueOnMatch: true,
		MatchOIDGlob: "*", SeverityID: ptr(2), ChannelIDs: []int64{ch1}})
	s.CreateRule(store.Rule{Ord: 2, Name: "second", Enabled: true,
		MatchOIDGlob: "*", ChannelIDs: []int64{ch2}})

	e := New(s)
	d, _ := e.Classify(event("1.2.3", "10.0.0.1", "msg", 5))
	got := append([]int64(nil), d.ChannelIDs...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != 2 || got[0] != ch1 || got[1] != ch2 {
		t.Errorf("channels = %v, want union [%d %d]", got, ch1, ch2)
	}
}

func TestDeviceAndRegexMatch(t *testing.T) {
	s := newStore(t)
	ch, _ := s.AddChannel("c", "shoutrrr", "x", true)
	devID, _ := s.CreateDevice(store.Device{SourceIP: "10.0.0.9", Name: "sfvh", Enabled: true})
	rx := "down"
	s.CreateRule(store.Rule{Ord: 1, Name: "dev+rx", Enabled: true,
		MatchDeviceID: &devID, MatchOIDGlob: "*", MatchVarbindRegex: &rx,
		SeverityID: ptr(1), ChannelIDs: []int64{ch}})

	e := New(s)
	// Wrong device: no match.
	if d, _ := e.Classify(event("1.2.3", "10.0.0.1", "Port down", 5)); d.Matched {
		t.Error("should not match different device")
	}
	// Right device but message lacks regex: no match.
	if d, _ := e.Classify(event("1.2.3", "10.0.0.9", "Port up", 5)); d.Matched {
		t.Error("should not match when regex absent")
	}
	// Right device + regex present: match.
	if d, _ := e.Classify(event("1.2.3", "10.0.0.9", "Port down", 5)); !d.Matched {
		t.Error("should match device + regex")
	}
}

func TestBypassFloodControl(t *testing.T) {
	s := newStore(t)
	ch, _ := s.AddChannel("c", "shoutrrr", "x", true)
	s.CreateRule(store.Rule{Ord: 1, Name: "crit", Enabled: true, BypassFloodControl: true,
		MatchOIDGlob: "*", SeverityID: ptr(1), ChannelIDs: []int64{ch}})
	e := New(s)
	d, _ := e.Classify(event("1.2.3", "10.0.0.1", "x", 5))
	if !d.BypassFloodControl {
		t.Error("expected bypass flag set")
	}
}

func TestDisabledRuleIgnored(t *testing.T) {
	s := newStore(t)
	ch, _ := s.AddChannel("c", "shoutrrr", "x", true)
	s.CreateRule(store.Rule{Ord: 1, Name: "off", Enabled: false,
		MatchOIDGlob: "*", SeverityID: ptr(1), ChannelIDs: []int64{ch}})
	e := New(s)
	d, _ := e.Classify(event("1.2.3", "10.0.0.1", "x", 5))
	if d.Matched {
		t.Error("disabled rule must not match")
	}
}

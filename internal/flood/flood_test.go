package flood

import (
	"testing"
	"time"
)

var base = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

func ev(key, name string) Event {
	return Event{Key: key, EventName: name, SeverityName: "High", SeverityRank: 2, ChannelIDs: []int64{1}}
}

func TestNoneAlwaysSends(t *testing.T) {
	c := New(Config{Strategy: StrategyNone}, nil)
	for i := 0; i < 5; i++ {
		if c.Admit(ev("k", "linkDown"), base) != Send {
			t.Fatal("none must always Send")
		}
	}
}

func TestDedupeSuppressesWithinWindow(t *testing.T) {
	c := New(Config{Strategy: StrategyDedupe, DedupeWindow: 30 * time.Second}, nil)
	if c.Admit(ev("k", "x"), base) != Send {
		t.Fatal("first should Send")
	}
	if c.Admit(ev("k", "x"), base.Add(10*time.Second)) != Suppress {
		t.Fatal("duplicate within window should Suppress")
	}
	if c.Admit(ev("k", "x"), base.Add(31*time.Second)) != Send {
		t.Fatal("after window should Send again")
	}
	// Different key is independent.
	if c.Admit(ev("other", "x"), base.Add(11*time.Second)) != Send {
		t.Fatal("different key should Send")
	}
}

func TestRateLimitHoldsAndSummarizes(t *testing.T) {
	var flushed []Rollup
	c := New(Config{Strategy: StrategyRateLimit, RateN: 2, RateWindow: time.Minute},
		func(r Rollup) { flushed = append(flushed, r) })

	// First 2 send, next 3 held.
	outcomes := []Outcome{}
	for i := 0; i < 5; i++ {
		outcomes = append(outcomes, c.Admit(ev("k", "linkDown"), base.Add(time.Duration(i)*time.Second)))
	}
	want := []Outcome{Send, Send, Hold, Hold, Hold}
	for i := range want {
		if outcomes[i] != want[i] {
			t.Fatalf("outcome[%d] = %v, want %v", i, outcomes[i], want[i])
		}
	}
	// Before the window closes: no summary.
	c.tick(base.Add(30 * time.Second))
	if len(flushed) != 0 {
		t.Fatal("summary emitted too early")
	}
	// After the window closes: one "+3 more" summary.
	c.tick(base.Add(61 * time.Second))
	if len(flushed) != 1 || flushed[0].Count != 3 {
		t.Fatalf("expected one summary of 3, got %+v", flushed)
	}
}

func TestDigestCollectsAndFlushes(t *testing.T) {
	var flushed []Rollup
	c := New(Config{Strategy: StrategyDigest, DigestInterval: 5 * time.Minute},
		func(r Rollup) { flushed = append(flushed, r) })

	c.Admit(ev("a", "linkDown"), base)
	c.Admit(ev("b", "linkDown"), base.Add(time.Second))
	c.Admit(Event{Key: "c", EventName: "coldStart", SeverityName: "Critical", SeverityRank: 1, ChannelIDs: []int64{2}}, base.Add(2*time.Second))

	// Before interval: nothing.
	c.tick(base.Add(time.Minute))
	if len(flushed) != 0 {
		t.Fatal("digest emitted too early")
	}
	// After interval: one rollup covering 3 events.
	c.tick(base.Add(5 * time.Minute))
	if len(flushed) != 1 || flushed[0].Count != 3 {
		t.Fatalf("expected digest of 3, got %+v", flushed)
	}
	// Grouped body: Critical (rank 1) listed before High.
	if got := flushed[0].Body; got == "" {
		t.Fatal("empty digest body")
	}
}

func TestConfigureSwitchesAtRuntime(t *testing.T) {
	c := New(Config{Strategy: StrategyNone}, nil)
	if c.Admit(ev("k", "x"), base) != Send {
		t.Fatal("none sends")
	}
	c.Configure(Config{Strategy: StrategyDedupe, DedupeWindow: time.Minute})
	if c.Admit(ev("k", "x"), base) != Send {
		t.Fatal("first after switch sends")
	}
	if c.Admit(ev("k", "x"), base.Add(time.Second)) != Suppress {
		t.Fatal("dedupe active after runtime switch")
	}
}

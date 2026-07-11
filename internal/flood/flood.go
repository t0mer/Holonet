// Package flood implements the user-selectable flood-control strategies
// (design §3.4): none, dedupe, rate_limit, and digest. Strategy and tunables
// live in settings and can change at runtime. The immediate per-event decision
// is synchronous (Admit); deferred rollups (rate-limit "+K more" summaries and
// digests) are emitted from tick, which Start drives on a ticker.
package flood

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Strategy names (stored in settings under flood.strategy).
const (
	StrategyNone      = "none"
	StrategyDedupe    = "dedupe"
	StrategyRateLimit = "rate_limit"
	StrategyDigest    = "digest"
)

// Outcome is the immediate decision for an event.
type Outcome int

const (
	Send     Outcome = iota // dispatch now
	Suppress                // deduped: persist + count, but do not notify
	Hold                    // represented later in a summary/digest rollup
)

// Event carries what flood control needs to decide and to render rollups.
type Event struct {
	Key          string  // aggregation key (source_ip + trap_oid [+ varbind hash])
	EventName    string  // resolved event name
	SeverityName string  // for digest grouping
	SeverityRank int     // for digest ordering
	ChannelIDs   []int64 // resolved destination channels
}

// Rollup is a summary or digest message the controller emits on tick.
type Rollup struct {
	Title      string
	Body       string
	ChannelIDs []int64
	Count      int
}

// Config holds the strategy and its tunables.
type Config struct {
	Strategy       string
	DedupeWindow   time.Duration
	RateN          int
	RateWindow     time.Duration
	DigestInterval time.Duration
}

// DefaultConfig is used when settings are missing.
func DefaultConfig() Config {
	return Config{
		Strategy:       StrategyNone,
		DedupeWindow:   30 * time.Second,
		RateN:          5,
		RateWindow:     time.Minute,
		DigestInterval: 5 * time.Minute,
	}
}

type rateBucket struct {
	windowStart time.Time
	sent        int
	held        []Event
}

// Controller applies the active flood-control strategy.
type Controller struct {
	mu       sync.Mutex
	cfg      Config
	lastSeen map[string]time.Time // dedupe
	buckets  map[string]*rateBucket
	digest   []Event
	digestAt time.Time // start of the current digest interval
	flush    func(Rollup)
}

// New builds a Controller with the given config and rollup emitter. flush is
// called (outside the lock) whenever a summary or digest is ready.
func New(cfg Config, flush func(Rollup)) *Controller {
	return &Controller{
		cfg:      cfg,
		lastSeen: map[string]time.Time{},
		buckets:  map[string]*rateBucket{},
		flush:    flush,
	}
}

// Configure swaps the strategy/tunables at runtime.
func (c *Controller) Configure(cfg Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
}

// Strategy returns the active strategy name.
func (c *Controller) Strategy() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.Strategy
}

// Admit returns the immediate decision for an event.
func (c *Controller) Admit(ev Event, now time.Time) Outcome {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.cfg.Strategy {
	case StrategyDedupe:
		if last, ok := c.lastSeen[ev.Key]; ok && now.Sub(last) < c.cfg.DedupeWindow {
			return Suppress
		}
		c.lastSeen[ev.Key] = now
		return Send

	case StrategyRateLimit:
		b := c.buckets[ev.Key]
		if b == nil || now.Sub(b.windowStart) >= c.cfg.RateWindow {
			b = &rateBucket{windowStart: now}
			c.buckets[ev.Key] = b
		}
		if b.sent < c.cfg.RateN {
			b.sent++
			return Send
		}
		b.held = append(b.held, ev)
		return Hold

	case StrategyDigest:
		if c.digestAt.IsZero() {
			c.digestAt = now
		}
		c.digest = append(c.digest, ev)
		return Hold

	default: // none
		return Send
	}
}

// tick emits any due rollups: closed rate-limit windows with held events, and
// the digest when its interval elapses. Returns the rollups to flush.
func (c *Controller) tick(now time.Time) []Rollup {
	c.mu.Lock()
	var out []Rollup

	if c.cfg.Strategy == StrategyRateLimit {
		for key, b := range c.buckets {
			if now.Sub(b.windowStart) >= c.cfg.RateWindow {
				if len(b.held) > 0 {
					out = append(out, summaryRollup(b.held))
				}
				delete(c.buckets, key)
			}
		}
	}

	if c.cfg.Strategy == StrategyDigest && !c.digestAt.IsZero() &&
		now.Sub(c.digestAt) >= c.cfg.DigestInterval && len(c.digest) > 0 {
		out = append(out, digestRollup(c.digest))
		c.digest = nil
		c.digestAt = now
	}
	c.mu.Unlock()

	for _, r := range out {
		if c.flush != nil {
			c.flush(r)
		}
	}
	return out
}

// Start runs tick on an interval until ctx is done.
func (c *Controller) Start(done <-chan struct{}, interval time.Duration, clock func() time.Time) {
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			c.tick(clock())
		}
	}
}

// summaryRollup builds a "+K more" summary for a rate-limited key.
func summaryRollup(held []Event) Rollup {
	name := held[0].EventName
	return Rollup{
		Title:      "Rate limit summary",
		Body:       fmt.Sprintf("+%d more %s suppressed during the rate-limit window", len(held), name),
		ChannelIDs: unionChannels(held),
		Count:      len(held),
	}
}

// digestRollup builds one rollup grouped by severity + event name.
func digestRollup(events []Event) Rollup {
	type key struct {
		sev  string
		rank int
		name string
	}
	counts := map[key]int{}
	for _, e := range events {
		counts[key{e.SeverityName, e.SeverityRank, e.EventName}]++
	}
	ks := make([]key, 0, len(counts))
	for k := range counts {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].rank != ks[j].rank {
			return ks[i].rank < ks[j].rank
		}
		return ks[i].name < ks[j].name
	})
	var b strings.Builder
	for _, k := range ks {
		sev := k.sev
		if sev == "" {
			sev = "Unclassified"
		}
		fmt.Fprintf(&b, "%s · %s ×%d\n", sev, k.name, counts[k])
	}
	return Rollup{
		Title:      fmt.Sprintf("Digest: %d events", len(events)),
		Body:       strings.TrimRight(b.String(), "\n"),
		ChannelIDs: unionChannels(events),
		Count:      len(events),
	}
}

func unionChannels(events []Event) []int64 {
	set := map[int64]struct{}{}
	for _, e := range events {
		for _, ch := range e.ChannelIDs {
			set[ch] = struct{}{}
		}
	}
	out := make([]int64, 0, len(set))
	for ch := range set {
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

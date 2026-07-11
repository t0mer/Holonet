// Package rules is the classification/routing engine (design §3.3). Rules are
// evaluated in ascending ord; the first match assigns severity and (unless it
// continues) terminates. Matching rules contribute their channel sets; routing
// falls back to the per-severity default routes.
package rules

import (
	"path"
	"regexp"

	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/store"
)

// dataSource is the slice of the store the engine reads.
type dataSource interface {
	ListRules() ([]store.Rule, error)
	DeviceByIP(ip string) (*store.Device, error)
	DefaultRouteChannels(severityID int64) ([]int64, error)
}

// Engine classifies and routes events against the current rule set.
type Engine struct {
	data dataSource
}

// New builds an Engine backed by the store.
func New(data dataSource) *Engine { return &Engine{data: data} }

// Decision is the engine's output for one event.
type Decision struct {
	SeverityID         *int64  // final severity (rule-assigned or oid_map default)
	MatchedRuleID      *int64  // first matching rule, if any
	ChannelIDs         []int64 // resolved, deduped destination channels
	BypassFloodControl bool    // true if any matching rule opts out of flood control
	Matched            bool    // whether any rule matched
}

// Classify evaluates the event against the ordered rule set and resolves routing.
func (e *Engine) Classify(ev decode.Event) (Decision, error) {
	d := Decision{SeverityID: ev.SeverityID} // default: oid_map severity from decode

	rulesList, err := e.data.ListRules()
	if err != nil {
		return Decision{}, err
	}

	// Resolve the device id for this source IP once (nil if unknown).
	var deviceID *int64
	if dev, err := e.data.DeviceByIP(ev.RawTrap.SourceIP); err == nil {
		deviceID = &dev.ID
	}

	channelSet := map[int64]struct{}{}
	for i := range rulesList {
		r := rulesList[i]
		if !r.Enabled {
			continue
		}
		if !ruleMatches(r, ev, deviceID) {
			continue
		}

		if !d.Matched {
			// First match sets severity and the matched-rule marker.
			d.Matched = true
			id := r.ID
			d.MatchedRuleID = &id
			if r.SeverityID != nil {
				d.SeverityID = r.SeverityID
			}
		}
		if r.BypassFloodControl {
			d.BypassFloodControl = true
		}

		// Contribute this rule's channels (explicit set, else default routes).
		for _, ch := range e.resolveChannels(r, d.SeverityID) {
			channelSet[ch] = struct{}{}
		}

		if !r.ContinueOnMatch {
			break
		}
	}

	if !d.Matched {
		// No rule matched: route by the default routes for the oid_map severity.
		if d.SeverityID != nil {
			for _, ch := range e.defaultRoutes(*d.SeverityID) {
				channelSet[ch] = struct{}{}
			}
		}
	}

	d.ChannelIDs = keys(channelSet)
	return d, nil
}

// resolveChannels returns a rule's explicit channels, or the default routes for
// the decision severity when the rule specifies none.
func (e *Engine) resolveChannels(r store.Rule, severityID *int64) []int64 {
	if len(r.ChannelIDs) > 0 {
		return r.ChannelIDs
	}
	if severityID != nil {
		return e.defaultRoutes(*severityID)
	}
	return nil
}

func (e *Engine) defaultRoutes(severityID int64) []int64 {
	ch, err := e.data.DefaultRouteChannels(severityID)
	if err != nil {
		return nil
	}
	return ch
}

// ruleMatches reports whether a rule matches the event.
func ruleMatches(r store.Rule, ev decode.Event, deviceID *int64) bool {
	// Device match: nil = any.
	if r.MatchDeviceID != nil {
		if deviceID == nil || *deviceID != *r.MatchDeviceID {
			return false
		}
	}
	// OID glob: "*" or empty = any.
	if !globMatch(r.MatchOIDGlob, ev.TrapOID) {
		return false
	}
	// Varbind regex against the composed message string.
	if r.MatchVarbindRegex != nil && *r.MatchVarbindRegex != "" {
		re, err := regexp.Compile(*r.MatchVarbindRegex)
		if err != nil || !re.MatchString(ev.Message) {
			return false
		}
	}
	return true
}

// globMatch matches an OID against a glob pattern using '*' wildcards. OIDs have
// no '/', so path.Match's '*' spans dotted segments cleanly.
func globMatch(pattern, oid string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	ok, err := path.Match(pattern, oid)
	if err != nil {
		return false
	}
	return ok
}

func keys(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

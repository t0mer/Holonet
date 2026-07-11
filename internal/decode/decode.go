// Package decode turns a raw trap into a normalized, classified Event: it
// resolves the trap OID against the OID map, extracts varbinds, assigns a
// default severity, and composes a human-readable message (design §3.2).
package decode

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

// resolver is the subset of the store the decoder needs.
type resolver interface {
	LookupOID(oid string) (*store.OIDEntry, error)
	GetSetting(key string) (string, error)
}

// Decoder resolves trap OIDs and default severities from the store.
type Decoder struct {
	store resolver
}

// New returns a Decoder backed by the given store.
func New(s resolver) *Decoder { return &Decoder{store: s} }

// Event is a decoded, classified trap ready for the rule engine (design §3.2).
type Event struct {
	RawTrap      snmp.RawTrap
	TrapOID      string
	ResolvedName string
	Varbinds     []snmp.Varbind
	Message      string
	SeverityID   *int64
	Unmapped     bool
}

// Decode resolves the trap OID, looks it up in the OID map, assigns a default
// severity, and builds the human message. Unmapped OIDs take the configured
// unknown-event default severity and are flagged for the UI to prompt a mapping.
func (d *Decoder) Decode(raw snmp.RawTrap) (Event, error) {
	trapOID := raw.TrapOID
	if trapOID == "" {
		trapOID = extractTrapOID(raw.Varbinds)
	}
	ev := Event{
		RawTrap:  raw,
		TrapOID:  trapOID,
		Varbinds: raw.Varbinds,
	}

	entry, err := d.store.LookupOID(trapOID)
	switch {
	case err == nil:
		ev.ResolvedName = entry.Name
		ev.SeverityID = entry.DefaultSeverityID
	case errors.Is(err, store.ErrNotFound):
		// Unmapped: name falls back to the raw OID, severity to the setting.
		ev.ResolvedName = trapOID
		ev.Unmapped = true
		if sev, sErr := d.unknownDefaultSeverity(); sErr == nil {
			ev.SeverityID = sev
		}
	default:
		return Event{}, fmt.Errorf("decode: lookup trap oid %q: %w", trapOID, err)
	}

	ev.Message = composeMessage(ev)
	return ev, nil
}

// unknownDefaultSeverity reads the configured default severity id for unmapped
// events. A missing/blank setting yields nil (no severity), never an error.
func (d *Decoder) unknownDefaultSeverity() (*int64, error) {
	v, err := d.store.GetSetting("unknown_default_severity_id")
	if errors.Is(err, store.ErrNotFound) || v == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, nil
	}
	return &id, nil
}

// extractTrapOID finds the snmpTrapOID.0 varbind value.
func extractTrapOID(vbs []snmp.Varbind) string {
	for _, vb := range vbs {
		if vb.OID == snmp.SnmpTrapOID {
			if s, ok := vb.Value.(string); ok {
				return s
			}
			return fmt.Sprintf("%v", vb.Value)
		}
	}
	return ""
}

// composeMessage prefers a Sophos-style message-text varbind, otherwise builds
// a summary from the event name, source, and the informative varbinds.
func composeMessage(ev Event) string {
	if msg := sophosMessageText(ev.Varbinds); msg != "" {
		return msg
	}
	var b strings.Builder
	b.WriteString(ev.ResolvedName)
	b.WriteString(" from ")
	b.WriteString(ev.RawTrap.SourceIP)
	extras := informativeVarbinds(ev.Varbinds)
	if len(extras) > 0 {
		b.WriteString(" [")
		b.WriteString(strings.Join(extras, ", "))
		b.WriteString("]")
	}
	return b.String()
}

// sophosMessageText returns the value of a plausible human-message varbind.
// Heuristic for slice 1: the longest OctetString value that isn't itself an OID
// or a bare number. Refined once the SFOS MIB is imported.
func sophosMessageText(vbs []snmp.Varbind) string {
	best := ""
	for _, vb := range vbs {
		if vb.OID == snmp.SnmpTrapOID || vb.OID == snmp.SysUpTime {
			continue
		}
		s, ok := vb.Value.(string)
		if !ok || len(s) < 8 {
			continue
		}
		if looksLikeOID(s) || isNumeric(s) {
			continue
		}
		if strings.ContainsAny(s, " :=") && len(s) > len(best) {
			best = s
		}
	}
	return best
}

// informativeVarbinds renders the non-structural varbinds compactly.
func informativeVarbinds(vbs []snmp.Varbind) []string {
	var out []string
	for _, vb := range vbs {
		if vb.OID == snmp.SnmpTrapOID || vb.OID == snmp.SysUpTime {
			continue
		}
		label := vb.Name
		if label == "" {
			label = vb.OID
		}
		out = append(out, fmt.Sprintf("%s=%v", label, vb.Value))
	}
	return out
}

func looksLikeOID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return strings.Contains(s, ".")
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

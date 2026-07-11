package snmp

import "time"

// Well-known OIDs used when decoding v2c/v3 traps.
const (
	// SnmpTrapOID is snmpTrapOID.0 — its value is the trap's identifying OID.
	SnmpTrapOID = "1.3.6.1.6.3.1.1.4.1.0"
	// SysUpTime is sysUpTime.0 — the first varbind of every v2c/v3 trap PDU.
	SysUpTime = "1.3.6.1.2.1.1.3.0"
)

// Varbind is one normalized variable binding from a trap PDU.
type Varbind struct {
	OID   string `json:"oid"`
	Name  string `json:"name,omitempty"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// RawTrap is a trap as received off the wire, before decoding/classification
// (design §3.1). TrapOID is best-effort populated by the sink when the
// snmpTrapOID.0 varbind is present; the decoder re-resolves it defensively.
type RawTrap struct {
	ReceivedAt time.Time
	SourceIP   string
	Version    string    // v2c | v3
	Community  string     // v2c community (plaintext, in-memory only)
	User       string     // v3 USM user name
	TrapOID    string
	Varbinds   []Varbind
}

package store

import "time"

// Severity is a priority level (design §3.3).
type Severity struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Rank      int    `json:"rank"`
	Color     string `json:"color"`
	Emoji     string `json:"emoji"`
	IsBuiltin bool   `json:"is_builtin"`
}

// OIDEntry is a row of the OID→event map (design §3.2).
type OIDEntry struct {
	ID                int64  `json:"id"`
	OID               string `json:"oid"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	DefaultSeverityID *int64 `json:"default_severity_id"`
	IsBuiltin         bool   `json:"is_builtin"`
}

// Channel is a notification destination (design §3.5). ConfigSealed is the
// AES-GCM sealed JSON config and is never serialized to API responses.
type Channel struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	ConfigSealed string `json:"-"`
	Enabled      bool   `json:"enabled"`
}

// Community is a v2c community string (sealed at rest).
type Community struct {
	ID              int64  `json:"id"`
	CommunitySealed string `json:"-"`
	Enabled         bool   `json:"enabled"`
}

// Trap is a received, decoded, and classified trap (design §3.6).
type Trap struct {
	ID             int64     `json:"id"`
	ReceivedAt     time.Time `json:"received_at"`
	SourceIP       string    `json:"source_ip"`
	SNMPVersion    string    `json:"snmp_version"`
	TrapOID        string    `json:"trap_oid"`
	ResolvedName   string    `json:"resolved_name"`
	SeverityID     *int64    `json:"severity_id"`
	MatchedRuleID  *int64    `json:"matched_rule_id"`
	VarbindsJSON   string    `json:"varbinds_json"`
	AggregationKey string    `json:"aggregation_key"`
	Suppressed     bool      `json:"suppressed"`
	Unmapped       bool      `json:"unmapped"`
}

// Notification is one dispatch attempt to one channel (design §3.6).
type Notification struct {
	ID        int64      `json:"id"`
	TrapID    int64      `json:"trap_id"`
	ChannelID *int64     `json:"channel_id"`
	Status    string     `json:"status"` // sent | failed | held
	Attempts  int        `json:"attempts"`
	LastError string     `json:"last_error"`
	SentAt    *time.Time `json:"sent_at"`
}

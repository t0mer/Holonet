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

// Device is a known trap source (design §3.6).
type Device struct {
	ID       int64  `json:"id"`
	SourceIP string `json:"source_ip"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
}

// Rule is an ordered classification/routing rule (design §3.3).
type Rule struct {
	ID                 int64   `json:"id"`
	Ord                int     `json:"ord"`
	Name               string  `json:"name"`
	Enabled            bool    `json:"enabled"`
	ContinueOnMatch    bool    `json:"continue_on_match"`
	BypassFloodControl bool    `json:"bypass_flood_control"`
	MatchDeviceID      *int64  `json:"match_device_id"`
	MatchOIDGlob       string  `json:"match_oid_glob"`
	MatchVarbindRegex  *string `json:"match_varbind_regex"`
	SeverityID         *int64  `json:"severity_id"`
	ChannelIDs         []int64 `json:"channel_ids"`
}

// TrapView is a trap enriched with joined display fields for the Events tab.
type TrapView struct {
	Trap
	SeverityName  *string `json:"severity_name"`
	SeverityColor *string `json:"severity_color"`
	SeverityEmoji *string `json:"severity_emoji"`
	DeviceName    *string `json:"device_name"`
	MatchedRule   *string `json:"matched_rule_name"`
}

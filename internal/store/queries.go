package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// tsFormat is the on-disk timestamp format: RFC3339 with nanoseconds in UTC,
// which sorts lexicographically for the Events tab (design §3.8, decision 9).
const tsFormat = "2006-01-02T15:04:05.000000000Z07:00"

func formatTS(t time.Time) string { return t.UTC().Format(tsFormat) }

func parseTS(s string) time.Time {
	// Accept both the nano format and the seconds format used by seed rows.
	for _, layout := range []string{tsFormat, "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("store: not found")

// GetSetting returns the value for key, or ErrNotFound.
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return v, nil
}

// SetSetting upserts a settings value.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}

// LookupOID returns the oid_map entry for oid, or ErrNotFound when unmapped.
func (s *Store) LookupOID(oid string) (*OIDEntry, error) {
	var e OIDEntry
	err := s.db.QueryRow(
		`SELECT id, oid, name, description, default_severity_id, is_builtin
		 FROM oid_map WHERE oid = ?`, oid).
		Scan(&e.ID, &e.OID, &e.Name, &e.Description, &e.DefaultSeverityID, &e.IsBuiltin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup oid %s: %w", oid, err)
	}
	return &e, nil
}

// GetSeverity returns a severity level by id, or ErrNotFound.
func (s *Store) GetSeverity(id int64) (*Severity, error) {
	var sev Severity
	err := s.db.QueryRow(
		`SELECT id, name, rank, color, emoji, is_builtin
		 FROM severity_levels WHERE id = ?`, id).
		Scan(&sev.ID, &sev.Name, &sev.Rank, &sev.Color, &sev.Emoji, &sev.IsBuiltin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get severity %d: %w", id, err)
	}
	return &sev, nil
}

// ListEnabledCommunities returns all enabled v2c community rows.
func (s *Store) ListEnabledCommunities() ([]Community, error) {
	rows, err := s.db.Query(`SELECT id, community_sealed, enabled FROM v2c_communities WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("list communities: %w", err)
	}
	defer rows.Close()
	var out []Community
	for rows.Next() {
		var c Community
		if err := rows.Scan(&c.ID, &c.CommunitySealed, &c.Enabled); err != nil {
			return nil, fmt.Errorf("scan community: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddCommunity inserts a sealed v2c community and returns its id.
func (s *Store) AddCommunity(sealed string, enabled bool) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO v2c_communities (community_sealed, enabled, created_at) VALUES (?, ?, ?)`,
		sealed, boolToInt(enabled), formatTS(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("add community: %w", err)
	}
	return res.LastInsertId()
}

// ListEnabledChannels returns all enabled channels.
func (s *Store) ListEnabledChannels() ([]Channel, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, config_sealed, enabled FROM channels WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.ConfigSealed, &c.Enabled); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddChannel inserts a channel with sealed config and returns its id.
func (s *Store) AddChannel(name, kind, configSealed string, enabled bool) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO channels (name, kind, config_sealed, enabled, created_at) VALUES (?, ?, ?, ?, ?)`,
		name, kind, configSealed, boolToInt(enabled), formatTS(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("add channel: %w", err)
	}
	return res.LastInsertId()
}

// InsertTrap persists a decoded/classified trap and returns its id.
func (s *Store) InsertTrap(t Trap) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO traps
		   (received_at, source_ip, snmp_version, trap_oid, resolved_name,
		    severity_id, matched_rule_id, varbinds_json, aggregation_key, suppressed, unmapped)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		formatTS(t.ReceivedAt), t.SourceIP, t.SNMPVersion, t.TrapOID, t.ResolvedName,
		t.SeverityID, t.MatchedRuleID, t.VarbindsJSON, t.AggregationKey,
		boolToInt(t.Suppressed), boolToInt(t.Unmapped))
	if err != nil {
		return 0, fmt.Errorf("insert trap: %w", err)
	}
	return res.LastInsertId()
}

// InsertNotification records one dispatch attempt and returns its id.
func (s *Store) InsertNotification(n Notification) (int64, error) {
	var sentAt any
	if n.SentAt != nil {
		sentAt = formatTS(*n.SentAt)
	}
	res, err := s.db.Exec(
		`INSERT INTO notifications (trap_id, channel_id, status, attempts, last_error, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.TrapID, n.ChannelID, n.Status, n.Attempts, n.LastError, sentAt)
	if err != nil {
		return 0, fmt.Errorf("insert notification: %w", err)
	}
	return res.LastInsertId()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

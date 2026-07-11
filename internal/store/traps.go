package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// sortableTrapColumns whitelists the server-side sort keys for the Events tab
// (design §3.8, decision 9). This is an allow-list — user input never reaches
// the SQL string directly.
var sortableTrapColumns = map[string]string{
	"received_at":   "t.received_at",
	"source_ip":     "t.source_ip",
	"resolved_name": "t.resolved_name",
	"severity":      "sev.rank",
	"matched_rule":  "r.name",
	"status":        "t.suppressed",
	"id":            "t.id",
}

const trapSelect = `
SELECT t.id, t.received_at, t.source_ip, t.snmp_version, t.trap_oid, t.resolved_name,
       t.severity_id, t.matched_rule_id, t.varbinds_json, t.aggregation_key, t.suppressed, t.unmapped,
       sev.name, sev.color, sev.emoji, d.name, r.name
FROM traps t
LEFT JOIN severity_levels sev ON sev.id = t.severity_id
LEFT JOIN devices d ON d.source_ip = t.source_ip
LEFT JOIN rules r ON r.id = t.matched_rule_id`

// ListTraps returns enriched trap rows sorted server-side. sortKey must be a key
// of sortableTrapColumns (defaults to received_at); order is asc|desc (default
// desc); limit caps rows (default 20, max 500).
func (s *Store) ListTraps(sortKey, order string, limit int) ([]TrapView, error) {
	col, ok := sortableTrapColumns[sortKey]
	if !ok {
		col = "t.received_at"
	}
	dir := "DESC"
	if strings.EqualFold(order, "asc") {
		dir = "ASC"
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	// col and dir come only from the whitelists above; limit is a bind param.
	q := fmt.Sprintf("%s ORDER BY %s %s, t.id DESC LIMIT ?", trapSelect, col, dir)
	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("list traps: %w", err)
	}
	defer rows.Close()
	var out []TrapView
	for rows.Next() {
		v, err := scanTrapView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetTrap returns one enriched trap by id, or ErrNotFound.
func (s *Store) GetTrap(id int64) (*TrapView, error) {
	row := s.db.QueryRow(trapSelect+" WHERE t.id = ?", id)
	v, err := scanTrapView(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func scanTrapView(sc scannable) (TrapView, error) {
	var v TrapView
	var receivedAt string
	err := sc.Scan(
		&v.ID, &receivedAt, &v.SourceIP, &v.SNMPVersion, &v.TrapOID, &v.ResolvedName,
		&v.SeverityID, &v.MatchedRuleID, &v.VarbindsJSON, &v.AggregationKey, &v.Suppressed, &v.Unmapped,
		&v.SeverityName, &v.SeverityColor, &v.SeverityEmoji, &v.DeviceName, &v.MatchedRule)
	if err != nil {
		return TrapView{}, err
	}
	v.ReceivedAt = parseTS(receivedAt)
	return v, nil
}

// CountTrapsBySeverity returns severity name → count for the dashboard.
func (s *Store) CountTrapsBySeverity() (map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT COALESCE(sev.name,'Unclassified') AS name, COUNT(*)
		 FROM traps t LEFT JOIN severity_levels sev ON sev.id=t.severity_id
		 GROUP BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

// CountTraps returns the total number of stored traps.
func (s *Store) CountTraps() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM traps`).Scan(&n)
	return n, err
}

// CountNotifications returns status → count across all notifications.
func (s *Store) CountNotifications() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM notifications GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

// ListNotificationsForTrap returns the dispatch attempts for a trap.
func (s *Store) ListNotificationsForTrap(trapID int64) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT id, trap_id, channel_id, status, attempts, last_error, sent_at
		 FROM notifications WHERE trap_id=? ORDER BY id`, trapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		var sentAt sql.NullString
		if err := rows.Scan(&n.ID, &n.TrapID, &n.ChannelID, &n.Status, &n.Attempts, &n.LastError, &sentAt); err != nil {
			return nil, err
		}
		if sentAt.Valid {
			t := parseTS(sentAt.String)
			n.SentAt = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateTrapClassification updates the severity/rule/suppressed fields after a
// (re)classification, e.g. during replay.
func (s *Store) UpdateTrapClassification(id int64, severityID, matchedRuleID *int64, suppressed bool) error {
	_, err := s.db.Exec(
		`UPDATE traps SET severity_id=?, matched_rule_id=?, suppressed=? WHERE id=?`,
		severityID, matchedRuleID, boolToInt(suppressed), id)
	return err
}

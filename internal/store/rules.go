package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ListRules returns all rules ordered by ord, each with its channel set.
func (s *Store) ListRules() ([]Rule, error) {
	rows, err := s.db.Query(
		`SELECT id, ord, name, enabled, continue_on_match, bypass_flood_control,
		        match_device_id, match_oid_glob, match_varbind_regex, severity_id
		 FROM rules ORDER BY ord`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Attach channel sets.
	for i := range out {
		ch, err := s.ruleChannels(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].ChannelIDs = ch
	}
	return out, nil
}

// GetRule returns one rule with its channel set, or ErrNotFound.
func (s *Store) GetRule(id int64) (*Rule, error) {
	row := s.db.QueryRow(
		`SELECT id, ord, name, enabled, continue_on_match, bypass_flood_control,
		        match_device_id, match_oid_glob, match_varbind_regex, severity_id
		 FROM rules WHERE id=?`, id)
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ch, err := s.ruleChannels(id)
	if err != nil {
		return nil, err
	}
	r.ChannelIDs = ch
	return &r, nil
}

// scannable is implemented by *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanRule(sc scannable) (Rule, error) {
	var r Rule
	err := sc.Scan(&r.ID, &r.Ord, &r.Name, &r.Enabled, &r.ContinueOnMatch, &r.BypassFloodControl,
		&r.MatchDeviceID, &r.MatchOIDGlob, &r.MatchVarbindRegex, &r.SeverityID)
	return r, err
}

func (s *Store) ruleChannels(ruleID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT channel_id FROM rule_channels WHERE rule_id=?`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var ch int64
		if err := rows.Scan(&ch); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// CreateRule inserts a rule and its channel associations in one transaction.
func (s *Store) CreateRule(r Rule) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(
		`INSERT INTO rules
		   (ord, name, enabled, continue_on_match, bypass_flood_control,
		    match_device_id, match_oid_glob, match_varbind_regex, severity_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Ord, r.Name, boolToInt(r.Enabled), boolToInt(r.ContinueOnMatch), boolToInt(r.BypassFloodControl),
		r.MatchDeviceID, defaultGlob(r.MatchOIDGlob), r.MatchVarbindRegex, r.SeverityID, formatTS(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("create rule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := setRuleChannels(tx, id, r.ChannelIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateRule updates a rule and replaces its channel associations.
func (s *Store) UpdateRule(r Rule) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE rules SET ord=?, name=?, enabled=?, continue_on_match=?, bypass_flood_control=?,
		   match_device_id=?, match_oid_glob=?, match_varbind_regex=?, severity_id=? WHERE id=?`,
		r.Ord, r.Name, boolToInt(r.Enabled), boolToInt(r.ContinueOnMatch), boolToInt(r.BypassFloodControl),
		r.MatchDeviceID, defaultGlob(r.MatchOIDGlob), r.MatchVarbindRegex, r.SeverityID, r.ID); err != nil {
		return fmt.Errorf("update rule: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM rule_channels WHERE rule_id=?`, r.ID); err != nil {
		return err
	}
	if err := setRuleChannels(tx, r.ID, r.ChannelIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteRule removes a rule (channel associations cascade).
func (s *Store) DeleteRule(id int64) error {
	_, err := s.db.Exec(`DELETE FROM rules WHERE id=?`, id)
	return err
}

// ReorderRules assigns new ord values from the given id order. It offsets
// existing ords first to avoid tripping the UNIQUE(ord) constraint mid-update.
func (s *Store) ReorderRules(orderedIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Move current ords out of the target range to dodge the unique constraint.
	if _, err := tx.Exec(`UPDATE rules SET ord = ord + 1000000`); err != nil {
		return err
	}
	for i, id := range orderedIDs {
		if _, err := tx.Exec(`UPDATE rules SET ord=? WHERE id=?`, i, id); err != nil {
			return fmt.Errorf("reorder rule %d: %w", id, err)
		}
	}
	return tx.Commit()
}

func setRuleChannels(tx *sql.Tx, ruleID int64, channelIDs []int64) error {
	for _, ch := range channelIDs {
		if _, err := tx.Exec(`INSERT INTO rule_channels (rule_id, channel_id) VALUES (?, ?)`, ruleID, ch); err != nil {
			return fmt.Errorf("attach channel %d to rule %d: %w", ch, ruleID, err)
		}
	}
	return nil
}

func defaultGlob(g string) string {
	if g == "" {
		return "*"
	}
	return g
}

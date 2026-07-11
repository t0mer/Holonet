package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ---- Severity levels ----

// ListSeverities returns all severity levels ordered by rank.
func (s *Store) ListSeverities() ([]Severity, error) {
	rows, err := s.db.Query(`SELECT id, name, rank, color, emoji, is_builtin FROM severity_levels ORDER BY rank`)
	if err != nil {
		return nil, fmt.Errorf("list severities: %w", err)
	}
	defer rows.Close()
	var out []Severity
	for rows.Next() {
		var v Severity
		if err := rows.Scan(&v.ID, &v.Name, &v.Rank, &v.Color, &v.Emoji, &v.IsBuiltin); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateSeverity inserts a custom severity level.
func (s *Store) CreateSeverity(v Severity) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO severity_levels (name, rank, color, emoji, is_builtin) VALUES (?, ?, ?, ?, 0)`,
		v.Name, v.Rank, v.Color, v.Emoji)
	if err != nil {
		return 0, fmt.Errorf("create severity: %w", err)
	}
	return res.LastInsertId()
}

// UpdateSeverity updates a severity's editable fields (name, rank, color, emoji).
func (s *Store) UpdateSeverity(v Severity) error {
	_, err := s.db.Exec(
		`UPDATE severity_levels SET name=?, rank=?, color=?, emoji=? WHERE id=?`,
		v.Name, v.Rank, v.Color, v.Emoji, v.ID)
	if err != nil {
		return fmt.Errorf("update severity: %w", err)
	}
	return nil
}

// DeleteSeverity removes a custom severity. Built-ins cannot be deleted.
func (s *Store) DeleteSeverity(id int64) error {
	var builtin bool
	err := s.db.QueryRow(`SELECT is_builtin FROM severity_levels WHERE id=?`, id).Scan(&builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if builtin {
		return errors.New("store: cannot delete a built-in severity level")
	}
	_, err = s.db.Exec(`DELETE FROM severity_levels WHERE id=?`, id)
	return err
}

// ---- OID map ----

// ListOIDMap returns all OID entries ordered by OID.
func (s *Store) ListOIDMap() ([]OIDEntry, error) {
	rows, err := s.db.Query(`SELECT id, oid, name, description, default_severity_id, is_builtin FROM oid_map ORDER BY oid`)
	if err != nil {
		return nil, fmt.Errorf("list oid map: %w", err)
	}
	defer rows.Close()
	var out []OIDEntry
	for rows.Next() {
		var e OIDEntry
		if err := rows.Scan(&e.ID, &e.OID, &e.Name, &e.Description, &e.DefaultSeverityID, &e.IsBuiltin); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertOID inserts or updates an OID map entry keyed by OID.
func (s *Store) UpsertOID(e OIDEntry) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO oid_map (oid, name, description, default_severity_id, is_builtin, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(oid) DO UPDATE SET
		   name=excluded.name, description=excluded.description,
		   default_severity_id=excluded.default_severity_id, updated_at=excluded.updated_at`,
		e.OID, e.Name, e.Description, e.DefaultSeverityID, boolToInt(e.IsBuiltin), formatTS(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("upsert oid: %w", err)
	}
	return res.LastInsertId()
}

// UpdateOID updates an existing OID entry by id.
func (s *Store) UpdateOID(e OIDEntry) error {
	_, err := s.db.Exec(
		`UPDATE oid_map SET oid=?, name=?, description=?, default_severity_id=?, updated_at=? WHERE id=?`,
		e.OID, e.Name, e.Description, e.DefaultSeverityID, formatTS(time.Now()), e.ID)
	if err != nil {
		return fmt.Errorf("update oid: %w", err)
	}
	return nil
}

// DeleteOID removes an OID entry.
func (s *Store) DeleteOID(id int64) error {
	_, err := s.db.Exec(`DELETE FROM oid_map WHERE id=?`, id)
	return err
}

// ---- Devices ----

// ListDevices returns all devices.
func (s *Store) ListDevices() ([]Device, error) {
	rows, err := s.db.Query(`SELECT id, source_ip, name, enabled FROM devices ORDER BY source_ip`)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.SourceIP, &d.Name, &d.Enabled); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateDevice inserts a device.
func (s *Store) CreateDevice(d Device) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO devices (source_ip, name, enabled, created_at) VALUES (?, ?, ?, ?)`,
		d.SourceIP, d.Name, boolToInt(d.Enabled), formatTS(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("create device: %w", err)
	}
	return res.LastInsertId()
}

// UpdateDevice updates a device.
func (s *Store) UpdateDevice(d Device) error {
	_, err := s.db.Exec(`UPDATE devices SET source_ip=?, name=?, enabled=? WHERE id=?`,
		d.SourceIP, d.Name, boolToInt(d.Enabled), d.ID)
	return err
}

// DeleteDevice removes a device.
func (s *Store) DeleteDevice(id int64) error {
	_, err := s.db.Exec(`DELETE FROM devices WHERE id=?`, id)
	return err
}

// DeviceByIP returns the device for a source IP, or ErrNotFound.
func (s *Store) DeviceByIP(ip string) (*Device, error) {
	var d Device
	err := s.db.QueryRow(`SELECT id, source_ip, name, enabled FROM devices WHERE source_ip=?`, ip).
		Scan(&d.ID, &d.SourceIP, &d.Name, &d.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ---- Channels (extends queries.go) ----

// ListChannels returns all channels regardless of enabled state.
func (s *Store) ListChannels() ([]Channel, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, config_sealed, enabled FROM channels ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.ConfigSealed, &c.Enabled); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChannel returns one channel by id, or ErrNotFound.
func (s *Store) GetChannel(id int64) (*Channel, error) {
	var c Channel
	err := s.db.QueryRow(`SELECT id, name, kind, config_sealed, enabled FROM channels WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Kind, &c.ConfigSealed, &c.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateChannel updates a channel. If configSealed is empty the existing config
// is preserved (write-only secret semantics).
func (s *Store) UpdateChannel(c Channel) error {
	if c.ConfigSealed == "" {
		_, err := s.db.Exec(`UPDATE channels SET name=?, kind=?, enabled=? WHERE id=?`,
			c.Name, c.Kind, boolToInt(c.Enabled), c.ID)
		return err
	}
	_, err := s.db.Exec(`UPDATE channels SET name=?, kind=?, config_sealed=?, enabled=? WHERE id=?`,
		c.Name, c.Kind, c.ConfigSealed, boolToInt(c.Enabled), c.ID)
	return err
}

// DeleteChannel removes a channel (cascades to rule_channels/default_routes).
func (s *Store) DeleteChannel(id int64) error {
	_, err := s.db.Exec(`DELETE FROM channels WHERE id=?`, id)
	return err
}

// ---- Default routes ----

// ListDefaultRoutes returns severity_id → []channel_id mappings.
func (s *Store) ListDefaultRoutes() (map[int64][]int64, error) {
	rows, err := s.db.Query(`SELECT severity_id, channel_id FROM default_routes`)
	if err != nil {
		return nil, fmt.Errorf("list default routes: %w", err)
	}
	defer rows.Close()
	out := make(map[int64][]int64)
	for rows.Next() {
		var sev, ch int64
		if err := rows.Scan(&sev, &ch); err != nil {
			return nil, err
		}
		out[sev] = append(out[sev], ch)
	}
	return out, rows.Err()
}

// SetDefaultRoutes replaces the channel set routed to a severity by default.
func (s *Store) SetDefaultRoutes(severityID int64, channelIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM default_routes WHERE severity_id=?`, severityID); err != nil {
		return err
	}
	for _, ch := range channelIDs {
		if _, err := tx.Exec(`INSERT INTO default_routes (severity_id, channel_id) VALUES (?, ?)`, severityID, ch); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DefaultRouteChannels returns the channel ids routed to a severity by default.
func (s *Store) DefaultRouteChannels(severityID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT channel_id FROM default_routes WHERE severity_id=?`, severityID)
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

// ---- Communities (extends queries.go) ----

// ListCommunities returns all v2c communities (sealed, never plaintext to API).
func (s *Store) ListCommunities() ([]Community, error) {
	rows, err := s.db.Query(`SELECT id, community_sealed, enabled FROM v2c_communities ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Community
	for rows.Next() {
		var c Community
		if err := rows.Scan(&c.ID, &c.CommunitySealed, &c.Enabled); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCommunityEnabled toggles a community.
func (s *Store) SetCommunityEnabled(id int64, enabled bool) error {
	_, err := s.db.Exec(`UPDATE v2c_communities SET enabled=? WHERE id=?`, boolToInt(enabled), id)
	return err
}

// DeleteCommunity removes a community.
func (s *Store) DeleteCommunity(id int64) error {
	_, err := s.db.Exec(`DELETE FROM v2c_communities WHERE id=?`, id)
	return err
}

// ---- Admin ----

// GetAdmin returns the single admin account, or ErrNotFound if none exists yet.
func (s *Store) GetAdmin() (username, passHash string, err error) {
	err = s.db.QueryRow(`SELECT username, pass_hash FROM admin ORDER BY id LIMIT 1`).Scan(&username, &passHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return username, passHash, err
}

// CreateAdmin inserts the admin account.
func (s *Store) CreateAdmin(username, passHash string) error {
	_, err := s.db.Exec(`INSERT INTO admin (username, pass_hash, created_at) VALUES (?, ?, ?)`,
		username, passHash, formatTS(time.Now()))
	return err
}

// UpdateAdminPassword updates the admin password hash.
func (s *Store) UpdateAdminPassword(username, passHash string) error {
	_, err := s.db.Exec(`UPDATE admin SET pass_hash=? WHERE username=?`, passHash, username)
	return err
}

// ---- Settings ----

// AllSettings returns every settings key/value.
func (s *Store) AllSettings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

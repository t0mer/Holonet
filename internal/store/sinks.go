package store

import (
	"fmt"
	"time"
)

const v3UserSelect = `SELECT id, username, security_level, auth_protocol, auth_pass_sealed,
	priv_protocol, priv_pass_sealed, engine_id, enabled FROM snmpv3_users`

// ListV3Users returns all SNMPv3 users (sealed passwords, never to the API).
func (s *Store) ListV3Users() ([]SNMPv3User, error) {
	return s.queryV3Users(v3UserSelect + ` ORDER BY id`)
}

// ListEnabledV3Users returns the enabled SNMPv3 users for the trap sink.
func (s *Store) ListEnabledV3Users() ([]SNMPv3User, error) {
	return s.queryV3Users(v3UserSelect + ` WHERE enabled=1 ORDER BY id`)
}

// GetV3User returns one v3 user, or ErrNotFound.
func (s *Store) GetV3User(id int64) (*SNMPv3User, error) {
	users, err := s.queryV3Users(v3UserSelect+` WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, ErrNotFound
	}
	return &users[0], nil
}

func (s *Store) queryV3Users(q string, args ...any) ([]SNMPv3User, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query v3 users: %w", err)
	}
	defer rows.Close()
	var out []SNMPv3User
	for rows.Next() {
		var u SNMPv3User
		if err := rows.Scan(&u.ID, &u.Username, &u.SecurityLevel, &u.AuthProtocol, &u.AuthPassSealed,
			&u.PrivProtocol, &u.PrivPassSealed, &u.EngineID, &u.Enabled); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CreateV3User inserts a v3 user with sealed passwords.
func (s *Store) CreateV3User(u SNMPv3User) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO snmpv3_users
		   (username, security_level, auth_protocol, auth_pass_sealed,
		    priv_protocol, priv_pass_sealed, engine_id, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.SecurityLevel, u.AuthProtocol, u.AuthPassSealed,
		u.PrivProtocol, u.PrivPassSealed, u.EngineID, boolToInt(u.Enabled), formatTS(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("create v3 user: %w", err)
	}
	return res.LastInsertId()
}

// UpdateV3User updates a v3 user. Empty *Sealed fields preserve the stored
// secret (write-only credential semantics).
func (s *Store) UpdateV3User(u SNMPv3User) error {
	set := `username=?, security_level=?, auth_protocol=?, priv_protocol=?, engine_id=?, enabled=?`
	args := []any{u.Username, u.SecurityLevel, u.AuthProtocol, u.PrivProtocol, u.EngineID, boolToInt(u.Enabled)}
	if u.AuthPassSealed != "" {
		set += `, auth_pass_sealed=?`
		args = append(args, u.AuthPassSealed)
	}
	if u.PrivPassSealed != "" {
		set += `, priv_pass_sealed=?`
		args = append(args, u.PrivPassSealed)
	}
	args = append(args, u.ID)
	if _, err := s.db.Exec(`UPDATE snmpv3_users SET `+set+` WHERE id=?`, args...); err != nil {
		return fmt.Errorf("update v3 user: %w", err)
	}
	return nil
}

// DeleteV3User removes a v3 user.
func (s *Store) DeleteV3User(id int64) error {
	_, err := s.db.Exec(`DELETE FROM snmpv3_users WHERE id=?`, id)
	return err
}

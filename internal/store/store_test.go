package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationsSeedSeverities(t *testing.T) {
	s := openTemp(t)
	for name, wantRank := range map[string]int{"Critical": 1, "High": 2, "Medium": 3, "Low": 4, "Info": 5} {
		var rank int
		var builtin bool
		err := s.db.QueryRow(`SELECT rank, is_builtin FROM severity_levels WHERE name = ?`, name).Scan(&rank, &builtin)
		if err != nil {
			t.Fatalf("severity %s: %v", name, err)
		}
		if rank != wantRank {
			t.Errorf("%s rank = %d, want %d", name, rank, wantRank)
		}
		if !builtin {
			t.Errorf("%s should be builtin", name)
		}
	}
}

func TestMigrationsSeedGenericOIDs(t *testing.T) {
	s := openTemp(t)
	e, err := s.LookupOID("1.3.6.1.6.3.1.1.5.3")
	if err != nil {
		t.Fatalf("LookupOID linkDown: %v", err)
	}
	if e.Name != "linkDown" {
		t.Errorf("name = %q, want linkDown", e.Name)
	}
	if e.DefaultSeverityID == nil {
		t.Fatal("linkDown should have a default severity")
	}
	sev, err := s.GetSeverity(*e.DefaultSeverityID)
	if err != nil {
		t.Fatalf("GetSeverity: %v", err)
	}
	if sev.Name != "High" {
		t.Errorf("linkDown severity = %q, want High", sev.Name)
	}
}

func TestLookupOIDUnmapped(t *testing.T) {
	s := openTemp(t)
	if _, err := s.LookupOID("1.2.3.4.5.6.7.8.9"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idem.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()
	s2, err := Open(path) // re-running migrations must not error or duplicate seed
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()
	var count int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM severity_levels`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Errorf("severity count = %d, want 5 (seed ran twice?)", count)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := openTemp(t)
	got, err := s.GetSetting("snmp.bind_addr")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "0.0.0.0:1162" {
		t.Errorf("bind_addr = %q", got)
	}
	if err := s.SetSetting("snmp.bind_addr", "0.0.0.0:1620"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	got, _ = s.GetSetting("snmp.bind_addr")
	if got != "0.0.0.0:1620" {
		t.Errorf("after update bind_addr = %q", got)
	}
}

func TestInsertTrapAndNotification(t *testing.T) {
	s := openTemp(t)
	sevID := int64(2)
	trapID, err := s.InsertTrap(Trap{
		ReceivedAt:   time.Now(),
		SourceIP:     "10.0.0.1",
		SNMPVersion:  "v2c",
		TrapOID:      "1.3.6.1.6.3.1.1.5.3",
		ResolvedName: "linkDown",
		SeverityID:   &sevID,
		VarbindsJSON: `[{"oid":"x","value":"y"}]`,
	})
	if err != nil {
		t.Fatalf("InsertTrap: %v", err)
	}
	if trapID == 0 {
		t.Fatal("expected non-zero trap id")
	}
	now := time.Now()
	chID := int64(0) // no channel row; nullable FK stays satisfied via nil below
	_ = chID
	if _, err := s.InsertNotification(Notification{
		TrapID: trapID, Status: "sent", Attempts: 1, SentAt: &now,
	}); err != nil {
		t.Fatalf("InsertNotification: %v", err)
	}
}

package store

import (
	"testing"
	"time"
)

func TestDeleteBuiltinSeverityRejected(t *testing.T) {
	s := openTemp(t)
	sevs, _ := s.ListSeverities()
	var criticalID int64
	for _, v := range sevs {
		if v.Name == "Critical" {
			criticalID = v.ID
		}
	}
	if err := s.DeleteSeverity(criticalID); err == nil {
		t.Fatal("expected error deleting a built-in severity")
	}
}

func TestCustomSeverityLifecycle(t *testing.T) {
	s := openTemp(t)
	id, err := s.CreateSeverity(Severity{Name: "Notice", Rank: 6, Color: "#111", Emoji: "🔔"})
	if err != nil {
		t.Fatalf("CreateSeverity: %v", err)
	}
	if err := s.DeleteSeverity(id); err != nil {
		t.Fatalf("DeleteSeverity custom: %v", err)
	}
}

func TestRuleCRUDAndChannels(t *testing.T) {
	s := openTemp(t)
	chID, _ := s.AddChannel("c1", "shoutrrr", "sealed", true)
	sev := int64(2)
	glob := "1.3.6.1.4.1.*"
	id, err := s.CreateRule(Rule{
		Ord: 1, Name: "link events", Enabled: true, MatchOIDGlob: glob,
		SeverityID: &sev, ChannelIDs: []int64{chID},
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	got, err := s.GetRule(id)
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if got.MatchOIDGlob != glob || len(got.ChannelIDs) != 1 || got.ChannelIDs[0] != chID {
		t.Errorf("rule mismatch: %+v", got)
	}
	got.Name = "renamed"
	got.ChannelIDs = nil
	if err := s.UpdateRule(*got); err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	after, _ := s.GetRule(id)
	if after.Name != "renamed" || len(after.ChannelIDs) != 0 {
		t.Errorf("update not applied: %+v", after)
	}
}

func TestReorderRules(t *testing.T) {
	s := openTemp(t)
	a, _ := s.CreateRule(Rule{Ord: 1, Name: "a", Enabled: true})
	b, _ := s.CreateRule(Rule{Ord: 2, Name: "b", Enabled: true})
	c, _ := s.CreateRule(Rule{Ord: 3, Name: "c", Enabled: true})
	if err := s.ReorderRules([]int64{c, a, b}); err != nil {
		t.Fatalf("ReorderRules: %v", err)
	}
	rules, _ := s.ListRules()
	gotOrder := []int64{rules[0].ID, rules[1].ID, rules[2].ID}
	want := []int64{c, a, b}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Fatalf("order = %v, want %v", gotOrder, want)
		}
	}
}

func TestDefaultRoutes(t *testing.T) {
	s := openTemp(t)
	ch1, _ := s.AddChannel("c1", "shoutrrr", "x", true)
	ch2, _ := s.AddChannel("c2", "shoutrrr", "y", true)
	if err := s.SetDefaultRoutes(1, []int64{ch1, ch2}); err != nil {
		t.Fatalf("SetDefaultRoutes: %v", err)
	}
	got, _ := s.DefaultRouteChannels(1)
	if len(got) != 2 {
		t.Errorf("default routes = %v", got)
	}
	// Replace with one.
	s.SetDefaultRoutes(1, []int64{ch2})
	got, _ = s.DefaultRouteChannels(1)
	if len(got) != 1 || got[0] != ch2 {
		t.Errorf("after replace = %v", got)
	}
}

func TestListTrapsSortWhitelistAndLimit(t *testing.T) {
	s := openTemp(t)
	sev := int64(1)
	for i := 0; i < 3; i++ {
		s.InsertTrap(Trap{ReceivedAt: time.Now().Add(time.Duration(i) * time.Second),
			SourceIP: "10.0.0.1", SNMPVersion: "v2c", TrapOID: "1.2.3", ResolvedName: "e", SeverityID: &sev})
	}
	// Unknown sort key must fall back safely (no error, defaults applied).
	rows, err := s.ListTraps("'; DROP TABLE traps;--", "asc", 2)
	if err != nil {
		t.Fatalf("ListTraps: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("limit not applied: got %d", len(rows))
	}
	// Table still intact.
	if n, _ := s.CountTraps(); n != 3 {
		t.Errorf("trap count = %d, want 3 (injection?)", n)
	}
}

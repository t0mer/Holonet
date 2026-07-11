package decode

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
)

func newDecoder(t *testing.T) *Decoder {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s)
}

func raw(trapOID string, vbs ...snmp.Varbind) snmp.RawTrap {
	return snmp.RawTrap{
		ReceivedAt: time.Now(),
		SourceIP:   "192.0.2.10",
		Version:    "v2c",
		Community:  "public",
		TrapOID:    trapOID,
		Varbinds:   vbs,
	}
}

func TestDecodeMappedOID(t *testing.T) {
	d := newDecoder(t)
	ev, err := d.Decode(raw("", snmp.Varbind{OID: snmp.SnmpTrapOID, Type: "OID", Value: "1.3.6.1.6.3.1.1.5.3"}))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ev.TrapOID != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("TrapOID = %q", ev.TrapOID)
	}
	if ev.ResolvedName != "linkDown" {
		t.Errorf("ResolvedName = %q, want linkDown", ev.ResolvedName)
	}
	if ev.Unmapped {
		t.Error("mapped OID should not be flagged unmapped")
	}
	if ev.SeverityID == nil {
		t.Error("expected default severity for linkDown")
	}
}

func TestDecodeUnmappedOID(t *testing.T) {
	d := newDecoder(t)
	ev, err := d.Decode(raw("", snmp.Varbind{OID: snmp.SnmpTrapOID, Type: "OID", Value: "1.3.6.1.4.1.99999.1.2.3"}))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !ev.Unmapped {
		t.Error("unknown OID should be flagged unmapped")
	}
	if ev.ResolvedName != "1.3.6.1.4.1.99999.1.2.3" {
		t.Errorf("unmapped name should fall back to OID, got %q", ev.ResolvedName)
	}
	// Seeded unknown_default_severity_id points at Info.
	if ev.SeverityID == nil {
		t.Error("expected unknown-default severity")
	}
}

func TestDecodeUsesRawTrapOIDWhenNoVarbind(t *testing.T) {
	d := newDecoder(t)
	r := raw("1.3.6.1.6.3.1.1.5.1")
	ev, err := d.Decode(r)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ev.ResolvedName != "coldStart" {
		t.Errorf("ResolvedName = %q, want coldStart", ev.ResolvedName)
	}
}

func TestComposeMessagePrefersSophosText(t *testing.T) {
	d := newDecoder(t)
	ev, err := d.Decode(raw("",
		snmp.Varbind{OID: snmp.SnmpTrapOID, Type: "OID", Value: "1.3.6.1.6.3.1.1.5.3"},
		snmp.Varbind{OID: "1.3.6.1.4.1.2604.5.1.1", Type: "OctetString", Value: "Interface Port2 link is down"},
	))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ev.Message != "Interface Port2 link is down" {
		t.Errorf("Message = %q, want Sophos text", ev.Message)
	}
}

func TestComposeMessageFallback(t *testing.T) {
	d := newDecoder(t)
	ev, err := d.Decode(raw("",
		snmp.Varbind{OID: snmp.SnmpTrapOID, Type: "OID", Value: "1.3.6.1.6.3.1.1.5.4"},
		snmp.Varbind{OID: "1.3.6.1.2.1.2.2.1.1", Type: "Integer", Value: 3},
	))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := "linkUp from 192.0.2.10 [1.3.6.1.2.1.2.2.1.1=3]"
	if ev.Message != want {
		t.Errorf("Message = %q, want %q", ev.Message, want)
	}
}

package snmp

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/gosnmp/gosnmp"
)

type countMetrics struct {
	received, auth, panics int32
}

func (c *countMetrics) TrapReceived(string, string) { atomic.AddInt32(&c.received, 1) }
func (c *countMetrics) AuthFailure()                { atomic.AddInt32(&c.auth, 1) }
func (c *countMetrics) DecodePanic()                { atomic.AddInt32(&c.panics, 1) }

func v2cPacket(community string) *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: community,
		Variables: []gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(12345)},
			{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.6.3.1.1.5.3"},
			{Name: ".1.3.6.1.4.1.2604.5.1.1", Type: gosnmp.OctetString, Value: []byte("Port2 down")},
		},
	}
}

func addr() *net.UDPAddr { return &net.UDPAddr{IP: net.ParseIP("192.0.2.5"), Port: 1162} }

func TestToRawTrap(t *testing.T) {
	rt := toRawTrap(v2cPacket("public"), addr())
	if rt.Version != "v2c" || rt.Community != "public" {
		t.Fatalf("unexpected header: %+v", rt)
	}
	if rt.SourceIP != "192.0.2.5" {
		t.Errorf("SourceIP = %q", rt.SourceIP)
	}
	if rt.TrapOID != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("TrapOID = %q", rt.TrapOID)
	}
	// OctetString []byte must be normalized to string.
	var found bool
	for _, vb := range rt.Varbinds {
		if vb.OID == "1.3.6.1.4.1.2604.5.1.1" {
			found = true
			if s, ok := vb.Value.(string); !ok || s != "Port2 down" {
				t.Errorf("octet varbind value = %#v", vb.Value)
			}
		}
	}
	if !found {
		t.Error("expected the sophos varbind in output")
	}
}

func TestHandlerEmitsForKnownCommunity(t *testing.T) {
	m := &countMetrics{}
	s := NewV2CSink("", func(c string) bool { return c == "public" }, nil, m)
	out := make(chan RawTrap, 1)
	s.handler(context.Background(), out)(v2cPacket("public"), addr())
	select {
	case rt := <-out:
		if rt.TrapOID != "1.3.6.1.6.3.1.1.5.3" {
			t.Errorf("TrapOID = %q", rt.TrapOID)
		}
	default:
		t.Fatal("expected a RawTrap on the channel")
	}
	if m.received != 1 {
		t.Errorf("received count = %d", m.received)
	}
}

func TestHandlerDropsUnknownCommunity(t *testing.T) {
	m := &countMetrics{}
	s := NewV2CSink("", func(c string) bool { return false }, nil, m)
	out := make(chan RawTrap, 1)
	s.handler(context.Background(), out)(v2cPacket("guessme"), addr())
	if len(out) != 0 {
		t.Error("unknown community should be dropped")
	}
	if m.auth != 1 {
		t.Errorf("auth failure count = %d, want 1", m.auth)
	}
}

func TestHandlerIgnoresNonV2c(t *testing.T) {
	m := &countMetrics{}
	s := NewV2CSink("", func(string) bool { return true }, nil, m)
	out := make(chan RawTrap, 1)
	pkt := v2cPacket("public")
	pkt.Version = gosnmp.Version3
	s.handler(context.Background(), out)(pkt, addr())
	if len(out) != 0 {
		t.Error("v3 packet must be ignored in slice 1")
	}
}

func TestHandlerRecoversPanic(t *testing.T) {
	m := &countMetrics{}
	s := NewV2CSink("", func(string) bool { panic("boom") }, nil, m)
	out := make(chan RawTrap, 1)
	// Must not panic out of the handler.
	s.handler(context.Background(), out)(v2cPacket("public"), addr())
	if m.panics != 1 {
		t.Errorf("panic count = %d, want 1", m.panics)
	}
}

package snmp

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

// freeUDPPort asks the kernel for an unused UDP port.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("cannot allocate udp port: %v", err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	c.Close()
	return port
}

// TestV2CSinkReceivesRealTrap exercises the full UDP path: bind a listener and
// send an actual gosnmp v2c trap, asserting the decoded RawTrap comes through.
func TestV2CSinkReceivesRealTrap(t *testing.T) {
	port := freeUDPPort(t)
	bind := "127.0.0.1:" + strconv.Itoa(port)

	sink := NewV2CSink(bind, func(c string) bool { return c == "public" }, nil, NopMetrics{})
	traps := make(chan RawTrap, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = sink.Start(ctx, traps) }()

	// Wait for the listener to be ready by dialing until it binds.
	waitForUDP(t, bind)

	g := &gosnmp.GoSNMP{
		Target:    "127.0.0.1",
		Port:      uint16(port),
		Community: "public",
		Version:   gosnmp.Version2c,
		Timeout:   2 * time.Second,
		Retries:   1,
	}
	if err := g.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer g.Conn.Close()

	trap := gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.6.3.1.1.5.3"},
			{Name: "1.3.6.1.4.1.2604.5.1.1", Type: gosnmp.OctetString, Value: "Port2 link down"},
		},
	}
	if _, err := g.SendTrap(trap); err != nil {
		t.Fatalf("SendTrap: %v", err)
	}

	select {
	case rt := <-traps:
		if rt.Community != "public" {
			t.Errorf("community = %q", rt.Community)
		}
		if rt.TrapOID != "1.3.6.1.6.3.1.1.5.3" {
			t.Errorf("TrapOID = %q, want linkDown OID", rt.TrapOID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trap")
	}
}

func waitForUDP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("udp", addr)
		if err == nil {
			c.Close()
			time.Sleep(50 * time.Millisecond) // let Listen finish binding
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

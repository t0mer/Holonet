package snmp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// TrapSink receives traps and emits normalized RawTraps (design §3.1). The
// interface keeps the transport swappable and v2c/v3 independent.
type TrapSink interface {
	Start(ctx context.Context, out chan<- RawTrap) error
}

// Metrics is the counter surface the sink needs. The prometheus-backed
// implementation lands in Slice 3; a no-op is used until then.
type Metrics interface {
	TrapReceived(version, source string)
	AuthFailure()
	DecodePanic()
}

// NopMetrics discards all counters.
type NopMetrics struct{}

func (NopMetrics) TrapReceived(string, string) {}
func (NopMetrics) AuthFailure()                {}
func (NopMetrics) DecodePanic()                {}

// V2CSink is an SNMPv2c trap listener built on gosnmp's TrapListener.
type V2CSink struct {
	bindAddr string
	allow    func(community string) bool
	log      *slog.Logger
	metrics  Metrics
}

// NewV2CSink builds a v2c sink. allow reports whether a community string is
// accepted; unknown communities are counted and dropped.
func NewV2CSink(bindAddr string, allow func(string) bool, log *slog.Logger, m Metrics) *V2CSink {
	if log == nil {
		log = slog.Default()
	}
	if m == nil {
		m = NopMetrics{}
	}
	return &V2CSink{bindAddr: bindAddr, allow: allow, log: log, metrics: m}
}

// Start binds the listener and blocks, feeding decoded traps to out until ctx
// is cancelled (graceful shutdown) or the listener errors.
func (s *V2CSink) Start(ctx context.Context, out chan<- RawTrap) error {
	tl := gosnmp.NewTrapListener()
	tl.Params = gosnmp.Default
	tl.OnNewTrap = s.handler(ctx, out)

	errCh := make(chan error, 1)
	go func() { errCh <- tl.Listen(s.bindAddr) }()

	// Wait until listening, or fail fast if the bind errors.
	select {
	case err := <-errCh:
		return fmt.Errorf("snmp: listen %s: %w", s.bindAddr, err)
	case <-tl.Listening():
		s.log.Info("snmp trap sink listening", "addr", s.bindAddr, "versions", "v2c")
	case <-ctx.Done():
		tl.Close()
		return ctx.Err()
	}

	select {
	case <-ctx.Done():
		tl.Close()
		<-errCh // let Listen unwind
		s.log.Info("snmp trap sink stopped")
		return nil
	case err := <-errCh:
		return fmt.Errorf("snmp: listener exited: %w", err)
	}
}

// handler returns the OnNewTrap callback. It is fully recover-guarded: gosnmp's
// trap path has a documented history of panics on malformed packets, and a
// panic here must never crash the process (design §3.1).
func (s *V2CSink) handler(ctx context.Context, out chan<- RawTrap) gosnmp.TrapHandlerFunc {
	return func(pkt *gosnmp.SnmpPacket, addr *net.UDPAddr) {
		defer func() {
			if r := recover(); r != nil {
				s.metrics.DecodePanic()
				s.log.Warn("recovered panic decoding trap", "panic", r, "source", hostOf(addr))
			}
		}()

		if pkt == nil {
			return
		}
		// Slice 1 handles v2c only; v3 is wired in Slice 3.
		if pkt.Version != gosnmp.Version2c {
			s.log.Debug("ignoring non-v2c trap", "version", pkt.Version.String(), "source", hostOf(addr))
			return
		}
		if s.allow != nil && !s.allow(pkt.Community) {
			s.metrics.AuthFailure()
			s.log.Warn("dropping trap with unknown community", "source", hostOf(addr))
			return
		}

		rt := toRawTrap(pkt, addr)
		s.metrics.TrapReceived("v2c", rt.SourceIP)
		select {
		case out <- rt:
		case <-ctx.Done():
		}
	}
}

// toRawTrap normalizes a gosnmp packet into a RawTrap.
func toRawTrap(pkt *gosnmp.SnmpPacket, addr *net.UDPAddr) RawTrap {
	vbs := make([]Varbind, 0, len(pkt.Variables))
	trapOID := ""
	for _, pdu := range pkt.Variables {
		oid := strings.TrimPrefix(pdu.Name, ".")
		vb := Varbind{OID: oid, Type: typeName(pdu.Type), Value: normalizeValue(pdu)}
		if oid == SnmpTrapOID {
			if s, ok := vb.Value.(string); ok {
				trapOID = strings.TrimPrefix(s, ".")
			}
		}
		vbs = append(vbs, vb)
	}
	return RawTrap{
		ReceivedAt: time.Now(),
		SourceIP:   hostOf(addr),
		Version:    "v2c",
		Community:  pkt.Community,
		TrapOID:    trapOID,
		Varbinds:   vbs,
	}
}

// normalizeValue converts gosnmp's raw values into template-friendly forms:
// OctetString []byte becomes a string; everything else passes through.
func normalizeValue(pdu gosnmp.SnmpPDU) any {
	if b, ok := pdu.Value.([]byte); ok {
		return string(b)
	}
	return pdu.Value
}

// typeName maps the common ASN.1 BER types to readable names for storage.
func typeName(t gosnmp.Asn1BER) string {
	switch t {
	case gosnmp.Integer:
		return "Integer"
	case gosnmp.OctetString:
		return "OctetString"
	case gosnmp.Null:
		return "Null"
	case gosnmp.ObjectIdentifier:
		return "OID"
	case gosnmp.IPAddress:
		return "IPAddress"
	case gosnmp.Counter32:
		return "Counter32"
	case gosnmp.Gauge32:
		return "Gauge32"
	case gosnmp.TimeTicks:
		return "TimeTicks"
	case gosnmp.Counter64:
		return "Counter64"
	case gosnmp.OpaqueFloat:
		return "Float"
	case gosnmp.OpaqueDouble:
		return "Double"
	default:
		return fmt.Sprintf("0x%02x", byte(t))
	}
}

func hostOf(addr *net.UDPAddr) string {
	if addr == nil {
		return ""
	}
	return addr.IP.String()
}

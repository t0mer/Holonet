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

// Metrics is the counter surface the sink needs.
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

// Config configures a Sink. One listener bound to BindAddr serves v2c and v3 on
// the same UDP socket (the version is detected per packet).
type Config struct {
	BindAddr       string
	AllowCommunity func(community string) bool // v2c acceptance predicate
	V3Users        []USMUser                   // enabled SNMPv3 USM users
	Log            *slog.Logger
	Metrics        Metrics
}

// Sink is an SNMP trap listener built on gosnmp's TrapListener, serving v2c and
// (when V3 users are configured) v3.
type Sink struct {
	bindAddr string
	allow    func(community string) bool
	v3users  []USMUser
	log      *slog.Logger
	metrics  Metrics
}

// NewSink builds a Sink from Config.
func NewSink(cfg Config) *Sink {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	m := cfg.Metrics
	if m == nil {
		m = NopMetrics{}
	}
	return &Sink{
		bindAddr: cfg.BindAddr,
		allow:    cfg.AllowCommunity,
		v3users:  cfg.V3Users,
		log:      log,
		metrics:  m,
	}
}

// NewV2CSink builds a v2c-only sink (used by Slice 1 and tests).
func NewV2CSink(bindAddr string, allow func(string) bool, log *slog.Logger, m Metrics) *Sink {
	return NewSink(Config{BindAddr: bindAddr, AllowCommunity: allow, Log: log, Metrics: m})
}

// Start binds the listener and blocks, feeding decoded traps to out until ctx
// is cancelled (graceful shutdown) or the listener errors.
func (s *Sink) Start(ctx context.Context, out chan<- RawTrap) error {
	tl := gosnmp.NewTrapListener()
	tl.Params = gosnmp.Default
	tl.OnNewTrap = s.handler(ctx, out)

	// Configure SNMPv3 users (design §3.1). Invalid users are surfaced, never
	// loaded as usable — noAuthNoPriv is rejected.
	if len(s.v3users) > 0 {
		table, errs := buildUSMTable(s.v3users)
		for _, e := range errs {
			s.log.Warn("snmpv3 user rejected", "err", e)
		}
		if table != nil {
			tl.Params.TrapSecurityParametersTable = table
			tl.Params.SecurityModel = gosnmp.UserSecurityModel
			s.log.Info("snmpv3 enabled", "users", len(s.v3users)-len(errs))
		}
	}

	errCh := make(chan error, 1)
	go func() { errCh <- tl.Listen(s.bindAddr) }()

	versions := "v2c"
	if len(s.v3users) > 0 {
		versions = "v2c+v3"
	}
	select {
	case err := <-errCh:
		return fmt.Errorf("snmp: listen %s: %w", s.bindAddr, err)
	case <-tl.Listening():
		s.log.Info("snmp trap sink listening", "addr", s.bindAddr, "versions", versions)
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
// v3 trap path has a documented history of panics on malformed/mismatched
// packets, and a panic here must never crash the process (design §3.1).
func (s *Sink) handler(ctx context.Context, out chan<- RawTrap) gosnmp.TrapHandlerFunc {
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
		switch pkt.Version {
		case gosnmp.Version2c:
			if s.allow != nil && !s.allow(pkt.Community) {
				s.metrics.AuthFailure()
				s.log.Warn("dropping v2c trap with unknown community", "source", hostOf(addr))
				return
			}
			s.emit(ctx, out, toRawTrap(pkt, addr, "v2c"))
		case gosnmp.Version3:
			// Reaching here means gosnmp already authenticated (and decrypted)
			// the packet against a configured USM user.
			s.emit(ctx, out, toRawTrap(pkt, addr, "v3"))
		default:
			s.log.Debug("ignoring unsupported trap version", "version", pkt.Version.String(), "source", hostOf(addr))
		}
	}
}

func (s *Sink) emit(ctx context.Context, out chan<- RawTrap, rt RawTrap) {
	s.metrics.TrapReceived(rt.Version, rt.SourceIP)
	select {
	case out <- rt:
	case <-ctx.Done():
	}
}

// toRawTrap normalizes a gosnmp packet into a RawTrap for the given version.
func toRawTrap(pkt *gosnmp.SnmpPacket, addr *net.UDPAddr, version string) RawTrap {
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
	rt := RawTrap{
		ReceivedAt: time.Now(),
		SourceIP:   hostOf(addr),
		Version:    version,
		TrapOID:    trapOID,
		Varbinds:   vbs,
	}
	if version == "v3" {
		if usm, ok := pkt.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok && usm != nil {
			rt.User = usm.UserName
		}
	} else {
		rt.Community = pkt.Community
	}
	return rt
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

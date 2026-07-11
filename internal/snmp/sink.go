package snmp

import (
	"context"
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"net"
	"os"
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
	Debug          bool // route gosnmp's internal decode/USM logging to stderr
}

// Sink is an SNMP trap listener built on gosnmp's TrapListener, serving v2c and
// (when V3 users are configured) v3.
type Sink struct {
	bindAddr string
	allow    func(community string) bool
	v3users  []USMUser
	log      *slog.Logger
	metrics  Metrics
	debug    bool
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
		debug:    cfg.Debug,
	}
}

// NewV2CSink builds a v2c-only sink (used by Slice 1 and tests).
func NewV2CSink(bindAddr string, allow func(string) bool, log *slog.Logger, m Metrics) *Sink {
	return NewSink(Config{BindAddr: bindAddr, AllowCommunity: allow, Log: log, Metrics: m})
}

// Start binds the listener and blocks, feeding decoded traps to out until ctx
// is cancelled (graceful shutdown) or the listener errors.
//
// gosnmp's v3 trap parser has a documented history of panics on malformed
// packets, and those panics fire inside gosnmp's own listen loop — before
// OnNewTrap, so the handler's recover() cannot catch them. To honour design
// §3.1 ("must never crash the process"), the listen loop itself is
// recover-guarded here: a panic is counted, logged, and the listener is rebuilt
// after a short backoff so trap reception self-heals.
func (s *Sink) Start(ctx context.Context, out chan<- RawTrap) error {
	first := true
	for {
		err, panicked := s.listenOnce(ctx, out, first)
		first = false
		if ctx.Err() != nil {
			s.log.Info("snmp trap sink stopped")
			return nil
		}
		if !panicked {
			return err // clean stop or a real bind/listener error
		}
		s.metrics.DecodePanic()
		s.log.Warn("recovered panic in trap listen loop; restarting listener")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// listenOnce builds a fresh listener, runs it until ctx is cancelled, the
// listener returns, or it panics, and reports whether it panicked.
func (s *Sink) listenOnce(ctx context.Context, out chan<- RawTrap, announce bool) (retErr error, panicked bool) {
	tl := s.buildListener(ctx, out)

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				s.log.Warn("panic in snmp listen loop", "panic", r)
			}
			close(done)
		}()
		retErr = tl.Listen(s.bindAddr)
	}()

	versions := "v2c"
	if len(s.v3users) > 0 {
		versions = "v2c+v3"
	}
	select {
	case <-done:
		// Exited (error or panic) before it was ready to listen.
		if retErr != nil {
			retErr = fmt.Errorf("snmp: listen %s: %w", s.bindAddr, retErr)
		}
		return retErr, panicked
	case <-tl.Listening():
		if announce {
			s.log.Info("snmp trap sink listening", "addr", s.bindAddr, "versions", versions)
		}
	case <-ctx.Done():
		tl.Close()
		<-done
		return nil, false
	}

	select {
	case <-ctx.Done():
		tl.Close()
		<-done
		return nil, false
	case <-done:
		if retErr != nil && !panicked {
			retErr = fmt.Errorf("snmp: listener exited: %w", retErr)
		}
		return retErr, panicked
	}
}

// buildListener constructs and configures a gosnmp TrapListener (v2c + optional
// v3). It is safe to call repeatedly (on restart).
func (s *Sink) buildListener(ctx context.Context, out chan<- RawTrap) *gosnmp.TrapListener {
	tl := gosnmp.NewTrapListener()
	tl.Params = gosnmp.Default
	tl.OnNewTrap = s.handler(ctx, out)

	// When debugging, route gosnmp's internal decode/USM logging to stderr. This
	// surfaces why v3 packets are dropped (auth/decrypt failures happen inside
	// gosnmp before OnNewTrap is ever called).
	usmLogger := gosnmp.NewLogger(stdlog.New(io.Discard, "", 0))
	if s.debug {
		usmLogger = gosnmp.NewLogger(stdlog.New(os.Stderr, "gosnmp ", stdlog.Ltime))
		tl.Params.Logger = usmLogger
	}

	// Configure SNMPv3 users (design §3.1). Invalid users are surfaced, never
	// loaded as usable — noAuthNoPriv is rejected.
	if len(s.v3users) > 0 {
		table, errs := buildUSMTable(s.v3users, usmLogger)
		for _, e := range errs {
			s.log.Warn("snmpv3 user rejected", "err", e)
		}
		if table != nil {
			tl.Params.TrapSecurityParametersTable = table
			// gosnmp's testAuthentication requires the listener's Version to be
			// Version3 to authenticate v3 packets; per-packet version detection
			// still routes v2c through the community path on the same socket.
			tl.Params.Version = gosnmp.Version3
			// NB: do NOT set Params.SecurityModel = UserSecurityModel — gosnmp's
			// listenUDP then runs an engine-ID block that dereferences the (nil)
			// Params.SecurityParameters and panics; the table path authenticates
			// without it.
			s.log.Info("snmpv3 enabled", "users", len(s.v3users)-len(errs))
		}
	}
	return tl
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

// Package metrics exposes HoloNet's Prometheus collectors under the holonet_
// namespace (design §3.9). The Metrics type satisfies the sink's and pipeline's
// counter interfaces so instrumentation is a single shared object.
package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "holonet"

// Metrics holds the registered collectors.
type Metrics struct {
	reg             *prometheus.Registry
	trapsReceived   *prometheus.CounterVec
	trapsSuppressed *prometheus.CounterVec
	notifications   *prometheus.CounterVec
	authFailures    prometheus.Counter
	decodePanics    prometheus.Counter
	activeChannels  prometheus.Gauge
}

// New builds and registers all collectors plus the Go runtime collectors.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		reg: reg,
		trapsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "traps_received_total",
			Help: "Traps received, by SNMP version and source.",
		}, []string{"version", "source"}),
		trapsSuppressed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "traps_suppressed_total",
			Help: "Traps suppressed or held by flood control, by strategy.",
		}, []string{"strategy"}),
		notifications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "notifications_total",
			Help: "Notification dispatch attempts, by channel and status.",
		}, []string{"channel", "status"}),
		authFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "trap_auth_failures_total",
			Help: "Traps dropped due to authentication failure.",
		}),
		decodePanics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "trap_decode_panics_total",
			Help: "Recovered panics while decoding traps.",
		}),
		activeChannels: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "active_channels",
			Help: "Number of enabled notification channels.",
		}),
	}
	reg.MustRegister(
		m.trapsReceived, m.trapsSuppressed, m.notifications,
		m.authFailures, m.decodePanics, m.activeChannels,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Handler serves the metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// ---- sink.Metrics ----

// TrapReceived counts a received trap.
func (m *Metrics) TrapReceived(version, source string) {
	m.trapsReceived.WithLabelValues(version, source).Inc()
}

// AuthFailure counts an authentication failure.
func (m *Metrics) AuthFailure() { m.authFailures.Inc() }

// DecodePanic counts a recovered decode panic.
func (m *Metrics) DecodePanic() { m.decodePanics.Inc() }

// ---- pipeline metrics ----

// TrapSuppressed counts a trap held/suppressed by flood control.
func (m *Metrics) TrapSuppressed(strategy string) {
	m.trapsSuppressed.WithLabelValues(strategy).Inc()
}

// NotificationRecorded counts a dispatch attempt outcome.
func (m *Metrics) NotificationRecorded(channelID int64, status string) {
	m.notifications.WithLabelValues(strconv.FormatInt(channelID, 10), status).Inc()
}

// SetActiveChannels sets the enabled-channel gauge.
func (m *Metrics) SetActiveChannels(n int) { m.activeChannels.Set(float64(n)) }

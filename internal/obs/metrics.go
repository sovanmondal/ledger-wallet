package obs

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics bundles the Prometheus registry and all instruments.
type Metrics struct {
	reg *prometheus.Registry

	httpRequests *prometheus.CounterVec   // rate + error rate (by status)
	httpDuration *prometheus.HistogramVec // latency, p99 derivable

	// Domain counters — the meaningful business events, not just HTTP noise.
	TransfersCreated       prometheus.Counter
	TransfersDeclinedFunds prometheus.Counter
	TransfersDeclinedOther prometheus.Counter
	IdempotentReplays      prometheus.Counter
	Conflicts              prometheus.Counter
	WalletsCreated         prometheus.Counter
	ReversalsCreated       prometheus.Counter
	ReversalsDeclined      prometheus.Counter
	ReversalsAlreadyDone   prometheus.Counter

	// TotalBalance is the global sum of all wallet balances — should be invariant.
	TotalBalance prometheus.Gauge
}

// NewMetrics builds and registers all instruments on a private registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		reg: reg,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by route, method and status.",
		}, []string{"route", "method", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds by route.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"route"}),
		TransfersCreated:       counter("transfers_created_total", "Completed transfers created."),
		TransfersDeclinedFunds: counter("transfers_declined_insufficient_funds_total", "Transfers declined for insufficient funds."),
		TransfersDeclinedOther: counter("transfers_declined_other_total", "Transfers declined for other reasons (missing wallet)."),
		IdempotentReplays:      counter("transfers_idempotent_replays_total", "Idempotent replays served the original result."),
		Conflicts:              counter("transfers_conflicts_total", "Same idempotency key with a different body (409)."),
		WalletsCreated:         counter("wallets_created_total", "New wallets created via get-or-create."),
		ReversalsCreated:       counter("reversals_created_total", "Completed reversals."),
		ReversalsDeclined:      counter("reversals_declined_total", "Reversals declined (recipient lacked funds)."),
		ReversalsAlreadyDone:   counter("reversals_already_done_total", "Attempts to reverse an already-reversed transfer (409)."),
		TotalBalance: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "wallet_total_balance_paise",
			Help: "Sum of all wallet balances in paise (conservation gauge).",
		}),
	}
	reg.MustRegister(
		m.httpRequests, m.httpDuration,
		m.TransfersCreated, m.TransfersDeclinedFunds, m.TransfersDeclinedOther,
		m.IdempotentReplays, m.Conflicts, m.WalletsCreated,
		m.ReversalsCreated, m.ReversalsDeclined, m.ReversalsAlreadyDone,
		m.TotalBalance,
	)
	return m
}

func counter(name, help string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: help})
}

// Handler exposes the registry at /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// Observe records one HTTP request's outcome.
func (m *Metrics) Observe(route, method string, status int, d time.Duration) {
	m.httpRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(route).Observe(d.Seconds())
}

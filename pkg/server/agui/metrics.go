package agui

import "github.com/prometheus/client_golang/prometheus"

var (
	aguiEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "events_total",
			Help:      "Total AG-UI SSE events emitted by type.",
		},
		[]string{"event_type"},
	)
	aguiErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "errors_total",
			Help:      "Total AG-UI errors by code.",
		},
		[]string{"code"},
	)
	aguiActiveStreams = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "active_streams",
			Help:      "Number of currently active AG-UI SSE streams.",
		},
	)
	aguiRunDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "run_duration_seconds",
			Help:      "AG-UI run duration in seconds.",
			Buckets:   []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300},
		},
	)
	aguiSlowClientDisconnects = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "slow_client_disconnects_total",
			Help:      "Streams closed due to slow client backpressure.",
		},
	)
	aguiLoadShedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "load_shed_total",
			Help:      "Runs rejected due to server at capacity.",
		},
	)
	aguiReconnectAttemptsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "saker",
			Subsystem: "agui",
			Name:      "reconnect_attempts_total",
			Help:      "Client reconnect attempts with Last-Event-ID.",
		},
	)
)

func init() {
	prometheus.MustRegister(aguiEventsTotal)
	prometheus.MustRegister(aguiErrorsTotal)
	prometheus.MustRegister(aguiActiveStreams)
	prometheus.MustRegister(aguiRunDuration)
	prometheus.MustRegister(aguiSlowClientDisconnects)
	prometheus.MustRegister(aguiLoadShedTotal)
	prometheus.MustRegister(aguiReconnectAttemptsTotal)
}

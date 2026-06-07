package observability

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	TransferTotal          *prometheus.CounterVec
	TransferAmount         prometheus.Histogram
	IdempotentRequestTotal prometheus.Counter
	ActiveAccountsTotal    prometheus.Gauge
	DBQueryDurationSeconds *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		TransferTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ledger",
				Name:      "transfer_total",
				Help:      "Total number of transfer attempts, labeled by outcome status",
			},
			[]string{"status"},
		),
		TransferAmount: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "ledger",
				Name:      "transfer_amount",
				Help:      "Distribution of transfer amounts in paise",
				Buckets: []float64{
					100, 1000, 10000, 50000, 100000, 500000, 1000000, 5000000, 10000000,
				},
			},
		),
		IdempotentRequestTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "ledger",
				Name:      "idempotent_request_total",
				Help:      "Total requests served from idempotency cache",
			},
		),
		ActiveAccountsTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "ledger",
				Name:      "active_accounts_total",
				Help:      "Current number of active accounts",
			},
		),
		DBQueryDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "ledger",
				Name:      "db_query_duration_seconds",
				Help:      "Duration of database queries in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
	}
}

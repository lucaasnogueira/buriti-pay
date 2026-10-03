package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// PaymentsProcessedTotal tracks count of processed payments by terminal status
	PaymentsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "buritipay",
			Subsystem: "payments",
			Name:      "processed_total",
			Help:      "Total count of payments processed by status",
		},
		[]string{"status"},
	)

	// APIIngestLatency tracks HTTP ingestion duration
	APIIngestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "buritipay",
			Subsystem: "api",
			Name:      "ingest_latency_seconds",
			Help:      "Latency of POST /payments ingestion in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"status_code"},
	)

	// WorkerProcessingLatency tracks asynchronous worker transaction execution duration
	WorkerProcessingLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "buritipay",
			Subsystem: "worker",
			Name:      "processing_latency_seconds",
			Help:      "Duration of asynchronous fund transfer execution in workers",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
		},
	)

	// QueueDepth tracks in-memory worker queue occupancy
	QueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "buritipay",
			Subsystem: "worker",
			Name:      "queue_depth",
			Help:      "Current number of jobs waiting in the in-memory channel",
		},
	)

	// LockContentionFailures tracks Redis lock acquisition timeouts
	LockContentionFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "buritipay",
			Subsystem: "lock",
			Name:      "failures_total",
			Help:      "Total count of distributed lock acquisition failures or timeouts",
		},
	)
)

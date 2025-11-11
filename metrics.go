package jsmachine

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "js"
)

// initMetrics initializes Prometheus metrics
func (p *Plugin) initMetrics() {
	// Counter: Total number of JavaScript executions
	p.executionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "executions_total",
			Help:      "Total number of JavaScript executions",
		},
		[]string{"status"}, // success, error, timeout
	)

	// Histogram: Execution duration in seconds
	p.executionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "execution_duration_seconds",
			Help:      "JavaScript execution duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		},
		[]string{"status"},
	)

	// Gauge: Number of VMs in the pool
	p.poolSizeGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pool_size",
			Help:      "Number of JavaScript VMs in the pool",
		},
	)

	// Gauge: Number of available (idle) VMs
	p.poolAvailable = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pool_available",
			Help:      "Number of available JavaScript VMs in the pool",
		},
	)

	// Gauge: Number of active executions
	p.activeExecutions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_executions",
			Help:      "Number of currently active JavaScript executions",
		},
	)

	// Histogram: Code size in bytes
	p.codeSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "code_size_bytes",
			Help:      "Size of JavaScript code in bytes",
			Buckets:   []float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000},
		},
	)

	// Set initial pool size gauge
	p.poolSizeGauge.Set(float64(p.cfg.PoolSize))
	p.poolAvailable.Set(float64(p.cfg.PoolSize))
}

// MetricsCollector returns prometheus collectors for the metrics plugin
func (p *Plugin) MetricsCollector() []prometheus.Collector {
	return []prometheus.Collector{
		p.executionsTotal,
		p.executionDuration,
		p.poolSizeGauge,
		p.poolAvailable,
		p.activeExecutions,
		p.codeSize,
	}
}

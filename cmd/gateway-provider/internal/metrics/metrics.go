package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ApplyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_gateway_apply_total",
		Help: "Total number of gateway apply operations",
	}, []string{"adapter", "operation", "status"})

	ApplyDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hnb_gateway_apply_duration_seconds",
		Help:    "Duration of gateway apply operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"adapter", "operation"})

	OperationsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hnb_gateway_operations_active",
		Help: "Number of active gateway operations",
	})

	MessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_gateway_messages_received_total",
		Help: "Total number of NATS messages received",
	}, []string{"step_type", "status"})

	HealthCheckDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hnb_gateway_health_check_duration_seconds",
		Help: "Duration of the last health check",
	})
)
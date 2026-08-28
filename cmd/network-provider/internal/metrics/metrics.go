package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HealthCheckFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_cni_health_check_failures_total",
		Help: "Total number of CNI health check failures",
	}, []string{"provider", "cluster_id"})

	ProviderPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hnb_cni_provider_phase",
		Help: "Current phase of CNI provider binding (0=pending,1=installing,2=ready,3=degraded,4=uninstalling)",
	}, []string{"provider", "cluster_id"})

	OperationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hnb_cni_operation_duration_seconds",
		Help:    "Duration of CNI provider operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider", "operation", "status"})

	DaemonSetDesired = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hnb_cni_daemonset_desired",
		Help: "Desired number of CNI DaemonSet pods",
	}, []string{"provider", "cluster_id"})

	DaemonSetReady = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hnb_cni_daemonset_ready",
		Help: "Ready number of CNI DaemonSet pods",
	}, []string{"provider", "cluster_id"})
)

var phaseValue = map[string]float64{
	"pending":      0,
	"installing":   1,
	"ready":        2,
	"degraded":     3,
	"uninstalling": 4,
}

func PhaseValue(phase string) float64 {
	if v, ok := phaseValue[phase]; ok {
		return v
	}
	return -1
}
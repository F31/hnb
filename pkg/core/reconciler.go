package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	reconHealthFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_cni_reconciler_health_failures_total",
		Help: "Total health check failures from reconciler",
	}, []string{"provider", "cluster_id"})

	reconPhaseTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_cni_reconciler_phase_transitions_total",
		Help: "Phase transitions triggered by reconciler",
	}, []string{"provider", "cluster_id", "from_phase", "to_phase"})
)

type ProviderReconciler struct {
	nc       *natslib.Conn
	js       jetstream.JetStream
	bindings BindingStore
	interval time.Duration
}

type BindingStore interface {
	List() ([]ProviderBinding, error)
	UpdatePhase(id string, phase ProviderPhase, healthFailures int, lastError string) error
}

func NewProviderReconciler(nc *natslib.Conn, js jetstream.JetStream, bindings BindingStore, interval time.Duration) *ProviderReconciler {
	return &ProviderReconciler{
		nc:       nc,
		js:       js,
		bindings: bindings,
		interval: interval,
	}
}

func (r *ProviderReconciler) Start(ctx context.Context) {
	log.Printf("[reconciler] starting provider lifecycle reconciler (interval=%s)", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[reconciler] stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *ProviderReconciler) reconcile(ctx context.Context) {
	bindings, err := r.bindings.List()
	if err != nil {
		log.Printf("[reconciler] list bindings: %v", err)
		return
	}

	for _, b := range bindings {
		r.reconcileOne(ctx, b)
	}
}

func (r *ProviderReconciler) reconcileOne(ctx context.Context, b ProviderBinding) {
	log.Printf("[reconciler] cluster=%s provider=%s phase=%s failures=%d",
		b.ClusterID, b.Provider, b.Phase, b.HealthFailures)

	prevPhase := b.Phase

	switch b.Phase {
	case PhaseReady:
		healthy := r.checkHealth(ctx, &b)
		if !healthy {
			reconHealthFailures.WithLabelValues(b.Provider, b.ClusterID).Inc()
			b.HealthFailures++
			if b.HealthFailures >= 3 {
				log.Printf("[reconciler] cluster=%s provider=%s: degraded after %d failures",
					b.ClusterID, b.Provider, b.HealthFailures)
				if err := r.bindings.UpdatePhase(b.ID, PhaseDegraded, b.HealthFailures, "health check failed 3x"); err != nil {
					log.Printf("[reconciler] update phase: %v", err)
				}
				reconPhaseTransitions.WithLabelValues(b.Provider, b.ClusterID, string(prevPhase), string(PhaseDegraded)).Inc()
			} else {
				if err := r.bindings.UpdatePhase(b.ID, b.Phase, b.HealthFailures, "health check failed"); err != nil {
					log.Printf("[reconciler] update health failures: %v", err)
				}
			}
		} else {
			if b.HealthFailures > 0 {
				if err := r.bindings.UpdatePhase(b.ID, PhaseReady, 0, ""); err != nil {
					log.Printf("[reconciler] reset health failures: %v", err)
				}
			}
		}

	case PhaseDegraded:
		log.Printf("[reconciler] cluster=%s provider=%s: attempting repair", b.ClusterID, b.Provider)
		r.requestRepair(ctx, &b)
		reconPhaseTransitions.WithLabelValues(b.Provider, b.ClusterID, string(prevPhase), string(PhaseInstalling)).Inc()

	case PhasePending:
		log.Printf("[reconciler] cluster=%s provider=%s: starting install", b.ClusterID, b.Provider)
		r.requestInstall(ctx, &b)
		reconPhaseTransitions.WithLabelValues(b.Provider, b.ClusterID, string(prevPhase), string(PhaseInstalling)).Inc()

	case PhaseUninstalling:
		log.Printf("[reconciler] cluster=%s provider=%s: starting uninstall", b.ClusterID, b.Provider)
		r.requestUninstall(ctx, &b)
		reconPhaseTransitions.WithLabelValues(b.Provider, b.ClusterID, string(prevPhase), string(PhaseUninstalling)).Inc()
	}
}

func (r *ProviderReconciler) checkHealth(ctx context.Context, b *ProviderBinding) bool {
	payload, _ := json.Marshal(map[string]string{
		"operation_id": fmt.Sprintf("reconciler-health-%s-%s", b.ClusterID, b.Provider),
		"action":       "health",
		"provider":     b.Provider,
		"target_id":    b.ClusterID,
	})

	msg, err := r.nc.Request(fmt.Sprintf("hnb.network.%s.health", b.Provider), payload, 30*time.Second)
	if err != nil {
		log.Printf("[reconciler] health check failed: %v", err)
		return false
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		log.Printf("[reconciler] bad health response: %v", err)
		return false
	}

	return result.Status == "succeeded"
}

func (r *ProviderReconciler) requestInstall(ctx context.Context, b *ProviderBinding) {
	payload, _ := json.Marshal(map[string]string{
		"operation_id": fmt.Sprintf("reconciler-install-%s-%s", b.ClusterID, b.Provider),
		"action":       "install",
		"provider":     b.Provider,
		"target_id":    b.ClusterID,
		"version":      b.Version,
	})

	if _, err := r.js.Publish(ctx, "hnb.network.install", payload); err != nil {
		log.Printf("[reconciler] publish install: %v", err)
		return
	}

	if err := r.bindings.UpdatePhase(b.ID, PhaseInstalling, 0, ""); err != nil {
		log.Printf("[reconciler] update phase to installing: %v", err)
	}
}

func (r *ProviderReconciler) requestRepair(ctx context.Context, b *ProviderBinding) {
	payload, _ := json.Marshal(map[string]string{
		"operation_id": fmt.Sprintf("reconciler-repair-%s-%s", b.ClusterID, b.Provider),
		"action":       "upgrade",
		"provider":     b.Provider,
		"target_id":    b.ClusterID,
		"version":      b.Version,
	})

	if _, err := r.js.Publish(ctx, "hnb.network.upgrade", payload); err != nil {
		log.Printf("[reconciler] publish repair: %v", err)
	}

	if err := r.bindings.UpdatePhase(b.ID, PhaseInstalling, 0, ""); err != nil {
		log.Printf("[reconciler] update phase to installing: %v", err)
	}
}

func (r *ProviderReconciler) requestUninstall(ctx context.Context, b *ProviderBinding) {
	payload, _ := json.Marshal(map[string]string{
		"operation_id": fmt.Sprintf("reconciler-uninstall-%s-%s", b.ClusterID, b.Provider),
		"action":       "uninstall",
		"provider":     b.Provider,
		"target_id":    b.ClusterID,
	})

	if _, err := r.js.Publish(ctx, "hnb.network.uninstall", payload); err != nil {
		log.Printf("[reconciler] publish uninstall: %v", err)
	}
}
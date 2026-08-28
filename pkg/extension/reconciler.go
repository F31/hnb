package extension

import (
	"context"
	"log"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	reconExtHealthFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_reconciler_health_failures_total",
		Help: "Health check failures from extension reconciler",
	}, []string{"extension"})

	reconExtPhaseTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_reconciler_phase_transitions_total",
		Help: "Phase transitions triggered by extension reconciler",
	}, []string{"extension", "from_phase", "to_phase"})
)

type ExtensionReconciler struct {
	manager  *ExtensionManager
	store    ExtensionStore
	interval time.Duration
}

func NewExtensionReconciler(manager *ExtensionManager, store ExtensionStore, interval time.Duration) *ExtensionReconciler {
	return &ExtensionReconciler{
		manager:  manager,
		store:    store,
		interval: interval,
	}
}

func (r *ExtensionReconciler) Start(ctx context.Context) {
	log.Printf("[ext-reconciler] starting extension lifecycle reconciler (interval=%s)", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[ext-reconciler] stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *ExtensionReconciler) reconcile(ctx context.Context) {
	extensions, err := r.store.List()
	if err != nil {
		log.Printf("[ext-reconciler] list extensions: %v", err)
		return
	}

	for _, ext := range extensions {
		r.reconcileOne(ctx, &ext)
	}
}

func (r *ExtensionReconciler) reconcileOne(ctx context.Context, ext *core.Extension) {
	prevPhase := ext.Phase

	switch ext.Phase {
	case core.ExtPending:
		log.Printf("[ext-reconciler] %s: starting install", ext.Name)
		if err := r.manager.Install(ctx, ext); err != nil {
			log.Printf("[ext-reconciler] %s install failed: %v", ext.Name, err)
		}
		reconExtPhaseTransitions.WithLabelValues(ext.Name, string(prevPhase), string(core.ExtInstalling)).Inc()

	case core.ExtReady:
		healthy, err := r.manager.HealthCheck(ctx, ext)
		if err != nil {
			log.Printf("[ext-reconciler] %s health check error: %v", ext.Name, err)
			ext.HealthFailures++
		}
		if !healthy {
			reconExtHealthFailures.WithLabelValues(ext.Name).Inc()
			ext.HealthFailures++
			if ext.HealthFailures >= 3 {
				log.Printf("[ext-reconciler] %s: degraded after %d failures", ext.Name, ext.HealthFailures)
				if err := r.store.UpdatePhase(ext.ID, core.ExtDegraded, ext.HealthFailures, "health check failed 3x"); err != nil {
					log.Printf("[ext-reconciler] update phase: %v", err)
				}
				reconExtPhaseTransitions.WithLabelValues(ext.Name, string(prevPhase), string(core.ExtDegraded)).Inc()
			} else {
				if err := r.store.UpdatePhase(ext.ID, ext.Phase, ext.HealthFailures, "health check failed"); err != nil {
					log.Printf("[ext-reconciler] update health failures: %v", err)
				}
			}
		} else {
			if ext.HealthFailures > 0 {
				if err := r.store.UpdatePhase(ext.ID, core.ExtReady, 0, ""); err != nil {
					log.Printf("[ext-reconciler] reset health failures: %v", err)
				}
			}
		}

	case core.ExtDegraded:
		log.Printf("[ext-reconciler] %s: attempting repair via upgrade", ext.Name)
		if err := r.manager.Upgrade(ctx, ext, ext.Version); err != nil {
			log.Printf("[ext-reconciler] %s repair failed: %v", ext.Name, err)
		}
		reconExtPhaseTransitions.WithLabelValues(ext.Name, string(prevPhase), string(core.ExtInstalling)).Inc()

	case core.ExtUninstalling:
		log.Printf("[ext-reconciler] %s: completing uninstall", ext.Name)
		if err := r.manager.Uninstall(ctx, ext); err != nil {
			log.Printf("[ext-reconciler] %s uninstall failed: %v", ext.Name, err)
		}
	}
}
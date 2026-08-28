package reconciler

import (
	"context"
	"log"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/F31/hnb/cmd/gslb-controller/internal/healthsource"
	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
)

// ReadModelProjector 更新只读投影（GSLB-007）。
type ReadModelProjector interface {
	ProjectReadModel(ctx context.Context, domain string, healthyTargets []string) error
}

// FailoverReporter 上报故障转移决策（GSLB-005：只写请求，绝不修改 DNS）。
type FailoverReporter interface {
	// EnsureFailoverForDomain 幂等创建自动故障转移请求；返回是否新建。
	EnsureFailoverForDomain(ctx context.Context, domain string) (bool, error)
}

// Reconciler 负责健康探测与状态投影。GSLB-005 起不再拥有 DNS 写能力：
// reconciler 中不存在任何 dns.Manager 调用；DNS 数据面只由 executor
// （经 NATS 执行命令驱动）访问。
type Reconciler struct {
	clusterStore *store.ClusterStore
	healthMgr    *healthsource.HealthManager
	decider      FailoverReporter
	projector    ReadModelProjector
	domain       string
	interval     time.Duration
}

func New(
	clusterStore *store.ClusterStore,
	healthMgr *healthsource.HealthManager,
	dynClient dynamic.Interface,
	kubeClient kubernetes.Interface,
	decider FailoverReporter,
	projector ReadModelProjector,
	domain string,
	interval time.Duration,
) *Reconciler {
	return &Reconciler{
		clusterStore: clusterStore,
		healthMgr:    healthMgr,
		decider:      decider,
		projector:    projector,
		domain:       domain,
		interval:     interval,
	}
}

func (r *Reconciler) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.reconcile(ctx)
	for {
		select {
		case <-ticker.C:
			r.reconcile(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	log.Printf("[reconciler] starting health projection cycle")

	clusters, err := r.clusterStore.ListActive(ctx)
	if err != nil {
		log.Printf("[reconciler] failed to list clusters: %v", err)
		return
	}

	targets := make([]healthsource.ClusterTarget, 0, len(clusters))
	for _, c := range clusters {
		targets = append(targets, healthsource.ClusterTarget{
			Name:     c.Name,
			Endpoint: c.APIEndpoint,
			Labels:   c.Labels,
		})
	}

	mergedStatus := r.healthMgr.ProbeAll(ctx, targets)

	for _, c := range clusters {
		ms, ok := mergedStatus[c.Name]
		if !ok {
			log.Printf("[reconciler] cluster %s: no health status", c.Name)
			continue
		}
		dbStatus := mapStatusToDB(ms.Status)
		if dbStatus != c.Status {
			if err := r.clusterStore.UpdateStatus(ctx, c.ID, dbStatus); err != nil {
				log.Printf("[reconciler] failed to update cluster %s status: %v", c.Name, err)
			}
		}
	}

	healthyTargets := r.healthMgr.HealthyTargets(targets)

	// GSLB-007：投影只读状态（健康池 + 当前 DNS 目标）
	if r.projector != nil {
		healthyNames := make([]string, 0, len(healthyTargets))
		for _, target := range healthyTargets {
			healthyNames = append(healthyNames, target.Name)
		}
		if err := r.projector.ProjectReadModel(ctx, r.domain, healthyNames); err != nil {
			log.Printf("[reconciler] read model projection failed: %v", err)
		}
	}

	// GSLB-005：全部成员失健康时不再直改 DNS，而是上报受控的故障转移请求
	// （PendingApproval，经审批后由 executor 执行）。reconciler 无 DNS 写能力。
	if len(targets) > 0 && len(healthyTargets) == 0 {
		if r.decider != nil {
			created, err := r.decider.EnsureFailoverForDomain(ctx, r.domain)
			if err != nil {
				log.Printf("[reconciler] failover decision failed: %v", err)
			} else if created {
				log.Printf("[reconciler] auto-failover request created for domain %s (approval required)", r.domain)
			} else {
				log.Printf("[reconciler] active failover request already exists for domain %s", r.domain)
			}
		}
	} else {
		log.Printf("[reconciler] healthy=%d/%d targets (no failover decision)", len(healthyTargets), len(targets))
	}

	log.Printf("[reconciler] health projection complete: %d clusters, %d healthy (sources: %d)",
		len(clusters), len(healthyTargets), len(r.healthMgr.Sources()))
}

func mapStatusToDB(status string) string {
	switch status {
	case "healthy":
		return "healthy"
	case "degraded":
		return "degraded"
	case "unreachable":
		return "unreachable"
	default:
		return "unknown"
	}
}

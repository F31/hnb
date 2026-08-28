package handler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type clusterProjectionMode string

const (
	clusterProjectionDisabled clusterProjectionMode = "disabled"
	clusterProjectionShadow   clusterProjectionMode = "shadow"
	clusterProjectionCutover  clusterProjectionMode = "cutover"
)

var (
	clusterProjectionComparisons = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_cluster_read_shadow_comparisons_total",
		Help: "Cluster read projection comparisons by mode and result.",
	}, []string{"mode", "result"})
	clusterProjectionMismatches = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_cluster_read_shadow_mismatches_total",
		Help: "Cluster read projection mismatches by bounded dimension.",
	}, []string{"dimension"})
	clusterProjectionGateReady = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hnb_cluster_read_cutover_gate_ready",
		Help: "Whether the most recent cluster projection comparison permits cutover.",
	})
	clusterProjectionGateBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_cluster_read_cutover_blocked_total",
		Help: "Cluster read cutover requests blocked by reason.",
	}, []string{"reason"})
)

type clusterProjectionDiff struct {
	LegacyCount    int
	CanonicalCount int
	StatusMismatch int
	TenantMismatch int
}

func (d clusterProjectionDiff) mismatched() bool {
	return d.LegacyCount != d.CanonicalCount || d.StatusMismatch > 0 || d.TenantMismatch > 0
}

type clusterProjectionComparator interface {
	Compare(context.Context, string) (clusterProjectionDiff, error)
}

type clusterProjectionRecorder interface {
	Record(clusterProjectionMode, clusterProjectionDiff, error)
	Blocked(string)
}

type sqlClusterProjectionComparator struct {
	db *sql.DB
}

func (c sqlClusterProjectionComparator) Compare(ctx context.Context, tenantID string) (clusterProjectionDiff, error) {
	const query = `
		SELECT
			count(*) FILTER (WHERE rt.is_active AND rt.target_type IN ('kubernetes','edge_runtime')),
			count(*) FILTER (WHERE rt.is_active AND rt.target_type IN ('kubernetes','edge_runtime')
				AND rt.lifecycle_state IS NOT NULL AND rt.health_state IS NOT NULL
				AND rt.connectivity_state IS NOT NULL AND rt.freshness_state IS NOT NULL),
			count(*) FILTER (WHERE rt.is_active AND rt.target_type IN ('kubernetes','edge_runtime') AND
				(CASE
					WHEN rt.observed_at IS NULL OR rt.observed_at + rt.stale_threshold_seconds * interval '1 second' < now() THEN 'STALE'
					WHEN rt.status = 'decommissioned' THEN 'TERMINATED'
					WHEN rt.status IN ('degraded','offline') THEN 'DEGRADED'
					WHEN rt.status = 'online' THEN 'RUNNING'
					ELSE 'UNKNOWN'
				 END) IS DISTINCT FROM
				(CASE
					WHEN rt.freshness_state = 'STALE' THEN 'STALE'
					WHEN rt.lifecycle_state = 'TERMINATED' THEN 'TERMINATED'
					WHEN rt.lifecycle_state = 'DELETING' THEN 'DELETING'
					WHEN rt.lifecycle_state = 'FAILED' THEN 'FAILED'
					WHEN rt.lifecycle_state = 'UPGRADING' THEN 'UPGRADING'
					WHEN rt.lifecycle_state = 'PROVISIONING' THEN 'PROVISIONING'
					WHEN rt.lifecycle_state = 'REGISTERING' THEN 'REGISTERING'
					WHEN rt.health_state IN ('DEGRADED','UNHEALTHY') OR rt.connectivity_state = 'DISCONNECTED' THEN 'DEGRADED'
					WHEN rt.lifecycle_state = 'ACTIVE' AND rt.health_state = 'HEALTHY' THEN 'RUNNING'
					ELSE 'UNKNOWN'
				 END)),
			(SELECT count(*) FROM runtime_target_nodes n
			 JOIN runtime_targets parent ON parent.id = n.target_id
			 WHERE parent.tenant_id = $1 AND n.tenant_id IS DISTINCT FROM parent.tenant_id) +
			(SELECT count(*) FROM capability_snapshots cs
			 JOIN runtime_targets parent ON parent.id = cs.target_id
			 WHERE parent.tenant_id = $1 AND cs.tenant_id IS DISTINCT FROM parent.tenant_id)
		FROM runtime_targets rt
		WHERE rt.tenant_id = $1`
	var diff clusterProjectionDiff
	err := c.db.QueryRowContext(ctx, query, tenantID).Scan(
		&diff.LegacyCount, &diff.CanonicalCount, &diff.StatusMismatch, &diff.TenantMismatch,
	)
	return diff, err
}

type prometheusClusterProjectionRecorder struct{}

func (prometheusClusterProjectionRecorder) Record(mode clusterProjectionMode, diff clusterProjectionDiff, err error) {
	result := "match"
	if err != nil {
		result = "error"
	} else if diff.mismatched() {
		result = "mismatch"
	}
	clusterProjectionComparisons.WithLabelValues(string(mode), result).Inc()
	if diff.LegacyCount != diff.CanonicalCount {
		clusterProjectionMismatches.WithLabelValues("count").Inc()
	}
	if diff.StatusMismatch > 0 {
		clusterProjectionMismatches.WithLabelValues("status").Add(float64(diff.StatusMismatch))
	}
	if diff.TenantMismatch > 0 {
		clusterProjectionMismatches.WithLabelValues("tenant").Add(float64(diff.TenantMismatch))
	}
	if err == nil && !diff.mismatched() {
		clusterProjectionGateReady.Set(1)
	} else {
		clusterProjectionGateReady.Set(0)
	}
}

func (prometheusClusterProjectionRecorder) Blocked(reason string) {
	clusterProjectionGateBlocked.WithLabelValues(reason).Inc()
}

func parseClusterProjectionMode(value string) clusterProjectionMode {
	switch clusterProjectionMode(strings.ToLower(strings.TrimSpace(value))) {
	case clusterProjectionDisabled:
		return clusterProjectionDisabled
	case clusterProjectionCutover:
		return clusterProjectionCutover
	default:
		return clusterProjectionShadow
	}
}

func (h *ResourceClusterHandler) allowClusterProjectionRead(ctx context.Context, tenantID string) bool {
	if h.projectionMode == clusterProjectionDisabled || h.projectionComparator == nil {
		return true
	}
	diff, err := h.projectionComparator.Compare(ctx, tenantID)
	h.projectionRecorder.Record(h.projectionMode, diff, err)
	if h.projectionMode != clusterProjectionCutover {
		return true
	}
	if err != nil {
		h.projectionRecorder.Blocked("comparison_error")
		return false
	}
	if diff.mismatched() {
		h.projectionRecorder.Blocked("projection_mismatch")
		return false
	}
	return true
}

func clusterListWhere(tenantID, targetType, keyword, lifecycle, health, connectivity, freshness string, combinedStatuses ...string) (string, []any) {
	where := "WHERE rt.is_active = true AND rt.target_type IN ('kubernetes','edge_runtime') AND (rt.tenant_id = $1 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=rt.id AND tca.tenant_id=$1 AND tca.status='active'))"
	args := []any{tenantID}
	add := func(predicate string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(predicate, len(args))
	}
	if targetType != "" {
		add(" AND rt.target_type = $%d", targetType)
	}
	if keyword != "" {
		add(" AND (rt.name ILIKE $%d OR COALESCE(rt.display_name,'') ILIKE $%d)", "%"+keyword+"%")
	}
	if lifecycle != "" {
		add(" AND rt.lifecycle_state = $%d", lifecycle)
	}
	if health != "" {
		add(" AND rt.health_state = $%d", health)
	}
	if connectivity != "" {
		add(" AND rt.connectivity_state = $%d", connectivity)
	}
	if freshness != "" {
		add(" AND rt.freshness_state = $%d", freshness)
	}
	// Match mapCombinedStatus precedence so count, rows and UI badges agree.
	if len(combinedStatuses) > 0 && combinedStatuses[0] != "" {
		switch combinedStatuses[0] {
		case "STALE":
			where += " AND rt.freshness_state = 'STALE'"
		case "TERMINATED", "DELETING", "FAILED", "UPGRADING", "PROVISIONING", "REGISTERING":
			add(" AND rt.freshness_state <> 'STALE' AND rt.lifecycle_state = $%d", combinedStatuses[0])
		case "DEGRADED":
			where += " AND rt.freshness_state <> 'STALE' AND rt.lifecycle_state NOT IN ('TERMINATED','DELETING','FAILED','UPGRADING','PROVISIONING','REGISTERING') AND (rt.health_state IN ('DEGRADED','UNHEALTHY') OR rt.connectivity_state = 'DISCONNECTED')"
		case "RUNNING":
			where += " AND rt.freshness_state <> 'STALE' AND rt.lifecycle_state = 'ACTIVE' AND rt.health_state = 'HEALTHY' AND rt.connectivity_state <> 'DISCONNECTED'"
		case "UNKNOWN":
			where += " AND rt.freshness_state <> 'STALE' AND rt.lifecycle_state NOT IN ('TERMINATED','DELETING','FAILED','UPGRADING','PROVISIONING','REGISTERING','ACTIVE') AND rt.health_state NOT IN ('DEGRADED','UNHEALTHY') AND rt.connectivity_state <> 'DISCONNECTED'"
		}
	}
	return where, args
}

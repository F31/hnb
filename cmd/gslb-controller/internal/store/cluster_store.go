package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Cluster struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	TenantID      string            `json:"tenant_id"`
	ClusterType   string            `json:"cluster_type"`
	APIEndpoint   string            `json:"api_endpoint"`
	KubeconfigRef string            `json:"kubeconfig_ref,omitempty"`
	Region        string            `json:"region,omitempty"`
	Zone          string            `json:"zone,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Status        string            `json:"status"`
	LastHeartbeat *time.Time        `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type ClusterStore struct {
	db *sql.DB
}

func NewClusterStore(db *sql.DB) *ClusterStore {
	return &ClusterStore{db: db}
}

func (s *ClusterStore) ListActive(ctx context.Context) ([]*Cluster, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, name, tenant_id, target_type, COALESCE(connection_endpoint, ''), labels,
		       status, lifecycle_state, health_state, connectivity_state, observed_at, created_at, updated_at
		FROM runtime_targets
		WHERE target_type = 'kubernetes'
		  AND is_active
		  AND COALESCE(status, 'unknown') <> 'decommissioned'
		  AND COALESCE(lifecycle_state, 'UNKNOWN') <> 'TERMINATED'
		  AND COALESCE(connection_endpoint, '') <> ''
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query active runtime targets: %w", err)
	}
	return scanRuntimeTargetClusters(rows)
}

func (s *ClusterStore) ListAll(ctx context.Context) ([]*Cluster, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, name, tenant_id, target_type, COALESCE(connection_endpoint, ''), labels,
		       status, lifecycle_state, health_state, connectivity_state, observed_at, created_at, updated_at
		FROM runtime_targets
		WHERE target_type = 'kubernetes'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query all runtime targets: %w", err)
	}
	return scanRuntimeTargetClusters(rows)
}

func scanRuntimeTargetClusters(rows *sql.Rows) ([]*Cluster, error) {
	defer rows.Close()

	var clusters []*Cluster
	for rows.Next() {
		c := &Cluster{}
		var labelsJSON []byte
		var status, lifecycleState, healthState, connectivityState sql.NullString
		var observedAt sql.NullTime

		if err := rows.Scan(&c.ID, &c.Name, &c.TenantID, &c.ClusterType, &c.APIEndpoint, &labelsJSON, &status, &lifecycleState, &healthState, &connectivityState, &observedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan runtime target cluster: %w", err)
		}
		if observedAt.Valid {
			c.LastHeartbeat = &observedAt.Time
		}
		if len(labelsJSON) > 0 {
			_ = json.Unmarshal(labelsJSON, &c.Labels)
		}
		c.Status = runtimeTargetGSLBStatus(status.String, lifecycleState.String, healthState.String, connectivityState.String)
		clusters = append(clusters, c)
	}
	if clusters == nil {
		clusters = []*Cluster{}
	}
	return clusters, rows.Err()
}

func (s *ClusterStore) UpdateStatus(ctx context.Context, id, status string) error {
	runtimeStatus, healthState, connectivityState := gslbStatusRuntimeTargetState(status)
	_, err := s.db.ExecContext(ctx, `
		UPDATE runtime_targets
		SET status = $1,
		    health_state = $2,
		    connectivity_state = $3,
		    observed_at = now(),
		    last_known_state_at = now(),
		    updated_at = now(),
		    projection_version = projection_version + 1
		WHERE id = $4`, runtimeStatus, healthState, connectivityState, id)
	return err
}

func (s *ClusterStore) RecordHeartbeat(ctx context.Context, clusterID, status, version string, nodeCount int, capacity map[string]any) error {
	snapshot := map[string]any{
		"capacity":   capacity,
		"nodeCount":  nodeCount,
		"source":     "gslb-controller",
		"gslbStatus": status,
	}
	capacityJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal capacity: %w", err)
	}

	runtimeStatus, healthState, connectivityState := gslbStatusRuntimeTargetState(status)
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO capability_snapshots (target_id, tenant_id, kube_version, snapshot_json, observed_at)
		SELECT id, tenant_id, $2, $3, $4
		FROM runtime_targets
		WHERE id = $1`,
		clusterID, nullable(version), capacityJSON, now,
	)
	if err != nil {
		return fmt.Errorf("insert capability snapshot heartbeat: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE runtime_targets
		SET status = $1,
		    health_state = $2,
		    connectivity_state = $3,
		    observed_at = $4,
		    last_known_state_at = $4,
		    updated_at = $4,
		    projection_version = projection_version + 1
		WHERE id = $5`, runtimeStatus, healthState, connectivityState, now, clusterID)
	return err
}

func runtimeTargetGSLBStatus(status, lifecycleState, healthState, connectivityState string) string {
	if status == "decommissioned" || lifecycleState == "TERMINATED" || connectivityState == "DISCONNECTED" || status == "offline" {
		return "unreachable"
	}
	if status == "degraded" || healthState == "DEGRADED" || healthState == "UNHEALTHY" {
		return "degraded"
	}
	if status == "online" || healthState == "HEALTHY" || connectivityState == "CONNECTED" {
		return "healthy"
	}
	return "unknown"
}

func gslbStatusRuntimeTargetState(status string) (runtimeStatus, healthState, connectivityState string) {
	switch status {
	case "healthy":
		return "online", "HEALTHY", "CONNECTED"
	case "degraded":
		return "degraded", "DEGRADED", "CONNECTED"
	case "unreachable":
		return "offline", "UNHEALTHY", "DISCONNECTED"
	default:
		return "unknown", "UNKNOWN", "UNKNOWN"
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

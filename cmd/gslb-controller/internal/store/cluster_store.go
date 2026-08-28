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
		SELECT id, name, tenant_id, cluster_type, api_endpoint, region, zone, labels, status, last_heartbeat, created_at, updated_at
		FROM clusters WHERE status IN ('active', 'healthy')
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query active clusters: %w", err)
	}
	defer rows.Close()

	var clusters []*Cluster
	for rows.Next() {
		c := &Cluster{}
		var labelsJSON []byte
		var region, zone, lastHeartbeat sql.NullString

		if err := rows.Scan(&c.ID, &c.Name, &c.TenantID, &c.ClusterType, &c.APIEndpoint, &region, &zone, &labelsJSON, &c.Status, &lastHeartbeat, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		c.Region = region.String
		c.Zone = zone.String
		if lastHeartbeat.Valid {
			t, _ := time.Parse(time.RFC3339, lastHeartbeat.String)
			c.LastHeartbeat = &t
		}
		if len(labelsJSON) > 0 {
			_ = json.Unmarshal(labelsJSON, &c.Labels)
		}
		clusters = append(clusters, c)
	}
	if clusters == nil {
		clusters = []*Cluster{}
	}
	return clusters, rows.Err()
}

func (s *ClusterStore) ListAll(ctx context.Context) ([]*Cluster, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, tenant_id, cluster_type, api_endpoint, region, zone, labels, status, last_heartbeat, created_at, updated_at
		FROM clusters ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query all clusters: %w", err)
	}
	defer rows.Close()

	var clusters []*Cluster
	for rows.Next() {
		c := &Cluster{}
		var labelsJSON []byte
		var region, zone, lastHeartbeat sql.NullString

		if err := rows.Scan(&c.ID, &c.Name, &c.TenantID, &c.ClusterType, &c.APIEndpoint, &region, &zone, &labelsJSON, &c.Status, &lastHeartbeat, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		c.Region = region.String
		c.Zone = zone.String
		if lastHeartbeat.Valid {
			t, _ := time.Parse(time.RFC3339, lastHeartbeat.String)
			c.LastHeartbeat = &t
		}
		if len(labelsJSON) > 0 {
			_ = json.Unmarshal(labelsJSON, &c.Labels)
		}
		clusters = append(clusters, c)
	}
	if clusters == nil {
		clusters = []*Cluster{}
	}
	return clusters, rows.Err()
}

func (s *ClusterStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clusters SET status = $1, updated_at = now() WHERE id = $2`, status, id)
	return err
}

func (s *ClusterStore) RecordHeartbeat(ctx context.Context, clusterID, status, version string, nodeCount int, capacity map[string]any) error {
	capacityJSON, err := json.Marshal(capacity)
	if err != nil {
		return fmt.Errorf("marshal capacity: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO cluster_heartbeats (cluster_id, status, version, node_count, capacity, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		clusterID, status, nullable(version), nodeCount, capacityJSON, now,
	)
	if err != nil {
		return fmt.Errorf("insert heartbeat: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `UPDATE clusters SET status = $1, last_heartbeat = $2, updated_at = $2 WHERE id = $3`,
		status, now, clusterID)
	return err
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
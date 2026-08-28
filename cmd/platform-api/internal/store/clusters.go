package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	Version       int64             `json:"version"`
	Scope         string            `json:"scope"`
}

type ClusterHeartbeat struct {
	ID         string         `json:"id"`
	ClusterID  string         `json:"cluster_id"`
	Status     string         `json:"status"`
	Version    string         `json:"version,omitempty"`
	NodeCount  int            `json:"node_count,omitempty"`
	Capacity   map[string]any `json:"capacity,omitempty"`
	ObservedAt time.Time      `json:"observed_at"`
}

type ClusterStore struct {
	db *sql.DB
}

func NewClusterStore(db *sql.DB) *ClusterStore {
	return &ClusterStore{db: db}
}

func (s *ClusterStore) Register(ctx context.Context, c *Cluster) error {
	c.ID = uuid.NewString()
	c.Status = "pending"
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	c.Version = 1
	if c.Scope == "" {
		c.Scope = "tenant"
	}

	labelsJSON, err := json.Marshal(c.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO clusters (id, name, tenant_id, cluster_type, api_endpoint, kubeconfig_ref, region, zone, labels, status, created_at, updated_at, version, scope)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		c.ID, c.Name, c.TenantID, c.ClusterType, c.APIEndpoint, nullable(c.KubeconfigRef), nullable(c.Region), nullable(c.Zone), labelsJSON, c.Status, c.CreatedAt, c.UpdatedAt, c.Version, c.Scope,
	)
	return err
}

func (s *ClusterStore) Get(ctx context.Context, id, tenantID string) (*Cluster, error) {
	c := &Cluster{}
	var labelsJSON []byte
	var kubeconfigRef, region, zone sql.NullString
	var lastHeartbeat sql.NullTime
	var version sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, tenant_id, cluster_type, api_endpoint, kubeconfig_ref, region, zone, labels, status, last_heartbeat, created_at, updated_at, version, scope
		FROM clusters WHERE id = $1 AND tenant_id = $2`, id, tenantID,
	).Scan(&c.ID, &c.Name, &c.TenantID, &c.ClusterType, &c.APIEndpoint, &kubeconfigRef, &region, &zone, &labelsJSON, &c.Status, &lastHeartbeat, &c.CreatedAt, &c.UpdatedAt, &version, &c.Scope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrClusterNotFound
		}
		return nil, err
	}

	c.KubeconfigRef = kubeconfigRef.String
	c.Region = region.String
	c.Zone = zone.String
	if lastHeartbeat.Valid {
		c.LastHeartbeat = &lastHeartbeat.Time
	}
	if version.Valid {
		c.Version = version.Int64
	}
	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &c.Labels); err != nil {
			return nil, fmt.Errorf("decode cluster labels: %w", err)
		}
	}
	return c, nil
}

func (s *ClusterStore) List(ctx context.Context, tenantID string) ([]*Cluster, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, tenant_id, cluster_type, api_endpoint, region, zone, labels, status, last_heartbeat, created_at, updated_at, version, scope
		FROM clusters WHERE tenant_id = $1 AND scope = 'tenant' ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clusters []*Cluster
	for rows.Next() {
		c := &Cluster{}
		var labelsJSON []byte
		var region, zone sql.NullString
		var lastHeartbeat sql.NullTime
		var version sql.NullInt64

		if err := rows.Scan(&c.ID, &c.Name, &c.TenantID, &c.ClusterType, &c.APIEndpoint, &region, &zone, &labelsJSON, &c.Status, &lastHeartbeat, &c.CreatedAt, &c.UpdatedAt, &version, &c.Scope); err != nil {
			return nil, err
		}
		c.Region = region.String
		c.Zone = zone.String
		if lastHeartbeat.Valid {
			c.LastHeartbeat = &lastHeartbeat.Time
		}
		if version.Valid {
			c.Version = version.Int64
		}
		if len(labelsJSON) > 0 {
			if err := json.Unmarshal(labelsJSON, &c.Labels); err != nil {
				return nil, fmt.Errorf("decode cluster labels: %w", err)
			}
		}
		clusters = append(clusters, c)
	}
	if clusters == nil {
		clusters = []*Cluster{}
	}
	return clusters, rows.Err()
}

func (s *ClusterStore) Unregister(ctx context.Context, id, tenantID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrClusterNotFound
	}
	return nil
}

func (s *ClusterStore) RecordHeartbeat(ctx context.Context, h *ClusterHeartbeat) error {
	h.ID = uuid.NewString()
	h.ObservedAt = time.Now().UTC()

	capacityJSON, err := json.Marshal(h.Capacity)
	if err != nil {
		return fmt.Errorf("marshal capacity: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO cluster_heartbeats (id, cluster_id, status, version, node_count, capacity, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		h.ID, h.ClusterID, h.Status, nullable(h.Version), h.NodeCount, capacityJSON, h.ObservedAt,
	)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `UPDATE clusters SET status = $1, last_heartbeat = $2, updated_at = $2 WHERE id = $3`,
		h.Status, h.ObservedAt, h.ClusterID)
	return err
}

func (s *ClusterStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clusters SET status = $1, updated_at = now() WHERE id = $2`, status, id)
	return err
}

// Store interface implementations — pure data access, no auth logic
func (s *ClusterStore) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	c := &Cluster{
		Name:          req.Name,
		TenantID:      req.TenantID,
		ClusterType:   req.ClusterType,
		APIEndpoint:   req.APIEndpoint,
		KubeconfigRef: req.KubeconfigRef,
		Region:        req.Region,
		Zone:          req.Zone,
		Labels:        req.Labels,
	}
	if err := s.Register(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *ClusterStore) GetCluster(ctx context.Context, id, tenantID string) (*Cluster, error) {
	c, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if c.TenantID != tenantID {
		return nil, ErrTenantMismatch
	}
	return c, nil
}

func (s *ClusterStore) ListClusters(ctx context.Context, tenantID string) ([]*Cluster, error) {
	return s.List(ctx, tenantID)
}

func (s *ClusterStore) DeleteCluster(ctx context.Context, id, tenantID string) error {
	return s.Unregister(ctx, id, tenantID)
}

func (s *ClusterStore) HeartbeatCluster(ctx context.Context, id, tenantID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin heartbeat transaction: %w", err)
	}
	defer tx.Rollback()

	var exists string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM clusters WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenantID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClusterNotFound
	}
	if err != nil {
		return fmt.Errorf("lock cluster for heartbeat: %w", err)
	}

	hbID := uuid.NewString()
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_heartbeats (id, cluster_id, status, version, node_count, capacity, observed_at)
		VALUES ($1, $2, 'healthy', NULL, 0, '{}', $3)`,
		hbID, id, now,
	)
	if err != nil {
		return fmt.Errorf("insert heartbeat: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE clusters SET status = 'healthy', last_heartbeat = $1, updated_at = $1
		WHERE id = $2 AND tenant_id = $3`,
		now, id, tenantID,
	)
	if err != nil {
		return fmt.Errorf("update cluster heartbeat: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect heartbeat update: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("heartbeat update affected %d rows, expected 1", rows)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit heartbeat transaction: %w", err)
	}
	return nil
}

func (s *ClusterStore) UpdateCluster(ctx context.Context, id, tenantID string, req UpdateClusterRequest) (*Cluster, error) {
	c, err := s.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	regionSet := req.Region != nil
	regionVal := ""
	if regionSet {
		regionVal = *req.Region
	}
	zoneSet := req.Zone != nil
	zoneVal := ""
	if zoneSet {
		zoneVal = *req.Zone
	}
	statusSet := req.Status != nil
	statusVal := ""
	if statusSet {
		statusVal = *req.Status
	}
	labelsSet := req.Labels != nil
	// "null" is a valid JSON literal: when labels are not being updated the
	// CASE branch is not taken, but the parameter must still cast cleanly
	// (a nil []byte parameter encodes as an empty string, which fails ::jsonb).
	labelsVal := json.RawMessage("null")
	if labelsSet {
		labelsVal, err = json.Marshal(*req.Labels)
		if err != nil {
			return nil, err
		}
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE clusters SET
			region = CASE WHEN $1 THEN NULLIF($2, '') ELSE region END,
			zone = CASE WHEN $3 THEN NULLIF($4, '') ELSE zone END,
			labels = CASE WHEN $5 THEN $6::jsonb ELSE labels END,
			status = CASE WHEN $7 THEN NULLIF($8, '') ELSE status END,
			version = version + 1,
			updated_at = now()
		WHERE id = $9 AND tenant_id = $10 AND version = $11`,
		regionSet, regionVal, zoneSet, zoneVal, labelsSet, labelsVal, statusSet, statusVal, id, tenantID, c.Version)
	if err != nil {
		return nil, err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, fmt.Errorf("optimistic lock failed: version mismatch")
	}
	return s.Get(ctx, id, tenantID)
}

// Request / response types
type CreateClusterRequest struct {
	Name          string            `json:"name"`
	TenantID      string            `json:"tenantId"`
	ClusterType   string            `json:"clusterType"`
	APIEndpoint   string            `json:"apiEndpoint"`
	KubeconfigRef string            `json:"kubeconfigRef,omitempty"`
	Region        string            `json:"region,omitempty"`
	Zone          string            `json:"zone,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

type UpdateClusterRequest struct {
	Region *string            `json:"region,omitempty"`
	Zone   *string            `json:"zone,omitempty"`
	Labels *map[string]string `json:"labels,omitempty"`
	Status *string            `json:"status,omitempty"`
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

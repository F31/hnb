package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/core"
)

type RuntimeTargetStore struct {
	db *sql.DB
}

func NewRuntimeTargetStore(db *sql.DB) *RuntimeTargetStore {
	return &RuntimeTargetStore{db: db}
}

func (s *RuntimeTargetStore) Create(rt *core.RuntimeTarget) error {
	labelsJSON, err := json.Marshal(rt.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	edgeConfigJSON, err := json.Marshal(rt.EdgeConfig)
	if err != nil {
		return fmt.Errorf("marshal edge_config: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO runtime_targets (
			id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active
		) VALUES (
			$1, $2, $3, NULLIF($4, ''), $5, $6,
			NULLIF($7, ''), $8,
			$9, NULLIF($10, ''), NULLIF($11, ''),
			$12, $13, $14, $15, $16
		)`,
		rt.ID, rt.TenantID, rt.Name, rt.DisplayName, string(rt.TargetType), string(rt.Distribution),
		string(rt.EdgeType), string(edgeConfigJSON),
		string(rt.ConnectionType), rt.ConnectionEndpoint, rt.AgentVersion,
		string(rt.Status), string(labelsJSON), rt.ObservedAt, rt.StaleThresholdSec, rt.IsActive,
	)
	return err
}

func (s *RuntimeTargetStore) Get(id string) (*core.RuntimeTarget, error) {
	rt := &core.RuntimeTarget{}
	var displayName, connectionEndpoint, agentVersion, edgeType sql.NullString
	var labelsJSON, edgeConfigJSON []byte
	var observedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active,
			created_at, updated_at
		FROM runtime_targets WHERE id = $1`, id,
	).Scan(
		&rt.ID, &rt.TenantID, &rt.Name, &displayName, &rt.TargetType, &rt.Distribution,
		&edgeType, &edgeConfigJSON,
		&rt.ConnectionType, &connectionEndpoint, &agentVersion,
		&rt.Status, &labelsJSON, &observedAt, &rt.StaleThresholdSec, &rt.IsActive,
		&rt.CreatedAt, &rt.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if displayName.Valid {
		rt.DisplayName = displayName.String
	}
	if connectionEndpoint.Valid {
		rt.ConnectionEndpoint = connectionEndpoint.String
	}
	if agentVersion.Valid {
		rt.AgentVersion = agentVersion.String
	}
	if edgeType.Valid {
		rt.EdgeType = core.EdgeType(edgeType.String)
	}
	if observedAt.Valid {
		rt.ObservedAt = &observedAt.Time
	}
	if err := json.Unmarshal(labelsJSON, &rt.Labels); err != nil {
		return nil, fmt.Errorf("decode labels: %w", err)
	}
	if edgeConfigJSON != nil {
		if err := json.Unmarshal(edgeConfigJSON, &rt.EdgeConfig); err != nil {
			return nil, fmt.Errorf("decode edge_config: %w", err)
		}
	}

	return rt, nil
}

func (s *RuntimeTargetStore) ListByTenant(tenantID string) ([]*core.RuntimeTarget, error) {
	rows, err := s.db.Query(`
		SELECT id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active,
			created_at
		FROM runtime_targets WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTargets(rows)
}

func (s *RuntimeTargetStore) ListByType(targetType core.TargetType) ([]*core.RuntimeTarget, error) {
	rows, err := s.db.Query(`
		SELECT id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active,
			created_at
		FROM runtime_targets WHERE target_type = $1 ORDER BY created_at DESC`, string(targetType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTargets(rows)
}

func (s *RuntimeTargetStore) UpdateStatus(id string, status core.TargetStatus, observedAt time.Time) error {
	result, err := s.db.Exec(`
		UPDATE runtime_targets SET status = $1, observed_at = $2, updated_at = now()
		WHERE id = $3`, string(status), observedAt, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("runtime target %q not found", id)
	}
	return nil
}

func (s *RuntimeTargetStore) UpdateDistribution(id string, dist core.Distribution) error {
	_, err := s.db.Exec(`
		UPDATE runtime_targets SET distribution = $1, updated_at = now()
		WHERE id = $2`, string(dist), id)
	return err
}

func (s *RuntimeTargetStore) UpdateEdgeConfig(id string, edgeType core.EdgeType, config map[string]any) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal edge_config: %w", err)
	}
	_, err = s.db.Exec(`
		UPDATE runtime_targets SET edge_type = $1, edge_config = $2, updated_at = now()
		WHERE id = $3`, string(edgeType), string(configJSON), id)
	return err
}

func scanTargets(rows *sql.Rows) ([]*core.RuntimeTarget, error) {
	var targets []*core.RuntimeTarget
	for rows.Next() {
		rt := &core.RuntimeTarget{}
		var displayName, connectionEndpoint, agentVersion, edgeType sql.NullString
		var labelsJSON, edgeConfigJSON []byte
		var observedAt sql.NullTime

		err := rows.Scan(
			&rt.ID, &rt.TenantID, &rt.Name, &displayName, &rt.TargetType, &rt.Distribution,
			&edgeType, &edgeConfigJSON,
			&rt.ConnectionType, &connectionEndpoint, &agentVersion,
			&rt.Status, &labelsJSON, &observedAt, &rt.StaleThresholdSec, &rt.IsActive,
			&rt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if displayName.Valid {
			rt.DisplayName = displayName.String
		}
		if connectionEndpoint.Valid {
			rt.ConnectionEndpoint = connectionEndpoint.String
		}
		if agentVersion.Valid {
			rt.AgentVersion = agentVersion.String
		}
		if edgeType.Valid {
			rt.EdgeType = core.EdgeType(edgeType.String)
		}
		if observedAt.Valid {
			rt.ObservedAt = &observedAt.Time
		}
		if err := json.Unmarshal(labelsJSON, &rt.Labels); err != nil {
			return nil, fmt.Errorf("decode labels: %w", err)
		}
		if edgeConfigJSON != nil {
			if err := json.Unmarshal(edgeConfigJSON, &rt.EdgeConfig); err != nil {
				return nil, fmt.Errorf("decode edge_config: %w", err)
			}
		}

		targets = append(targets, rt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}
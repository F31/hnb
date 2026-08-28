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

func (s *PGStore) CreateRuntimeTarget(ctx context.Context, rt *RuntimeTarget) error {
	rt.ID = uuid.NewString()
	rt.CreatedAt = time.Now().UTC()
	if rt.Status == "" {
		rt.Status = "unknown"
	}
	if rt.Distribution == "" {
		rt.Distribution = "standard"
	}
	if rt.ConnectionType == "" {
		rt.ConnectionType = "agent"
	}
	if rt.StaleThresholdSec == 0 {
		rt.StaleThresholdSec = 300
	}
	rt.IsActive = true

	labelsJSON, err := json.Marshal(rt.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	edgeConfigJSON, err := json.Marshal(rt.EdgeConfig)
	if err != nil {
		return fmt.Errorf("marshal edge_config: %w", err)
	}
	credentialRefParam, err := credentialRefParam(rt.CredentialRef)
	if err != nil {
		return fmt.Errorf("marshal credential_ref: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		WITH selected_workspace AS (
			INSERT INTO workspaces (tenant_id, name, display_name)
			VALUES ($2, 'default', 'Default')
			ON CONFLICT (tenant_id, name) DO UPDATE
				SET updated_at = workspaces.updated_at
			RETURNING id
		)
		INSERT INTO runtime_targets (
			id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active,
			credential_ref, workspace_id
		) SELECT
			$1, $2, $3, NULLIF($4, ''), $5, $6,
			NULLIF($7, ''), $8,
			$9, NULLIF($10, ''), NULLIF($11, ''),
			$12, $13, $14, $15, $16,
			$17::jsonb, id
		FROM selected_workspace`,
		rt.ID, rt.TenantID, rt.Name, rt.DisplayName, rt.TargetType, rt.Distribution,
		rt.EdgeType, string(edgeConfigJSON),
		rt.ConnectionType, rt.ConnectionEndpoint, rt.AgentVersion,
		rt.Status, string(labelsJSON), rt.ObservedAt, rt.StaleThresholdSec, rt.IsActive,
		credentialRefParam,
	)
	return err
}

// credentialRefParam renders the bound credential reference as a jsonb
// parameter; a nil reference yields a SQL NULL (not a JSON null literal).
func credentialRefParam(ref *CredentialRef) (interface{}, error) {
	if ref == nil {
		return nil, nil
	}
	b, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *PGStore) GetRuntimeTarget(ctx context.Context, id, tenantID string) (*RuntimeTarget, error) {
	rt := &RuntimeTarget{}
	var displayName, connectionEndpoint, agentVersion, kubernetesVersion, edgeType sql.NullString
	var labelsJSON, edgeConfigJSON, credentialRefJSON []byte
	var observedAt, lastKnownStateAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active,
			created_at, updated_at, projection_version,
			last_known_state_at, COALESCE(lifecycle_state, 'UNKNOWN'),
			COALESCE(health_state, 'UNKNOWN'), COALESCE(connectivity_state, 'UNKNOWN'),
			COALESCE(freshness_state, 'UNKNOWN'), COALESCE(observation_generation, 0),
			COALESCE(observation_revision, 0), credential_ref,
			(SELECT cs.kube_version FROM capability_snapshots cs
			 WHERE cs.tenant_id = runtime_targets.tenant_id AND cs.target_id = runtime_targets.id
			 ORDER BY cs.observed_at DESC LIMIT 1)
		FROM runtime_targets WHERE id = $1 AND (tenant_id = $2 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=runtime_targets.id AND tca.tenant_id=$2 AND tca.status='active'))`, id, tenantID,
	).Scan(
		&rt.ID, &rt.TenantID, &rt.Name, &displayName, &rt.TargetType, &rt.Distribution,
		&edgeType, &edgeConfigJSON,
		&rt.ConnectionType, &connectionEndpoint, &agentVersion,
		&rt.Status, &labelsJSON, &observedAt, &rt.StaleThresholdSec, &rt.IsActive,
		&rt.CreatedAt, &rt.UpdatedAt, &rt.ProjectionVersion,
		&lastKnownStateAt, &rt.LifecycleState, &rt.HealthState, &rt.ConnectivityState,
		&rt.FreshnessState, &rt.ObservationGeneration, &rt.ObservationRevision,
		&credentialRefJSON,
		&kubernetesVersion,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
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
	if kubernetesVersion.Valid {
		rt.KubernetesVersion = kubernetesVersion.String
	}
	if edgeType.Valid {
		rt.EdgeType = edgeType.String
	}
	if observedAt.Valid {
		rt.ObservedAt = &observedAt.Time
	}
	if lastKnownStateAt.Valid {
		rt.LastKnownStateAt = &lastKnownStateAt.Time
	}
	if err := json.Unmarshal(labelsJSON, &rt.Labels); err != nil {
		return nil, fmt.Errorf("decode labels: %w", err)
	}
	if edgeConfigJSON != nil {
		if err := json.Unmarshal(edgeConfigJSON, &rt.EdgeConfig); err != nil {
			return nil, fmt.Errorf("decode edge_config: %w", err)
		}
	}
	if credentialRefJSON != nil && string(credentialRefJSON) != "null" {
		ref := &CredentialRef{}
		if err := json.Unmarshal(credentialRefJSON, ref); err != nil {
			return nil, fmt.Errorf("decode credential_ref: %w", err)
		}
		rt.CredentialRef = ref
	}

	return rt, nil
}

func (s *PGStore) ListRuntimeTargets(ctx context.Context, tenantID string) ([]*RuntimeTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, display_name, target_type, distribution,
			edge_type, edge_config,
			connection_type, connection_endpoint, agent_version,
			status, labels, observed_at, stale_threshold_seconds, is_active,
			created_at
		FROM runtime_targets WHERE tenant_id = $1 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=runtime_targets.id AND tca.tenant_id=$1 AND tca.status='active') ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPlatformTargets(rows)
}

func (s *PGStore) UpdateRuntimeTargetDescription(ctx context.Context, id, tenantID, description string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE runtime_targets SET description = $1, updated_at = now()
		WHERE id = $2 AND tenant_id = $3`, description, id, tenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func (s *PGStore) UpdateRuntimeTargetStatus(ctx context.Context, id, tenantID string, status string, observedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE runtime_targets SET status = $1, observed_at = $2, updated_at = now()
		WHERE id = $3 AND tenant_id = $4`, status, observedAt, id, tenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func (s *PGStore) DeleteRuntimeTarget(ctx context.Context, id, tenantID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id = $1 AND (tenant_id = $2 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=runtime_targets.id AND tca.tenant_id=$2 AND tca.status='active'))`, id, tenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func scanPlatformTargets(rows *sql.Rows) ([]*RuntimeTarget, error) {
	var targets []*RuntimeTarget
	for rows.Next() {
		rt := &RuntimeTarget{}
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
			rt.EdgeType = edgeType.String
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

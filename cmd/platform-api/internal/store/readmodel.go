package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// GetOperation reads the query projection (operation_read_model) joined with
// authoritative step rows. It never scans the write-side operations table on
// the request path (KERNEL-003).
func (s *PGStore) GetOperation(ctx context.Context, id, tenantID string) (*Operation, error) {
	op := &Operation{}
	var projectID, environmentID, namespaceID, planID, approvedBy sql.NullString
	var startedAt, completedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT operation_id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, total_steps, completed_steps, failed_steps,
			initiated_by, approved_by, target_cluster_ids,
			created_at, started_at, completed_at,
			last_state_changed_at
		FROM operation_read_model
		WHERE operation_id = $1 AND tenant_id = $2`, id, tenantID,
	).Scan(
		&op.ID, &op.TenantID, &projectID, &environmentID, &namespaceID,
		&planID, &op.OperationType, &op.Status, &op.TotalSteps, &op.CompletedSteps, &op.FailedSteps,
		&op.InitiatedBy, &approvedBy, pq.Array(&op.TargetClusterIDs),
		&op.CreatedAt, &startedAt, &completedAt,
		&op.LastStateChangedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query operation read model: %w", err)
	}
	op.ProjectID = projectID.String
	op.EnvironmentID = environmentID.String
	op.NamespaceID = namespaceID.String
	op.PlanID = planID.String
	op.ApprovedBy = approvedBy.String
	if startedAt.Valid {
		op.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		op.CompletedAt = &completedAt.Time
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(plan_step_id, ''), step_name, step_type,
			COALESCE(provider_id, ''), status, depends_on,
			COALESCE(error_message, ''), started_at, completed_at
		FROM operation_steps
		WHERE operation_id = $1
		ORDER BY created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("query operation steps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var step Step
		var startedAt, completedAt sql.NullTime
		var dependsOn []string
		if err := rows.Scan(
			&step.ID, &step.PlanStepID, &step.Name, &step.StepType,
			&step.ProviderID, &step.Status, pq.Array(&dependsOn),
			&step.ErrorMessage, &startedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan operation step: %w", err)
		}
		step.DependsOn = dependsOn
		if startedAt.Valid {
			step.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			step.CompletedAt = &completedAt.Time
		}
		op.Steps = append(op.Steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation steps: %w", err)
	}
	return op, nil
}

// ListOperations queries only operation_read_model, always scoped by tenant.
func (s *PGStore) ListOperations(ctx context.Context, q ListQuery) ([]OperationSummary, int, error) {
	conditions := []string{"tenant_id = $1"}
	args := []any{q.TenantID}
	if q.Status != "" {
		args = append(args, q.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if q.OperationType != "" {
		args = append(args, q.OperationType)
		conditions = append(conditions, fmt.Sprintf("operation_type = $%d", len(args)))
	}
	where := strings.Join(conditions, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM operation_read_model WHERE `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count operation read model: %w", err)
	}

	queryArgs := append(append([]any{}, args...), q.Limit, q.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT operation_id, tenant_id, project_id, environment_id, namespace_id,
			operation_type, status, total_steps, completed_steps, failed_steps,
			initiated_by, approved_by, COALESCE(summary, ''),
			target_cluster_ids, COALESCE(intent_id::text, ''),
			created_at, started_at, completed_at, last_state_changed_at
		FROM operation_read_model
		WHERE `+where+`
		ORDER BY created_at DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2),
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list operation read model: %w", err)
	}
	defer rows.Close()

	var summaries []OperationSummary
	for rows.Next() {
		var item OperationSummary
		var projectID, environmentID, namespaceID, approvedBy sql.NullString
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.TenantID, &projectID, &environmentID, &namespaceID,
			&item.OperationType, &item.Status, &item.TotalSteps, &item.CompletedSteps, &item.FailedSteps,
			&item.InitiatedBy, &approvedBy, &item.Summary,
			pq.Array(&item.TargetClusterIDs), &item.IntentID,
			&item.CreatedAt, &startedAt, &completedAt, &item.LastStateChangedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan operation summary: %w", err)
		}
		item.ProjectID = projectID.String
		item.EnvironmentID = environmentID.String
		item.NamespaceID = namespaceID.String
		item.ApprovedBy = approvedBy.String
		if startedAt.Valid {
			item.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}
		summaries = append(summaries, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate operation summaries: %w", err)
	}
	return summaries, total, nil
}

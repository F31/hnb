package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// CreateOperationBatch persists only the parent receipt. Child operations are
// created by the API orchestrator after per-target admission succeeds.
func (s *PGStore) CreateOperationBatch(ctx context.Context, batch OperationBatch) (*OperationBatch, bool, error) {
	if batch.Kind != "BatchDeleteRuntimeTargets" || batch.TenantID == "" || batch.IdempotencyKey == "" || len(batch.TargetIDs) == 0 {
		return nil, false, fmt.Errorf("invalid operation batch")
	}
	if batch.ID == "" {
		batch.ID = uuid.NewString()
	}
	if batch.Status == "" {
		batch.Status = "pending"
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO operation_batches
		(id, tenant_id, kind, status, initiated_by, correlation_id, idempotency_key, total_children)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tenant_id,idempotency_key) DO NOTHING RETURNING created_at,updated_at`,
		batch.ID, batch.TenantID, batch.Kind, batch.Status, batch.InitiatedBy, batch.CorrelationID, batch.IdempotencyKey, len(batch.TargetIDs)).Scan(&batch.CreatedAt, &batch.UpdatedAt)
	if err == nil {
		return &batch, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id,tenant_id,kind,status,initiated_by,correlation_id,idempotency_key,total_children,succeeded_children,failed_children,cancelled_children,created_at,updated_at FROM operation_batches WHERE tenant_id=$1 AND idempotency_key=$2`, batch.TenantID, batch.IdempotencyKey)
	var out OperationBatch
	if err = row.Scan(&out.ID, &out.TenantID, &out.Kind, &out.Status, &out.InitiatedBy, &out.CorrelationID, &out.IdempotencyKey, &out.TotalChildren, &out.SucceededChildren, &out.FailedChildren, &out.CancelledChildren, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return nil, false, err
	}
	if out.Kind != batch.Kind {
		return nil, false, ErrIdempotencyConflict
	}
	return &out, false, nil
}

func (s *PGStore) GetOperationBatch(ctx context.Context, id, tenantID string) (*OperationBatch, error) {
	var out OperationBatch
	err := s.db.QueryRowContext(ctx, `SELECT b.id,b.tenant_id,b.kind,b.status,b.initiated_by,b.correlation_id,b.idempotency_key,b.total_children,b.succeeded_children,b.failed_children,b.cancelled_children,b.created_at,b.updated_at,COALESCE(array_agg(c.target_id::text) FILTER (WHERE c.target_id IS NOT NULL), '{}') FROM operation_batches b LEFT JOIN operation_batch_children c ON c.batch_id=b.id WHERE b.id=$1 AND b.tenant_id=$2 GROUP BY b.id`, id, tenantID).Scan(&out.ID, &out.TenantID, &out.Kind, &out.Status, &out.InitiatedBy, &out.CorrelationID, &out.IdempotencyKey, &out.TotalChildren, &out.SucceededChildren, &out.FailedChildren, &out.CancelledChildren, &out.CreatedAt, &out.UpdatedAt, pq.Array(&out.TargetIDs))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &out, err
}

func (s *PGStore) AttachOperationBatchChild(ctx context.Context, batchID, operationID, targetID string, ordinal int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO operation_batch_children(batch_id,operation_id,target_id,ordinal) VALUES($1,$2,$3,$4) ON CONFLICT (batch_id,target_id) DO NOTHING`, batchID, operationID, targetID, ordinal)
	return err
}

// RefreshOperationBatchStatus derives the parent outcome from durable child
// Operations. It intentionally never cancels a running child.
func (s *PGStore) RefreshOperationBatchStatus(ctx context.Context, batchID, tenantID string) (*OperationBatch, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE operation_batches b SET
		succeeded_children=x.succeeded, failed_children=x.failed, cancelled_children=x.cancelled,
		status=CASE WHEN x.running>0 THEN 'running'
			WHEN x.failed>0 AND x.succeeded>0 THEN 'partial_succeeded'
			WHEN x.failed>0 THEN 'failed'
			WHEN x.cancelled=x.total AND x.total>0 THEN 'cancelled'
			WHEN x.succeeded=x.total AND x.total>0 THEN 'succeeded'
			ELSE 'pending' END, updated_at=now()
		FROM (SELECT c.batch_id,count(*) AS total,
			count(*) FILTER (WHERE o.status='succeeded') AS succeeded,
			count(*) FILTER (WHERE o.status='failed') AS failed,
			count(*) FILTER (WHERE o.status='cancelled') AS cancelled,
			count(*) FILTER (WHERE o.status IN ('pending','pending_approval','queued','queued_offline','in_progress','paused','compensating')) AS running
			FROM operation_batch_children c JOIN operations o ON o.id=c.operation_id WHERE c.batch_id=$1 GROUP BY c.batch_id) x
		WHERE b.id=x.batch_id AND b.tenant_id=$2`, batchID, tenantID)
	if err != nil {
		return nil, err
	}
	return s.GetOperationBatch(ctx, batchID, tenantID)
}

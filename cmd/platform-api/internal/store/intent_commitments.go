package store

import (
	"context"
	"database/sql"
	"errors"
)

func (s *PGStore) GetIntentCommitment(ctx context.Context, tenantID, kind, action, idempotencyKey string) (*IntentCommitment, error) {
	return scanIntentCommitment(s.db.QueryRowContext(ctx, `
		SELECT id, execution_plan_id, operation_id, semantic_digest, intent_kind,
		       commitment_action, accepted_status, correlation_id, created_at, response_http_status
		FROM runtime_intents
		WHERE tenant_id = $1 AND intent_kind = $2 AND commitment_action = $3 AND idempotency_key = $4`,
		tenantID, kind, action, idempotencyKey))
}

type commitmentRow interface {
	Scan(...any) error
}

func scanIntentCommitment(row commitmentRow) (*IntentCommitment, error) {
	commitment := &IntentCommitment{}
	err := row.Scan(&commitment.IntentID, &commitment.ExecutionPlanID, &commitment.OperationID,
		&commitment.SemanticDigest, &commitment.Kind, &commitment.Action, &commitment.AcceptedStatus,
		&commitment.CorrelationID, &commitment.CreatedAt, &commitment.HTTPStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return commitment, err
}

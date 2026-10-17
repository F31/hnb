package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

// stepRequestedMessageType is the command the worker consumes to execute a step.
const stepRequestedMessageType = "hnb.command.operation.step-requested.v1"

// RedriveResult reports what one sweep did.
type RedriveResult struct {
	Requeued  []string
	Exhausted []string
}

// RequeueOrphanedSteps re-publishes step requests that were delivered to a consumer
// that then went away. JetStream redelivery only helps while a consumer is attached,
// so a message that is acked or consumed by a terminating worker can strand its step
// in "pending" forever.
//
// The sweep is safe to run concurrently and repeatedly: candidates are locked with
// SKIP LOCKED, steps that still have an unpublished request are skipped, and step
// execution remains single-winner because AcquireLease refuses a second owner. A
// re-drive re-arms the original row without changing its payload or identity.
func (s *OperationStore) RequeueOrphanedSteps(ctx context.Context, idleFor time.Duration, limit int) (RedriveResult, error) {
	var result RedriveResult
	if idleFor <= 0 || limit <= 0 {
		return result, fmt.Errorf("redrive requires a positive idle threshold and limit")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin redrive transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT step.id, step.operation_id, step.updated_at, step.max_retries,
			request.message_id, request.attempt, request.max_attempts
		FROM operation_steps AS step
		JOIN operations AS operation ON operation.id = step.operation_id
		JOIN LATERAL (
			SELECT event.message_id, event.attempt, event.max_attempts
			FROM outbox_events AS event
			WHERE event.step_id = step.id AND event.message_type = $1 AND event.status = $2
				AND event.idempotency_key = step.idempotency_key
			ORDER BY event.occurred_at DESC
			LIMIT 1
		) AS request ON true
		WHERE step.status = $3
			AND operation.status IN ($4, $5)
			AND NOT EXISTS (
				SELECT 1 FROM outbox_events AS unpublished
				WHERE unpublished.step_id = step.id AND unpublished.message_type = $1
					AND unpublished.status <> $2
			)
			AND step.updated_at < now() - ($6 * interval '1 millisecond')
		ORDER BY step.updated_at
		LIMIT $7
		FOR UPDATE OF step SKIP LOCKED`,
		stepRequestedMessageType, "published", engine.StepPending,
		engine.StatusQueued, engine.StatusInProgress,
		idleFor.Milliseconds(), limit)
	if err != nil {
		return result, fmt.Errorf("select orphaned steps: %w", err)
	}
	type candidate struct {
		id, operationID, messageID string
		attempt, maxAttempts       int
		maxRetries                 int
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.operationID, new(time.Time), &c.maxRetries,
			&c.messageID, &c.attempt, &c.maxAttempts); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan orphaned step: %w", err)
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}

	for _, c := range candidates {
		// The relay only publishes attempts strictly below max_attempts. Leave
		// room for the increment, otherwise we strand an unpublishable pending row.
		if c.attempt >= c.maxRetries || c.attempt >= c.maxAttempts-1 {
			result.Exhausted = append(result.Exhausted, c.id)
			continue
		}
		if err := requeueStepRequest(ctx, tx, c); err != nil {
			return result, err
		}
		result.Requeued = append(result.Requeued, c.id)
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit redrive: %w", err)
	}
	return result, nil
}

// requeueStepRequest re-arms the committed request for another delivery.
//
// It must reuse the existing outbox row rather than inserting a copy: the row is
// unique per (tenant, message type, idempotency key), and operation-worker
// requires the message envelope, the payload and the stored step to all carry the
// same idempotency key. A copy with a derived key is rejected as "envelope and
// payload idempotencyKey differ", and rewriting the payload would break the match
// against operation_steps. Resetting the row's delivery state is what the outbox
// attempt counter exists for, and it keeps one auditable row per request.
func requeueStepRequest(ctx context.Context, tx *sql.Tx, c struct {
	id, operationID, messageID string
	attempt, maxAttempts       int
	maxRetries                 int
}) error {
	var updated int
	if err := tx.QueryRowContext(ctx, `
		UPDATE outbox_events SET
			status = 'pending', attempt = attempt + 1, next_attempt_at = now(),
			last_error = NULL, updated_at = now()
		WHERE message_id = $1 AND status = 'published'
		RETURNING 1`, c.messageID).Scan(&updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Another sweep already re-armed this request.
			return nil
		}
		return fmt.Errorf("re-arm step request: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE operation_steps SET updated_at = now() WHERE id = $1`, c.id); err != nil {
		return fmt.Errorf("mark redriven step: %w", err)
	}
	return nil
}

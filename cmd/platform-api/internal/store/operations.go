package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type PGStore struct {
	db           *sql.DB
	clusterStore *ClusterStore
	dialect      Dialect
}

func NewPGStore(db *sql.DB) *PGStore {
	return &PGStore{
		db:           db,
		clusterStore: NewClusterStore(db),
		dialect:      postgresDialect{},
	}
}

func (s *PGStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *PGStore) Ready(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// SubmitOperation persists execution_plans + operations + operation_steps +
// operation_audit + operation_read_model (+ step-requested outbox events when
// the operation starts queued) in a single transaction (OP-007). A repeated
// idempotency key returns the existing Operation with created=false.
func (s *PGStore) SubmitOperation(ctx context.Context, cmd SubmitCommand) (*Operation, bool, error) {
	planDigest, planJSON, err := buildPlan(cmd)
	if err != nil {
		return nil, false, err
	}
	correlationID := cmd.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin submit transaction: %w", err)
	}
	defer tx.Rollback()

	var planID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO execution_plans (
			release_id, tenant_id, project_id, environment_id,
			plan_digest, plan_json, policy_result, status
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, '{}', 'active')
		ON CONFLICT (tenant_id, plan_digest) DO NOTHING
		RETURNING id`,
		cmd.ReleaseID, cmd.TenantID, cmd.ProjectID, cmd.EnvironmentID,
		planDigest, string(planJSON),
	).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM execution_plans WHERE tenant_id = $1 AND plan_digest = $2`, cmd.TenantID, planDigest,
		).Scan(&planID)
	}
	if err != nil {
		return nil, false, fmt.Errorf("persist execution plan: %w", err)
	}

	opID := uuid.NewString()
	initialStatus := InitialStatus(cmd.OperationType)
	tagsJSON, err := json.Marshal(cmd.Tags)
	if err != nil {
		return nil, false, fmt.Errorf("marshal operation tags: %w", err)
	}
	var existingOpID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, plan_digest, status_reason,
			total_steps, target_cluster_ids, tags
		) VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			$6, $7, $8, $9,
			$10, $11, $12, '',
			$13, $14, $15
		) ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
		RETURNING id`,
		opID, cmd.TenantID, cmd.ProjectID, cmd.EnvironmentID, cmd.NamespaceID,
		planID, cmd.OperationType, initialStatus, cmd.InitiatedBy,
		correlationID, cmd.IdempotencyKey, planDigest,
		len(cmd.Steps), pq.Array(cmd.TargetClusterIDs), string(tagsJSON),
	).Scan(&existingOpID)
	if errors.Is(err, sql.ErrNoRows) {
		existing, getErr := s.getByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
		if getErr != nil {
			return nil, false, getErr
		}
		if existing.PlanDigest != "" && existing.PlanDigest != planDigest {
			return nil, false, fmt.Errorf("%w: idempotency key %q", ErrIdempotencyConflict, cmd.IdempotencyKey)
		}
		return existing, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("insert operation: %w", err)
	}

	stepIDs := make([]string, len(cmd.Steps))
	for i, step := range cmd.Steps {
		if step.MaxRetries <= 0 {
			step.MaxRetries = 3
		}
		if step.TimeoutSeconds <= 0 {
			step.TimeoutSeconds = 300
		}
		stepID := uuid.NewString()
		stepIDs[i] = stepID
		planStepID := planStepID(step, i)
		inputsJSON, err := marshalStepInputs(step)
		if err != nil {
			return nil, false, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO operation_steps (
				id, operation_id, plan_step_id, step_name, step_type, provider_id,
				status, idempotency_key, depends_on, optional,
				max_retries, timeout_seconds, step_input
			) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), 'pending', $7, $8, $9, $10, $11, $12)`,
			stepID, opID, planStepID, step.Name, step.StepType, step.ProviderID,
			StepIdempotencyKey(cmd.IdempotencyKey, planStepID), pq.Array(step.DependsOn), step.Optional,
			step.MaxRetries, step.TimeoutSeconds, string(inputsJSON),
		)
		if err != nil {
			return nil, false, fmt.Errorf("insert operation step %q: %w", planStepID, err)
		}
	}

	if err := insertAudit(ctx, tx, opID, "created", cmd.InitiatedBy, "", initialStatus, map[string]any{
		"operation_type": cmd.OperationType,
		"release_id":     cmd.ReleaseID,
		"plan_digest":    planDigest,
	}); err != nil {
		return nil, false, err
	}

	op := &Operation{
		ID: opID, TenantID: cmd.TenantID, ProjectID: cmd.ProjectID,
		EnvironmentID: cmd.EnvironmentID, NamespaceID: cmd.NamespaceID,
		PlanID: planID, OperationType: cmd.OperationType, Status: initialStatus,
		InitiatedBy: cmd.InitiatedBy, CorrelationID: correlationID,
		IdempotencyKey: cmd.IdempotencyKey, PlanDigest: planDigest,
		TotalSteps: len(cmd.Steps), TargetClusterIDs: cmd.TargetClusterIDs,
		Tags: cmd.Tags, CreatedAt: time.Now().UTC(),
	}
	if err := upsertReadModel(ctx, tx, op); err != nil {
		return nil, false, err
	}

	if initialStatus == StatusQueued {
		for i, step := range cmd.Steps {
			if len(step.DependsOn) > 0 {
				continue
			}
			if err := insertStepRequestedEvent(ctx, tx, op, stepIDs[i], step, i, cmd.InitiatedBy, 0); err != nil {
				return nil, false, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit submit transaction: %w", err)
	}
	created, err := s.GetOperation(ctx, opID, cmd.TenantID)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

func (s *PGStore) ApproveOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin approve transaction: %w", err)
	}
	defer tx.Rollback()

	op, err := lockOperation(ctx, tx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if op.Status != StatusPendingApproval {
		return nil, fmt.Errorf("%w: cannot approve operation in %s state", ErrInvalidState, op.Status)
	}

	if err := transitionOperation(ctx, tx, op, StatusQueued, actorID, reason, true); err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, tx, op.ID, "approved", actorID, StatusPendingApproval, StatusQueued, map[string]any{
		"reason": reason,
	}); err != nil {
		return nil, err
	}
	if err := upsertReadModel(ctx, tx, op); err != nil {
		return nil, err
	}
	if err := s.dispatchReadySteps(ctx, tx, op, actorID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit approve transaction: %w", err)
	}
	return s.GetOperation(ctx, id, tenantID)
}

func (s *PGStore) RejectOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin reject transaction: %w", err)
	}
	defer tx.Rollback()

	op, err := lockOperation(ctx, tx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if op.Status != StatusPendingApproval {
		return nil, fmt.Errorf("%w: cannot reject operation in %s state", ErrInvalidState, op.Status)
	}

	previous := op.Status
	if err := transitionOperation(ctx, tx, op, StatusCancelled, actorID, reason, true); err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, tx, op.ID, "rejected", actorID, previous, StatusCancelled, map[string]any{
		"reason": reason,
	}); err != nil {
		return nil, err
	}
	if err := upsertReadModel(ctx, tx, op); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit reject transaction: %w", err)
	}
	return s.GetOperation(ctx, id, tenantID)
}

func (s *PGStore) CancelOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin cancel transaction: %w", err)
	}
	defer tx.Rollback()

	op, err := lockOperation(ctx, tx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if !CanTransition(op.Status, StatusCancelled) {
		return nil, fmt.Errorf("%w: cannot cancel operation in %s state", ErrInvalidState, op.Status)
	}

	previous := op.Status
	if err := transitionOperation(ctx, tx, op, StatusCancelled, actorID, reason, false); err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, tx, op.ID, "cancelled", actorID, previous, StatusCancelled, map[string]any{
		"reason": reason,
	}); err != nil {
		return nil, err
	}
	if err := upsertReadModel(ctx, tx, op); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit cancel transaction: %w", err)
	}
	return s.GetOperation(ctx, id, tenantID)
}

// dispatchReadySteps emits one step-requested outbox event per pending step
// without dependencies. Called when an operation becomes queued after approval.
func (s *PGStore) dispatchReadySteps(ctx context.Context, tx *sql.Tx, op *Operation, actorID string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(plan_step_id, ''), step_type, idempotency_key, version
		FROM operation_steps
		WHERE operation_id = $1 AND status = 'pending'
			AND (depends_on IS NULL OR depends_on = '{}')`, op.ID)
	if err != nil {
		return fmt.Errorf("select ready steps: %w", err)
	}
	defer rows.Close()

	type readyStep struct {
		id, planStepID, stepType, idempotencyKey string
		version                                  int64
	}
	var ready []readyStep
	for rows.Next() {
		var st readyStep
		if err := rows.Scan(&st.id, &st.planStepID, &st.stepType, &st.idempotencyKey, &st.version); err != nil {
			return fmt.Errorf("scan ready step: %w", err)
		}
		ready = append(ready, st)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate ready steps: %w", err)
	}

	for _, st := range ready {
		payload := map[string]any{
			"operationId":     op.ID,
			"stepId":          st.id,
			"stepType":        st.stepType,
			"idempotencyKey":  st.idempotencyKey,
			"expectedVersion": st.version,
		}
		if err := insertOutboxEvent(ctx, tx, outboxEvent{
			MessageID:       uuid.NewString(),
			IdempotencyKey:  st.idempotencyKey,
			AggregateID:     op.ID,
			OperationID:     op.ID,
			StepID:          st.id,
			ExpectedVersion: st.version,
			TenantID:        op.TenantID,
			ProjectID:       op.ProjectID,
			EnvironmentID:   op.EnvironmentID,
			ActorID:         actorID,
			CorrelationID:   op.CorrelationID,
			Payload:         payload,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *PGStore) getByIdempotencyKey(ctx context.Context, tenantID, key string) (*Operation, error) {
	var id, planDigest string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(plan_digest, '') FROM operations WHERE tenant_id = $1 AND idempotency_key = $2`,
		tenantID, key,
	).Scan(&id, &planDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup idempotent operation: %w", err)
	}
	// plan_digest lives on the write-side operations table; the read model
	// projection does not carry it, and the idempotency-conflict check below
	// depends on it.
	op, err := s.GetOperation(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	op.PlanDigest = planDigest
	return op, nil
}

func lockOperation(ctx context.Context, tx *sql.Tx, id, tenantID string) (*Operation, error) {
	op := &Operation{}
	var tagsJSON []byte
	var startedAt, completedAt sql.NullTime
	var approvedBy, correlationID, planDigest, statusReason sql.NullString
	var projectID, environmentID, namespaceID, planID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, initiated_by, approved_by,
			correlation_id, idempotency_key, plan_digest, status_reason,
			total_steps, completed_steps, failed_steps, version, tags,
			created_at, started_at, completed_at
		FROM operations WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenantID,
	).Scan(
		&op.ID, &op.TenantID, &projectID, &environmentID, &namespaceID,
		&planID, &op.OperationType, &op.Status, &op.InitiatedBy, &approvedBy,
		&correlationID, &op.IdempotencyKey, &planDigest, &statusReason,
		&op.TotalSteps, &op.CompletedSteps, &op.FailedSteps, &op.Version, &tagsJSON,
		&op.CreatedAt, &startedAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock operation: %w", err)
	}
	op.ProjectID = projectID.String
	op.EnvironmentID = environmentID.String
	op.NamespaceID = namespaceID.String
	op.PlanID = planID.String
	op.ApprovedBy = approvedBy.String
	op.CorrelationID = correlationID.String
	op.PlanDigest = planDigest.String
	op.StatusReason = statusReason.String
	if startedAt.Valid {
		op.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		op.CompletedAt = &completedAt.Time
	}
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &op.Tags); err != nil {
			return nil, fmt.Errorf("decode operation tags: %w", err)
		}
	}
	return op, nil
}

// transitionOperation updates status with an optimistic guard on the state
// observed under the row lock and mutates op to the post-transition view.
func transitionOperation(ctx context.Context, tx *sql.Tx, op *Operation, to, actorID, reason string, setApprover bool) error {
	query := `
		UPDATE operations SET
			status = $1,
			status_reason = NULLIF($2, ''),
			version = version + 1,
			completed_at = CASE WHEN $1 IN ('succeeded', 'failed', 'cancelled') THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $3 AND status = $4`
	if setApprover {
		query = `
		UPDATE operations SET
			status = $1,
			status_reason = NULLIF($2, ''),
			approved_by = $5,
			version = version + 1,
			completed_at = CASE WHEN $1 IN ('succeeded', 'failed', 'cancelled') THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $3 AND status = $4`
	}
	var result sql.Result
	var err error
	if setApprover {
		result, err = tx.ExecContext(ctx, query, to, reason, op.ID, op.Status, actorID)
	} else {
		result, err = tx.ExecContext(ctx, query, to, reason, op.ID, op.Status)
	}
	if err != nil {
		return fmt.Errorf("transition operation to %s: %w", to, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect operation transition: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("%w: operation changed concurrently", ErrInvalidState)
	}
	if setApprover {
		op.ApprovedBy = actorID
	}
	op.Status = to
	op.StatusReason = reason
	op.Version++
	if to == StatusCancelled || to == StatusSucceeded || to == StatusFailed {
		now := time.Now().UTC()
		op.CompletedAt = &now
	}
	return nil
}

func insertAudit(ctx context.Context, tx *sql.Tx, operationID, eventType, actorID, previousState, newState string, detail map[string]any) error {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO operation_audit (
			operation_id, event_type, actor_id, previous_state, new_state, detail
		) VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)`,
		operationID, eventType, actorID, previousState, newState, string(detailJSON),
	)
	if err != nil {
		return fmt.Errorf("insert operation audit: %w", err)
	}
	return nil
}

// upsertReadModel mirrors the projection maintained by operation-worker so
// query responses always carry last_state_changed_at (KERNEL-003).
func upsertReadModel(ctx context.Context, tx *sql.Tx, op *Operation) error {
	summary := fmt.Sprintf("%s %s: %s", op.OperationType, op.ID, op.Status)
	tagsJSON, err := json.Marshal(op.Tags)
	if err != nil {
		return fmt.Errorf("marshal read model tags: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO operation_read_model (
			operation_id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, total_steps,
			completed_steps, failed_steps, initiated_by, approved_by,
			summary, target_cluster_ids, tags, intent_id, created_at, started_at, completed_at,
			last_state_changed_at
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			NULLIF($6, '')::uuid, $7, $8, $9, $10, $11, $12, NULLIF($13, ''),
			$14, $15, $16, NULLIF($17, '')::uuid, $18, $19, $20, now())
		ON CONFLICT (operation_id) DO UPDATE SET
			status = EXCLUDED.status,
			completed_steps = EXCLUDED.completed_steps,
			failed_steps = EXCLUDED.failed_steps,
			approved_by = EXCLUDED.approved_by,
			started_at = COALESCE(EXCLUDED.started_at, operation_read_model.started_at),
			completed_at = COALESCE(EXCLUDED.completed_at, operation_read_model.completed_at),
			intent_id = COALESCE(EXCLUDED.intent_id, operation_read_model.intent_id),
			last_state_changed_at = now()`,
		op.ID, op.TenantID, op.ProjectID, op.EnvironmentID, op.NamespaceID,
		op.PlanID, op.OperationType, op.Status, op.TotalSteps,
		op.CompletedSteps, op.FailedSteps, op.InitiatedBy, op.ApprovedBy,
		summary, pq.Array(op.TargetClusterIDs), string(tagsJSON), op.IntentID, op.CreatedAt, op.StartedAt, op.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert operation read model: %w", err)
	}
	return nil
}

type outboxEvent struct {
	MessageID       string
	IdempotencyKey  string
	AggregateID     string
	OperationID     string
	StepID          string
	ExpectedVersion int64
	TenantID        string
	ProjectID       string
	EnvironmentID   string
	ActorID         string
	CorrelationID   string
	Payload         any
}

// insertStepRequestedEvent writes one pending step-requested command for the
// outbox relay. platform-api never connects to NATS directly.
func insertStepRequestedEvent(ctx context.Context, tx *sql.Tx, op *Operation, stepID string, step StepInput, index int, actorID string, expectedVersion int64) error {
	planStepID := planStepID(step, index)
	key := StepIdempotencyKey(op.IdempotencyKey, planStepID)
	payload := map[string]any{
		"operationId":     op.ID,
		"stepId":          stepID,
		"stepType":        step.StepType,
		"idempotencyKey":  key,
		"expectedVersion": expectedVersion,
	}
	return insertOutboxEvent(ctx, tx, outboxEvent{
		MessageID:       uuid.NewString(),
		IdempotencyKey:  key,
		AggregateID:     op.ID,
		OperationID:     op.ID,
		StepID:          stepID,
		ExpectedVersion: expectedVersion,
		TenantID:        op.TenantID,
		ProjectID:       op.ProjectID,
		EnvironmentID:   op.EnvironmentID,
		ActorID:         actorID,
		CorrelationID:   op.CorrelationID,
		Payload:         payload,
	})
}

func insertOutboxEvent(ctx context.Context, tx *sql.Tx, event outboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, project_id, environment_id, actor_id,
			correlation_id, idempotency_key,
			aggregate_id, operation_id, step_id,
			expected_version, payload
		) VALUES (
			$1, $2, $3, $4, now(),
			$5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			$9, $10, NULLIF($11, ''), $12, $13, $14, $15
		)`,
		event.MessageID, StepRequestedSubject, SchemaVersion, StepRequestedSubject,
		event.TenantID, event.ProjectID, event.EnvironmentID, event.ActorID,
		event.CorrelationID, event.IdempotencyKey,
		event.AggregateID, event.OperationID, event.StepID,
		event.ExpectedVersion, string(payload),
	)
	if err != nil {
		return fmt.Errorf("insert step-requested outbox event: %w", err)
	}
	return nil
}

func planStepID(step StepInput, index int) string {
	if step.PlanStepID != "" {
		return step.PlanStepID
	}
	return fmt.Sprintf("step-%d", index+1)
}

func marshalStepInputs(step StepInput) ([]byte, error) {
	inputs := make(map[string]string, len(step.Inputs)+1)
	for k, v := range step.Inputs {
		inputs[k] = v
	}
	if step.SecretReference != "" {
		inputs["secretReference"] = step.SecretReference
	}
	data, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("marshal step inputs: %w", err)
	}
	return data, nil
}

// planDocument is the canonical structure the plan digest is computed over.
type planDocument struct {
	ReleaseID     string      `json:"release_id"`
	TenantID      string      `json:"tenant_id"`
	ProjectID     string      `json:"project_id"`
	EnvironmentID string      `json:"environment_id"`
	Steps         []StepInput `json:"steps"`
}

func buildPlan(cmd SubmitCommand) (digest string, planJSON []byte, err error) {
	steps := make([]StepInput, len(cmd.Steps))
	for i, step := range cmd.Steps {
		step.PlanStepID = planStepID(step, i)
		steps[i] = step
	}
	doc := planDocument{
		ReleaseID:     cmd.ReleaseID,
		TenantID:      cmd.TenantID,
		ProjectID:     cmd.ProjectID,
		EnvironmentID: cmd.EnvironmentID,
		Steps:         steps,
	}
	canonical, err := json.Marshal(doc)
	if err != nil {
		return "", nil, fmt.Errorf("marshal plan document: %w", err)
	}
	sum := sha256.Sum256(canonical)
	digest = fmt.Sprintf("%x", sum[:])

	planJSON, err = json.Marshal(map[string]any{
		"release_id":     cmd.ReleaseID,
		"tenant_id":      cmd.TenantID,
		"project_id":     cmd.ProjectID,
		"environment_id": cmd.EnvironmentID,
		"plan_digest":    digest,
		"steps":          steps,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", nil, fmt.Errorf("marshal execution plan: %w", err)
	}
	return digest, planJSON, nil
}

func isPgUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// ProvisionRuntimeTarget describes a new runtime-target row to be created
// atomically with an intent, for cluster create/import intents. The row is
// inserted with the plan-derived target ID so the console read model can show
// the cluster as PROVISIONING/REGISTERING before the lifecycle provider and
// observer report the live state.
type ProvisionRuntimeTarget struct {
	TargetID           string
	Name               string
	DisplayName        string
	TargetType         string // kubernetes | edge_runtime
	ConnectionType     string // agent | cloudhub
	ConnectionEndpoint string
	LifecycleState     string // PROVISIONING | REGISTERING
	Source             string // created | imported
}

// IntentSubmitCommand carries a fully validated RuntimeIntent and its server-planned ExecutionPlan.
type IntentSubmitCommand struct {
	Intent                *engine.RuntimeIntent
	ExecutionPlan         *engine.ExecutionPlan
	TenantID              string
	SubjectID             string
	CorrelationID         string
	PolicyVersion         string
	InitiatedBy           string
	MembershipID          string
	RuntimeTargetID       string
	ExpectedTargetVersion int64
	InitialStatus         string
	StalePolicyOutcome    string
	ConfirmationAccepted  bool
	ConfirmationReason    string
	CommitmentAction      string
	ServiceSubject        string
	DelegationTokenID     string
	DelegationKeyID       string
	TraceID               string
	AuthorizationScope    iam.DelegationScope
	ProvisionTarget       *ProvisionRuntimeTarget
}

// SubmitIntent atomically persists runtime_intent, execution_plan, operation,
// steps, audit evidence, read model, and outbox events in a single transaction
// (OP-007). Idempotency is scoped to (tenant + intentKind + idempotencyKey).
func (s *PGStore) SubmitIntent(ctx context.Context, cmd IntentSubmitCommand) (*Operation, bool, error) {
	correlationID := cmd.CorrelationID
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	intentDigest := cmd.Intent.ComputeIntentDigest()
	idemKey := cmd.Intent.Metadata.IdempotencyKey
	lookupKey := idempotencyKey(cmd.TenantID, string(cmd.Intent.Kind), cmd.CommitmentAction, idemKey)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin intent transaction: %w", err)
	}
	defer tx.Rollback()
	if commitment, lookupErr := getIntentCommitmentTx(ctx, tx, cmd.TenantID, string(cmd.Intent.Kind), cmd.CommitmentAction, idemKey); lookupErr == nil {
		if commitment.SemanticDigest != intentDigest {
			return nil, false, fmt.Errorf("%w: idempotency key %q", ErrIdempotencyConflict, lookupKey)
		}
		return &Operation{
			IntentID: commitment.IntentID, ID: commitment.OperationID, PlanID: commitment.ExecutionPlanID,
			TenantID: cmd.TenantID, OperationType: intentKindToOperationType(cmd.Intent.Kind), Status: commitment.AcceptedStatus,
			CorrelationID: commitment.CorrelationID, CreatedAt: commitment.CreatedAt,
		}, false, nil
	} else if !errors.Is(lookupErr, ErrNotFound) {
		return nil, false, lookupErr
	}
	if cmd.RuntimeTargetID != "" {
		var currentVersion int64
		err := tx.QueryRowContext(ctx, `
			SELECT projection_version FROM runtime_targets
			WHERE id = $1 AND is_active = true AND (tenant_id = $2 OR EXISTS (
				SELECT 1 FROM tenant_cluster_allocations tca
				WHERE tca.cluster_id=runtime_targets.id AND tca.tenant_id=$2 AND tca.status='active'))
			FOR SHARE`, cmd.RuntimeTargetID, cmd.TenantID).Scan(&currentVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, ErrTargetNotFound
		}
		if err != nil {
			return nil, false, fmt.Errorf("recheck runtime target: %w", err)
		}
		if currentVersion != cmd.ExpectedTargetVersion {
			return nil, false, ErrTargetVersionConflict
		}
	}
	if cmd.Intent.Kind == engine.IntentReconcileStorageClassBinding {
		if err := recheckStorageReconcile(ctx, tx, cmd); err != nil {
			return nil, false, err
		}
	}

	planJSON, err := json.Marshal(cmd.ExecutionPlan)
	if err != nil {
		return nil, false, fmt.Errorf("marshal execution plan JSON: %w", err)
	}

	var planID string
	planDigestHex := scopedPlanDigest(cmd.ExecutionPlan.SemanticDigest, lookupKey)
	// runtime_intent_id is bound at plan insert time: migration 055 makes
	// execution_plans rows immutable after creation, so the intent link must
	// not be written by a later UPDATE.
	intentID := uuid.New().String()
	err = tx.QueryRowContext(ctx, `
		INSERT INTO execution_plans (
			release_id, tenant_id, project_id, environment_id,
			plan_digest, plan_json, policy_result, status,
			runtime_intent_id
		) VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, '{}', 'active', $7)
		ON CONFLICT (tenant_id, plan_digest) DO NOTHING
		RETURNING id`,
		cmd.ExecutionPlan.ReleaseRef, cmd.TenantID, "", "",
		planDigestHex, string(planJSON), intentID,
	).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM execution_plans WHERE tenant_id = $1 AND plan_digest = $2`, cmd.TenantID, planDigestHex,
		).Scan(&planID)
	}
	if err != nil {
		return nil, false, fmt.Errorf("persist execution plan: %w", err)
	}

	opID := uuid.New().String()

	operationType := intentKindToOperationType(cmd.Intent.Kind)
	initialStatus := InitialStatus(operationType)
	if cmd.InitialStatus != "" {
		initialStatus = cmd.InitialStatus
	}
	tagsJSON, _ := json.Marshal(map[string]string{
		"intent_kind": string(cmd.Intent.Kind),
		"correlation": correlationID,
		"target_id":   cmd.RuntimeTargetID,
		"target_kind": cmd.Intent.Spec.TargetKind,
	})

	var existingOpID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, plan_digest, status_reason,
			total_steps, tags
		) VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''),
			$6, $7, $8, $9, $10, $11, $12, '', $13, $14)
		ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
		RETURNING id`,
		opID, cmd.TenantID, "", "", "",
		planID, operationType, initialStatus, cmd.InitiatedBy,
		correlationID, lookupKey, planDigestHex,
		len(cmd.ExecutionPlan.Steps), string(tagsJSON),
	).Scan(&existingOpID)
	if errors.Is(err, sql.ErrNoRows) {
		commitment, getErr := getIntentCommitmentTx(ctx, tx, cmd.TenantID, string(cmd.Intent.Kind), cmd.CommitmentAction, idemKey)
		if getErr != nil {
			return nil, false, getErr
		}
		if commitment.SemanticDigest != intentDigest {
			return nil, false, fmt.Errorf("%w: idempotency key %q", ErrIdempotencyConflict, lookupKey)
		}
		return &Operation{
			IntentID: commitment.IntentID, ID: commitment.OperationID, PlanID: commitment.ExecutionPlanID,
			TenantID: cmd.TenantID, OperationType: operationType, Status: commitment.AcceptedStatus,
			CorrelationID: commitment.CorrelationID, CreatedAt: commitment.CreatedAt,
		}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("insert operation: %w", err)
	}

	stepIDs := make([]string, len(cmd.ExecutionPlan.Steps))
	for i, step := range cmd.ExecutionPlan.Steps {
		stepID := uuid.New().String()
		stepIDs[i] = stepID
		inputsJSON, err := json.Marshal(step.Inputs)
		if err != nil {
			return nil, false, fmt.Errorf("marshal step inputs %q: %w", step.StepID, err)
		}
		if inputsJSON == nil {
			inputsJSON = []byte("{}")
		}
		secretRefsJSON, err := json.Marshal(step.SecretReferences)
		if err != nil {
			return nil, false, fmt.Errorf("marshal step secret references %q: %w", step.StepID, err)
		}
		if secretRefsJSON == nil {
			secretRefsJSON = []byte("[]")
		}
		compensationJSON, err := json.Marshal(step.Compensation)
		if err != nil {
			return nil, false, fmt.Errorf("marshal step compensation %q: %w", step.StepID, err)
		}
		if compensationJSON == nil {
			compensationJSON = []byte("{}")
		}
		maxAttempts := 3
		if step.RetryPolicy != nil && step.RetryPolicy.MaxAttempts > 0 {
			maxAttempts = step.RetryPolicy.MaxAttempts
		}
		timeoutSeconds := 300
		if step.TimeoutSeconds > 0 {
			timeoutSeconds = step.TimeoutSeconds
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO operation_steps (
				id, operation_id, plan_step_id, step_name, step_type, provider_id,
				provider_version, provider_digest, provider_protocol_version,
				input_schema, secret_references, compensation,
				target_ref, target_kind,
				status, idempotency_key, depends_on, optional,
				max_retries, timeout_seconds, step_input
			) VALUES ($1, $2, $3, $4, $5, NULLIF($6,''),
				NULLIF($7,''), NULLIF($8,''), NULLIF($9,''),
				NULLIF($10,''), $11, $12,
				NULLIF($13,''), NULLIF($14,''),
				'pending', $15, $16, false,
				$17, $18, $19)`,
			stepID, opID, step.StepID, step.StepID, step.StepType, step.ProviderID,
			step.ProviderVersion, step.ProviderDigest, step.ProviderProtocolVersion,
			step.InputSchema, string(secretRefsJSON), string(compensationJSON),
			step.TargetRef, step.TargetKind,
			intentIdemKey(lookupKey, step.StepID), pq.Array(step.DependsOn),
			maxAttempts, timeoutSeconds, string(inputsJSON),
		)
		if err != nil {
			return nil, false, fmt.Errorf("insert operation step %q: %w", step.StepID, err)
		}
	}

	if cmd.ProvisionTarget != nil {
		if err := s.provisionRuntimeTargetTx(ctx, tx, cmd, intentID); err != nil {
			return nil, false, err
		}
	}

	if err := insertAudit(ctx, tx, opID, "created", cmd.InitiatedBy, "", initialStatus, map[string]any{
		"operation_type": operationType,
		"release_id":     cmd.ExecutionPlan.ReleaseRef,
		"plan_digest":    planDigestHex,
		"intent_kind":    string(cmd.Intent.Kind),
	}); err != nil {
		return nil, false, err
	}

	if err := insertSecurityAudit(ctx, tx, cmd, opID, planID, intentDigest); err != nil {
		return nil, false, err
	}

	op := &Operation{
		IntentID: intentID, ID: opID, TenantID: cmd.TenantID,
		PlanID: planID, OperationType: operationType, Status: initialStatus,
		InitiatedBy: cmd.InitiatedBy, CorrelationID: correlationID,
		IdempotencyKey: lookupKey, PlanDigest: planDigestHex,
		TotalSteps: len(cmd.ExecutionPlan.Steps), Tags: map[string]string{"intent_kind": string(cmd.Intent.Kind)},
		CreatedAt: time.Now().UTC(),
	}
	if err := upsertReadModel(ctx, tx, op); err != nil {
		return nil, false, err
	}

	if err := insertRuntimeIntent(ctx, tx, cmd, intentID, opID, planID, intentDigest, initialStatus); err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET runtime_intent_id = $1 WHERE id = $2`, intentID, opID); err != nil {
		return nil, false, err
	}

	if initialStatus == StatusQueued {
		for i, step := range cmd.ExecutionPlan.Steps {
			if len(step.DependsOn) > 0 {
				continue
			}
			payload := map[string]any{
				"operationId":     opID,
				"stepId":          stepIDs[i],
				"stepType":        step.StepType,
				"idempotencyKey":  intentIdemKey(lookupKey, step.StepID),
				"expectedVersion": 0,
				"correlationId":   correlationID,
			}
			if err := insertOutboxEvent(ctx, tx, outboxEvent{
				MessageID:      uuid.New().String(),
				IdempotencyKey: intentIdemKey(lookupKey, step.StepID),
				AggregateID:    opID,
				OperationID:    opID,
				StepID:         stepIDs[i],
				TenantID:       cmd.TenantID,
				ActorID:        cmd.InitiatedBy,
				CorrelationID:  correlationID,
				Payload:        payload,
			}); err != nil {
				return nil, false, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit intent transaction: %w", err)
	}

	final, err := s.GetOperation(ctx, opID, cmd.TenantID)
	if err != nil {
		return nil, false, err
	}
	final.IntentID = intentID
	final.Status = initialStatus
	final.CorrelationID = correlationID
	return final, true, nil
}

// provisionRuntimeTargetTx creates the runtime_targets row for a cluster
// create/import intent, atomically with the intent transaction. It follows the
// same default-workspace pattern as CreateRuntimeTarget and is idempotent on
// the (deterministic) target ID.
func (s *PGStore) provisionRuntimeTargetTx(ctx context.Context, tx *sql.Tx, cmd IntentSubmitCommand, intentID string) error {
	pt := cmd.ProvisionTarget
	if pt == nil || pt.TargetID == "" {
		return nil
	}
	source := pt.Source
	if source != "created" && source != "imported" {
		source = "imported"
	}
	labelsJSON, _ := json.Marshal(map[string]any{
		"hnb.source":  source,
		"intent_kind": string(cmd.Intent.Kind),
		"intent_id":   intentID,
		"correlation": cmd.CorrelationID,
	})
	name := pt.Name
	if name == "" {
		name = pt.TargetID
	}
	// credential_ref: nil parameter → SQL NULL for targets created without a
	// credential binding; a non-nil JSON literal otherwise.
	var credentialRefParam interface{}
	if ref := cmd.Intent.Spec.CredentialSecretRef; ref != nil && strings.TrimSpace(ref.Name) != "" {
		if b, err := json.Marshal(ref); err == nil {
			credentialRefParam = string(b)
		}
	}
	_, err := tx.ExecContext(ctx, `
		WITH selected_workspace AS (
			INSERT INTO workspaces (tenant_id, name, display_name)
			VALUES ($2, 'default', 'Default')
			ON CONFLICT (tenant_id, name) DO UPDATE
				SET updated_at = workspaces.updated_at
			RETURNING id
		)
		INSERT INTO runtime_targets (
			id, tenant_id, name, display_name, target_type,
			connection_type, connection_endpoint,
			status, labels, stale_threshold_seconds, is_active,
			lifecycle_state, health_state, connectivity_state, freshness_state,
			credential_ref, projection_version, workspace_id
		) SELECT
			$1, $2, $3, NULLIF($4, ''), $5,
			$6, NULLIF($7, ''),
			'unknown', $8::jsonb, 300, true,
			$9, 'UNKNOWN', 'UNKNOWN', 'UNKNOWN',
			$10::jsonb, 0, id
		FROM selected_workspace
		ON CONFLICT (id) DO NOTHING`,
		pt.TargetID, cmd.TenantID, name, pt.DisplayName, pt.TargetType,
		pt.ConnectionType, pt.ConnectionEndpoint, string(labelsJSON), pt.LifecycleState,
		credentialRefParam,
	)
	if err != nil {
		return fmt.Errorf("provision runtime target: %w", err)
	}
	return nil
}

func recheckStorageReconcile(ctx context.Context, tx *sql.Tx, cmd IntentSubmitCommand) error {
	var version, offeringVersion int64
	var offeringID, targetID, storageClassName, uid, resourceVersion, freshness string
	err := tx.QueryRowContext(ctx, `
		SELECT version, offering_id::text, offering_version, target_id::text, storage_class_name,
		       storage_class_uid, storage_class_resource_version, freshness
		FROM storage_class_bindings
		WHERE tenant_id=$1 AND id=$2
		FOR SHARE`, cmd.TenantID, cmd.Intent.Spec.BindingID).Scan(
		&version, &offeringID, &offeringVersion, &targetID, &storageClassName, &uid, &resourceVersion, &freshness)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStorageBindingConflict
	}
	if err != nil {
		return fmt.Errorf("recheck storage binding: %w", err)
	}
	if version != cmd.Intent.Spec.BindingVersion || offeringID != cmd.Intent.Spec.OfferingID ||
		offeringVersion != cmd.Intent.Spec.OfferingVersion || targetID != cmd.Intent.Spec.TargetID ||
		storageClassName != cmd.Intent.Spec.StorageClassName || uid != cmd.Intent.Spec.StorageClassUID ||
		resourceVersion != cmd.Intent.Spec.StorageClassResourceVersion {
		return ErrStorageBindingConflict
	}
	if freshness != "Fresh" {
		return ErrStorageObservationConflict
	}
	return nil
}

func intentKindToOperationType(kind engine.IntentKind) string {
	switch kind {
	case engine.IntentInstallRelease:
		return "deploy"
	case engine.IntentUninstallRelease:
		return "delete"
	case engine.IntentUpgradeRelease:
		return "upgrade"
	case engine.IntentRollbackRelease:
		return "rollback"
	case engine.IntentChangeConfiguration:
		return "config_change"
	case engine.IntentCreateKubernetesTarget, engine.IntentImportRuntimeTarget:
		return "deploy"
	case engine.IntentUpgradeRuntimeTarget:
		return "upgrade"
	case engine.IntentDeleteRuntimeTarget:
		return "delete"
	case engine.IntentImportStorageClassBinding, engine.IntentReconcileStorageClassBinding:
		return "config_change"
	case engine.IntentInstallStorageDriver:
		return "deploy"
	case engine.IntentUpgradeStorageDriver:
		return "upgrade"
	case engine.IntentUninstallStorageDriver:
		return "delete"
	case engine.IntentReleaseRetainedVolume, engine.IntentSanitizeRetainedVolume:
		return "storage_reclaim"
	default:
		return "deploy"
	}
}

func intentIdemKey(base, stepID string) string {
	return base + ":" + stepID
}

func insertRuntimeIntent(ctx context.Context, tx *sql.Tx, cmd IntentSubmitCommand, intentID, opID, planID, digest, acceptedStatus string) error {
	intentDoc, err := json.Marshal(map[string]any{
		"apiVersion": cmd.Intent.APIVersion,
		"kind":       string(cmd.Intent.Kind),
		"metadata":   cmd.Intent.Metadata,
		"spec": map[string]any{
			"releaseId":                   cmd.Intent.Spec.ReleaseID,
			"targetRef":                   cmd.Intent.Spec.TargetRef,
			"scopeRef":                    cmd.Intent.Spec.ScopeRef,
			"parameters":                  cmd.Intent.Spec.Parameters,
			"secretReferences":            cmd.Intent.Spec.SecretReferences,
			"targetId":                    cmd.Intent.Spec.TargetID,
			"targetKind":                  cmd.Intent.Spec.TargetKind,
			"expectedVersion":             cmd.Intent.Spec.ExpectedVersion,
			"bindingId":                   cmd.Intent.Spec.BindingID,
			"bindingVersion":              cmd.Intent.Spec.BindingVersion,
			"offeringId":                  cmd.Intent.Spec.OfferingID,
			"offeringVersion":             cmd.Intent.Spec.OfferingVersion,
			"storageClassName":            cmd.Intent.Spec.StorageClassName,
			"storageClassUid":             cmd.Intent.Spec.StorageClassUID,
			"storageClassResourceVersion": cmd.Intent.Spec.StorageClassResourceVersion,
			"installationId":              cmd.Intent.Spec.InstallationID,
			"packageId":                   cmd.Intent.Spec.PackageID,
			"packageVersion":              cmd.Intent.Spec.PackageVersion,
			"currentVersion":              cmd.Intent.Spec.CurrentVersion,
			"desiredVersion":              cmd.Intent.Spec.DesiredVersion,
			"displayName":                 cmd.Intent.Spec.DisplayName,
			"kubernetesVersion":           cmd.Intent.Spec.KubernetesVersion,
			"cloudCoreEndpoint":           cmd.Intent.Spec.CloudCoreEndpoint,
			"credentialSecretRef":         cmd.Intent.Spec.CredentialSecretRef,
			"nodeGroupMappings":           cmd.Intent.Spec.NodeGroupMappings,
			"riskConfirmation":            cmd.Intent.Spec.RiskConfirmation,
			"volumeId":                    cmd.Intent.Spec.VolumeID,
			"workflowProviderRef":         cmd.Intent.Spec.WorkflowProviderRef,
			"persistentVolume":            cmd.Intent.Spec.PersistentVolume,
			"persistentVolumeClaim":       cmd.Intent.Spec.PersistentVolumeClaim,
			"podDependencies":             cmd.Intent.Spec.PodDependencies,
			"statefulSetDependencies":     cmd.Intent.Spec.StatefulSetDependencies,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal intent document: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO runtime_intents (
			id, tenant_id, subject_id, intent_kind, api_version,
			idempotency_key, semantic_digest, intent_document,
			execution_plan_id, operation_id, policy_version_id,
			correlation_id, runtime_target_id, commitment_action, accepted_status, response_http_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid,$12,NULLIF($13,'')::uuid,$14,$15,202)`,
		intentID, cmd.TenantID, cmd.SubjectID, string(cmd.Intent.Kind),
		cmd.Intent.APIVersion, cmd.Intent.Metadata.IdempotencyKey,
		digest, string(intentDoc),
		planID, opID, optionalUUID(cmd.PolicyVersion), cmd.CorrelationID, cmd.RuntimeTargetID,
		cmd.CommitmentAction, acceptedStatus,
	)
	return err
}

func insertSecurityAudit(ctx context.Context, tx *sql.Tx, cmd IntentSubmitCommand, opID, planID, intentDigest string) error {
	scopeJSON, err := json.Marshal(cmd.AuthorizationScope)
	if err != nil {
		return fmt.Errorf("marshal authorization scope: %w", err)
	}
	detail := map[string]any{
		"intent_kind":           string(cmd.Intent.Kind),
		"intent_hash":           intentDigest,
		"release_id":            cmd.Intent.Spec.ReleaseID,
		"target_ref":            cmd.Intent.Spec.TargetRef,
		"scope_ref":             cmd.Intent.Spec.ScopeRef,
		"plan_id":               planID,
		"operation_id":          opID,
		"decision":              "allow",
		"stale_policy_outcome":  cmd.StalePolicyOutcome,
		"confirmation_accepted": cmd.ConfirmationAccepted,
		"confirmation_reason":   cmd.ConfirmationReason,
		"service_subject":       cmd.ServiceSubject,
		"actor_membership_id":   cmd.MembershipID,
		"delegation_token_id":   cmd.DelegationTokenID,
		"delegation_key_id":     cmd.DelegationKeyID,
		"policy_version":        cmd.PolicyVersion,
	}
	decisionJSON, _ := json.Marshal(detail)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO security_audit_events (
			tenant_id, subject_id, event_type, decision, reason_code,
			action, resource_kind, resource_id, scope,
			correlation_id, trace_id, outcome, detail, execution_plan_id, operation_id,
			service_subject, actor_membership_id, delegation_token_id, delegation_key_id
		) VALUES ($1, $2, 'intent_received', 'allow', '',
			$3, 'cluster', NULLIF($4,''), $5,
			$6, $7, 'accepted', $8, $9, $10, $11, NULLIF($12,''), $13, $14)`,
		cmd.TenantID, cmd.SubjectID, cmd.CommitmentAction, cmd.RuntimeTargetID, string(scopeJSON),
		cmd.CorrelationID, cmd.TraceID, string(decisionJSON), planID, opID, cmd.ServiceSubject,
		cmd.MembershipID, cmd.DelegationTokenID, cmd.DelegationKeyID,
	)
	if err != nil {
		return fmt.Errorf("insert security audit event: %w", err)
	}
	return nil
}

func idempotencyKey(tenant, kind, action, key string) string {
	return fmt.Sprintf("%s:%s:%s:%s", tenant, kind, action, key)
}

func scopedPlanDigest(planDigest, lookupKey string) string {
	digest := sha256.Sum256([]byte(planDigest + "\x00" + lookupKey))
	return fmt.Sprintf("sha256:%x", digest)
}

func optionalUUID(value string) string {
	if _, err := uuid.Parse(value); err != nil {
		return ""
	}
	return value
}

func getIntentCommitmentTx(ctx context.Context, tx *sql.Tx, tenantID, kind, action, idempotencyKey string) (*IntentCommitment, error) {
	return scanIntentCommitment(tx.QueryRowContext(ctx, `
		SELECT id, execution_plan_id, operation_id, semantic_digest, intent_kind,
		       commitment_action, accepted_status, correlation_id, created_at, response_http_status
		FROM runtime_intents
		WHERE tenant_id=$1 AND intent_kind=$2 AND commitment_action=$3 AND idempotency_key=$4`,
		tenantID, kind, action, idempotencyKey))
}

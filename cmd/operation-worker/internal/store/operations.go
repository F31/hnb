package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

var (
	ErrLeaseNotAcquired     = errors.New("step lease is held by another worker")
	ErrLeaseLost            = errors.New("step lease is no longer valid")
	ErrStepAlreadyCompleted = errors.New("step is already completed")
	ErrStepVersionMismatch  = errors.New("step version does not match command")
	ErrOperationNotRunnable = errors.New("operation is not runnable")
)

type StepState struct {
	Status          engine.StepStatus
	IdempotencyKey  string
	Version         int64
	StepType        string
	ProviderID      string
	ProviderVersion string
	ProviderDigest  string
	Inputs          map[string]any
	Checkpoint      string
	DependsOn       []string
	RetryCount      int
	MaxRetries      int
	TimeoutSeconds  int
}

type Lease struct {
	ID         string
	Generation int64
}

type OutboxEvent struct {
	MessageID        string
	MessageType      string
	SchemaVersion    string
	Subject          string
	OccurredAt       time.Time
	TenantID         string
	ProjectID        string
	EnvironmentID    string
	ActorID          string
	CorrelationID    string
	CausationID      string
	IdempotencyKey   string
	AggregateID      string
	AggregateVersion int64
	OperationID      string
	StepID           string
	ExpectedVersion  int64
	Payload          any
}

type OperationStore struct {
	db *sql.DB
}

func NewOperationStore(db *sql.DB) *OperationStore {
	return &OperationStore{db: db}
}

func (s *OperationStore) CreateOperation(op *engine.Operation, planJSON string) error {
	_, err := s.db.Exec(`
		INSERT INTO operations (
			id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, initiated_by, approved_by,
			correlation_id, idempotency_key, plan_digest, status_reason,
			total_steps, completed_steps, failed_steps, tags
		) VALUES (
			$1, $2, $3, $4, $5,
			NULLIF($6, '')::uuid, $7, $8, $9, NULLIF($10, ''),
			$11, $12, $13, $14,
			$15, $16, $17, $18
		)`,
		op.ID, op.TenantID, op.ProjectID, op.EnvironmentID, op.NamespaceID,
		op.PlanID, string(op.Type), string(op.Status), op.InitiatedBy, op.ApprovedBy,
		op.CorrelationID, op.IdempotencyKey, op.PlanDigest, "",
		op.TotalSteps, op.CompletedSteps, op.FailedSteps, string(toJSON(op.Tags)),
	)
	return err
}

func (s *OperationStore) GetOperation(id string) (*engine.Operation, error) {
	return s.GetOperationContext(context.Background(), id)
}

func (s *OperationStore) GetOperationContext(ctx context.Context, id string) (*engine.Operation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, COALESCE(project_id, ''), COALESCE(environment_id, ''),
			COALESCE(namespace_id, ''), COALESCE(plan_id::text, ''), operation_type, status,
			initiated_by, COALESCE(approved_by, ''), COALESCE(correlation_id, ''),
			idempotency_key, COALESCE(plan_digest, ''),
			total_steps, completed_steps, failed_steps, version, target_cluster_ids, tags,
			created_at, started_at, completed_at
		FROM operations WHERE id = $1`, id)

	op := &engine.Operation{}
	var tagsJSON []byte
	var startedAt, completedAt sql.NullTime

	err := row.Scan(
		&op.ID, &op.TenantID, &op.ProjectID, &op.EnvironmentID, &op.NamespaceID,
		&op.PlanID, &op.Type, &op.Status, &op.InitiatedBy, &op.ApprovedBy,
		&op.CorrelationID, &op.IdempotencyKey, &op.PlanDigest,
		&op.TotalSteps, &op.CompletedSteps, &op.FailedSteps, &op.Version, pq.Array(&op.TargetClusterIDs), &tagsJSON,
		&op.CreatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		op.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		op.CompletedAt = &completedAt.Time
	}
	if err := json.Unmarshal(tagsJSON, &op.Tags); err != nil {
		return nil, fmt.Errorf("decode operation tags: %w", err)
	}

	return op, nil
}

func (s *OperationStore) GetStepState(ctx context.Context, operationID, stepID string) (StepState, error) {
	var state StepState
	var inputsJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT status, idempotency_key, version, step_type,
			COALESCE(provider_id, ''), COALESCE(provider_version, ''),
			COALESCE(provider_digest, ''), step_input,
			COALESCE(checkpoint, ''), depends_on,
			retry_count, max_retries, timeout_seconds
		FROM operation_steps
		WHERE id = $1 AND operation_id = $2`, stepID, operationID,
	).Scan(
		&state.Status, &state.IdempotencyKey, &state.Version, &state.StepType,
		&state.ProviderID, &state.ProviderVersion, &state.ProviderDigest, &inputsJSON,
		&state.Checkpoint, pq.Array(&state.DependsOn),
		&state.RetryCount, &state.MaxRetries, &state.TimeoutSeconds,
	)
	if err != nil {
		return state, err
	}
	if len(inputsJSON) > 0 {
		if err := json.Unmarshal(inputsJSON, &state.Inputs); err != nil {
			return state, fmt.Errorf("decode step inputs: %w", err)
		}
	}
	return state, err
}

func (s *OperationStore) SaveStepRetry(
	ctx context.Context,
	operationID, stepID, ownerID string,
	lease Lease,
	expectedVersion int64,
	result engine.StepResult,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin step retry transaction: %w", err)
	}
	defer tx.Rollback()
	outputs, err := json.Marshal(result.Outputs)
	if err != nil {
		return fmt.Errorf("marshal retry output: %w", err)
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE operation_steps AS step SET
			status = 'pending', step_output = $1, checkpoint = $2,
			error_message = $3, retry_count = retry_count + 1,
			started_at = COALESCE(started_at, $4),
			updated_at = now()
		WHERE step.id = $5 AND step.operation_id = $6 AND step.version = $7
			AND step.status IN ('pending', 'running')
			AND step.last_lease_id = $8 AND step.fencing_generation = $9
			AND EXISTS (
				SELECT 1 FROM worker_leases AS lease
				WHERE lease.step_id = step.id AND lease.owner_id = $10
					AND lease.lease_id = $8
					AND lease.fencing_generation = $9
					AND lease.expires_at > now()
			)`,
		string(outputs), result.Checkpoint, result.ErrorMessage, result.StartedAt,
		stepID, operationID, expectedVersion, lease.ID, lease.Generation, ownerID,
	)
	if err != nil {
		return fmt.Errorf("save fenced step retry: %w", err)
	}
	rows, err := update.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect step retry update: %w", err)
	}
	if rows != 1 {
		return ErrLeaseLost
	}
	detail, err := json.Marshal(map[string]any{
		"step_id":            stepID,
		"retryable":          true,
		"error":              result.ErrorMessage,
		"lease_id":           lease.ID,
		"fencing_generation": lease.Generation,
	})
	if err != nil {
		return fmt.Errorf("marshal retry audit detail: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operation_audit (operation_id, event_type, actor_id, detail)
		VALUES ($1, 'step_failed', $2, $3)`,
		operationID, ownerID, string(detail),
	); err != nil {
		return fmt.Errorf("insert step retry audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM worker_leases
		WHERE step_id = $1 AND owner_id = $2 AND lease_id = $3
			AND fencing_generation = $4`,
		stepID, ownerID, lease.ID, lease.Generation,
	); err != nil {
		return fmt.Errorf("release retry lease: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit step retry: %w", err)
	}
	return nil
}

func (s *OperationStore) DependenciesSatisfied(
	ctx context.Context,
	operationID string,
	dependsOn []string,
) (bool, error) {
	if len(dependsOn) == 0 {
		return true, nil
	}
	var satisfied bool
	err := s.db.QueryRowContext(ctx, `
		SELECT NOT EXISTS (
			SELECT 1 FROM unnest($2::text[]) AS dependency(step_id)
			WHERE NOT EXISTS (
				SELECT 1 FROM operation_steps AS prerequisite
				WHERE prerequisite.operation_id = $1
					AND (prerequisite.plan_step_id = dependency.step_id
						OR prerequisite.id::text = dependency.step_id)
					AND prerequisite.status = 'succeeded'
			)
		)`, operationID, pq.Array(dependsOn),
	).Scan(&satisfied)
	if err != nil {
		return false, fmt.Errorf("check step dependencies: %w", err)
	}
	return satisfied, nil
}

func (s *OperationStore) AcquireLease(
	ctx context.Context,
	operationID, stepID, ownerID string,
	expectedVersion int64,
	duration time.Duration,
) (Lease, error) {
	var lease Lease
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return lease, fmt.Errorf("begin lease acquisition transaction: %w", err)
	}
	defer tx.Rollback()

	op, err := getOperationForUpdate(ctx, tx, operationID)
	if err != nil {
		return lease, err
	}
	if op.Status != engine.StatusQueued && op.Status != engine.StatusInProgress {
		return lease, ErrOperationNotRunnable
	}

	var status engine.StepStatus
	var version, retainedGeneration int64
	err = tx.QueryRowContext(ctx, `
		SELECT status, version, fencing_generation
		FROM operation_steps
		WHERE id = $1 AND operation_id = $2
		FOR UPDATE`, stepID, operationID,
	).Scan(&status, &version, &retainedGeneration)
	if err != nil {
		return lease, fmt.Errorf("lock step for lease acquisition: %w", err)
	}
	if status != engine.StepPending && status != engine.StepRunning {
		return lease, ErrStepAlreadyCompleted
	}
	if version != expectedVersion {
		return lease, ErrStepVersionMismatch
	}

	if err := tx.QueryRowContext(ctx,
		`SELECT nextval('operation_fencing_generation_seq')`,
	).Scan(&lease.Generation); err != nil {
		return lease, fmt.Errorf("allocate fencing generation: %w", err)
	}
	if lease.Generation <= retainedGeneration {
		return lease, fmt.Errorf("allocated fencing generation %d does not exceed retained generation %d", lease.Generation, retainedGeneration)
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO worker_leases (step_id, owner_id, lease_id, fencing_generation, expires_at)
		VALUES ($1, $2, gen_random_uuid(), $3, now() + ($4 * interval '1 microsecond'))
		ON CONFLICT (step_id) DO UPDATE SET
			owner_id = EXCLUDED.owner_id,
			lease_id = EXCLUDED.lease_id,
			fencing_generation = EXCLUDED.fencing_generation,
			acquired_at = now(),
			expires_at = EXCLUDED.expires_at,
			version = worker_leases.version + 1,
			updated_at = now()
		WHERE worker_leases.expires_at <= now()
		RETURNING lease_id::text`,
		stepID, ownerID, lease.Generation, duration.Microseconds(),
	).Scan(&lease.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrLeaseNotAcquired
	}
	if err != nil {
		return Lease{}, fmt.Errorf("acquire step lease: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operation_steps SET
			last_lease_id = $1, fencing_generation = $2, updated_at = now()
		WHERE id = $3 AND operation_id = $4`,
		lease.ID, lease.Generation, stepID, operationID,
	); err != nil {
		return Lease{}, fmt.Errorf("retain step fencing generation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Lease{}, fmt.Errorf("commit lease acquisition: %w", err)
	}
	return lease, nil
}

func (s *OperationStore) RenewLease(
	ctx context.Context,
	stepID, ownerID string,
	lease Lease,
	duration time.Duration,
) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE worker_leases AS lease SET
			expires_at = now() + ($4 * interval '1 microsecond'),
			version = version + 1,
			updated_at = now()
		WHERE lease.step_id = $1 AND lease.owner_id = $2 AND lease.lease_id = $3
			AND lease.fencing_generation = $5 AND lease.expires_at > now()
			AND EXISTS (
				SELECT 1 FROM operation_steps AS step
				WHERE step.id = lease.step_id
					AND step.last_lease_id = lease.lease_id
					AND step.fencing_generation = lease.fencing_generation
			)`,
		stepID, ownerID, lease.ID, duration.Microseconds(), lease.Generation,
	)
	if err != nil {
		return fmt.Errorf("renew step lease: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect lease renewal: %w", err)
	}
	if rows != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *OperationStore) CommitStepSuccess(
	ctx context.Context,
	operationID, stepID, idempotencyKey, ownerID string,
	lease Lease,
	expectedVersion int64,
	result engine.StepResult,
	event OutboxEvent,
) error {
	result.Status = engine.StepSucceeded
	return s.commitStepResult(
		ctx, operationID, stepID, idempotencyKey, ownerID, lease,
		expectedVersion, result, event, nil,
	)
}

func (s *OperationStore) CommitStepFailure(
	ctx context.Context,
	operationID, stepID, idempotencyKey, ownerID string,
	lease Lease,
	expectedVersion int64,
	result engine.StepResult,
	event, failedEvent OutboxEvent,
) error {
	result.Status = engine.StepFailed
	return s.commitStepResult(
		ctx, operationID, stepID, idempotencyKey, ownerID, lease,
		expectedVersion, result, event, []OutboxEvent{failedEvent},
	)
}

func (s *OperationStore) commitStepResult(
	ctx context.Context,
	operationID, stepID, idempotencyKey, ownerID string,
	lease Lease,
	expectedVersion int64,
	result engine.StepResult,
	event OutboxEvent,
	additionalEvents []OutboxEvent,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin step transaction: %w", err)
	}
	defer tx.Rollback()

	op, err := getOperationForUpdate(ctx, tx, operationID)
	if err != nil {
		return err
	}
	if engine.IsTerminal(op.Status) || (op.Status != engine.StatusQueued && op.Status != engine.StatusInProgress) {
		return ErrOperationNotRunnable
	}

	var step StepState
	err = tx.QueryRowContext(ctx, `
		SELECT status, idempotency_key, version
		FROM operation_steps
		WHERE id = $1 AND operation_id = $2
		FOR UPDATE`, stepID, operationID,
	).Scan(&step.Status, &step.IdempotencyKey, &step.Version)
	if err != nil {
		return fmt.Errorf("lock step: %w", err)
	}
	if step.Status == engine.StepSucceeded || step.Status == engine.StepFailed {
		return ErrStepAlreadyCompleted
	}
	if step.IdempotencyKey != idempotencyKey || step.Version != expectedVersion {
		return ErrStepVersionMismatch
	}

	outputsJSON, err := json.Marshal(result.Outputs)
	if err != nil {
		return fmt.Errorf("marshal step output: %w", err)
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE operation_steps AS step SET
			status = $1, step_output = $2, checkpoint = $3,
			error_message = NULLIF($4, ''), started_at = COALESCE(started_at, $5),
			completed_at = $6, version = version + 1,
			updated_at = now()
		WHERE step.id = $7 AND step.operation_id = $8
			AND step.status IN ('pending', 'running')
			AND step.last_lease_id = $9 AND step.fencing_generation = $10
			AND EXISTS (
				SELECT 1 FROM worker_leases AS lease
				WHERE lease.step_id = step.id
					AND lease.owner_id = $11
					AND lease.lease_id = $9
					AND lease.fencing_generation = $10
					AND lease.expires_at > now()
			)`,
		string(result.Status), string(outputsJSON), result.Checkpoint, result.ErrorMessage,
		result.StartedAt, result.CompletedAt, stepID, operationID, lease.ID, lease.Generation, ownerID,
	)
	if err != nil {
		return fmt.Errorf("update fenced step: %w", err)
	}
	rows, err := update.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect fenced step update: %w", err)
	}
	if rows != 1 {
		return ErrLeaseLost
	}

	previousStatus := op.Status
	err = tx.QueryRowContext(ctx, `
		UPDATE operations SET
			completed_steps = completed_steps + CASE WHEN $2 = 'succeeded' THEN 1 ELSE 0 END,
			failed_steps = failed_steps + CASE WHEN $2 = 'failed' THEN 1 ELSE 0 END,
			status = CASE
				WHEN $2 = 'failed' THEN 'failed'
				WHEN completed_steps + 1 >= total_steps THEN 'succeeded'
				WHEN status = 'queued' THEN 'in_progress'
				ELSE status
			END,
			started_at = COALESCE(started_at, $3),
			completed_at = CASE
				WHEN $2 = 'failed' OR completed_steps + 1 >= total_steps THEN $4
				ELSE completed_at
			END,
			version = version + 1,
			updated_at = now()
		WHERE id = $1
		RETURNING status, completed_steps, failed_steps, version, started_at, completed_at`,
		operationID, string(result.Status), result.StartedAt, result.CompletedAt,
	).Scan(&op.Status, &op.CompletedSteps, &op.FailedSteps, &op.Version, &op.StartedAt, &op.CompletedAt)
	if err != nil {
		return fmt.Errorf("update operation progress: %w", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"step_id":            stepID,
		"status":             result.Status,
		"lease_id":           lease.ID,
		"fencing_generation": lease.Generation,
		"error":              result.ErrorMessage,
	})
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	auditType := engine.AuditStepCompleted
	if result.Status == engine.StepFailed {
		auditType = engine.AuditStepFailed
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO operation_audit (
			operation_id, event_type, actor_id, previous_state, new_state, detail
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		operationID, string(auditType), ownerID, string(previousStatus), string(op.Status), string(detailJSON),
	)
	if err != nil {
		return fmt.Errorf("insert operation audit: %w", err)
	}

	if err := upsertReadModel(ctx, tx, op); err != nil {
		return err
	}
	event.AggregateVersion = op.Version
	if err := insertOutboxEvent(ctx, tx, event); err != nil {
		return err
	}
	for _, additionalEvent := range additionalEvents {
		additionalEvent.AggregateVersion = op.Version
		if err := insertOutboxEvent(ctx, tx, additionalEvent); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM worker_leases
		WHERE step_id = $1 AND owner_id = $2 AND lease_id = $3
			AND fencing_generation = $4`,
		stepID, ownerID, lease.ID, lease.Generation,
	); err != nil {
		return fmt.Errorf("release step lease: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit step transaction: %w", err)
	}
	return nil
}

func getOperationForUpdate(ctx context.Context, tx *sql.Tx, id string) (*engine.Operation, error) {
	op := &engine.Operation{}
	var tagsJSON []byte
	var startedAt, completedAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, COALESCE(project_id, ''), COALESCE(environment_id, ''),
			COALESCE(namespace_id, ''), COALESCE(plan_id::text, ''), operation_type, status,
			initiated_by, COALESCE(approved_by, ''), COALESCE(correlation_id, ''),
			idempotency_key, COALESCE(plan_digest, ''),
			total_steps, completed_steps, failed_steps, version, tags,
			created_at, started_at, completed_at
		FROM operations WHERE id = $1 FOR UPDATE`, id,
	).Scan(
		&op.ID, &op.TenantID, &op.ProjectID, &op.EnvironmentID, &op.NamespaceID,
		&op.PlanID, &op.Type, &op.Status, &op.InitiatedBy, &op.ApprovedBy,
		&op.CorrelationID, &op.IdempotencyKey, &op.PlanDigest,
		&op.TotalSteps, &op.CompletedSteps, &op.FailedSteps, &op.Version, &tagsJSON,
		&op.CreatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("lock operation: %w", err)
	}
	if startedAt.Valid {
		op.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		op.CompletedAt = &completedAt.Time
	}
	if err := json.Unmarshal(tagsJSON, &op.Tags); err != nil {
		return nil, fmt.Errorf("decode operation tags: %w", err)
	}
	return op, nil
}

func upsertReadModel(ctx context.Context, tx *sql.Tx, op *engine.Operation) error {
	summary := fmt.Sprintf("%s %s: %s", op.Type, op.ID, op.Status)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO operation_read_model (
			operation_id, tenant_id, project_id, environment_id, namespace_id,
			plan_id, operation_type, status, total_steps,
			completed_steps, failed_steps, initiated_by, approved_by,
			summary, tags, created_at, started_at, completed_at,
			last_state_changed_at
		) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			NULLIF($6, '')::uuid, $7, $8, $9, $10, $11, $12, NULLIF($13, ''),
			$14, $15, $16, $17, $18, now())
		ON CONFLICT (operation_id) DO UPDATE SET
			status = EXCLUDED.status,
			completed_steps = EXCLUDED.completed_steps,
			failed_steps = EXCLUDED.failed_steps,
			approved_by = EXCLUDED.approved_by,
			started_at = COALESCE(EXCLUDED.started_at, operation_read_model.started_at),
			completed_at = COALESCE(EXCLUDED.completed_at, operation_read_model.completed_at),
			last_state_changed_at = now()`,
		op.ID, op.TenantID, op.ProjectID, op.EnvironmentID, op.NamespaceID,
		op.PlanID, string(op.Type), string(op.Status), op.TotalSteps,
		op.CompletedSteps, op.FailedSteps, op.InitiatedBy, op.ApprovedBy,
		summary, string(toJSON(op.Tags)), op.CreatedAt, op.StartedAt, op.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert operation read model: %w", err)
	}
	return nil
}

func insertOutboxEvent(ctx context.Context, tx *sql.Tx, event OutboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, project_id, environment_id, actor_id,
			correlation_id, causation_id, idempotency_key,
			aggregate_id, aggregate_version, operation_id, step_id,
			expected_version, payload
		) VALUES (
			$1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''),
			$10, NULLIF($11, '')::uuid, $12, NULLIF($13, ''), $14, $15, $16, $17, $18
		)`,
		event.MessageID, event.MessageType, event.SchemaVersion, event.Subject, event.OccurredAt,
		event.TenantID, event.ProjectID, event.EnvironmentID, event.ActorID,
		event.CorrelationID, event.CausationID, event.IdempotencyKey,
		event.AggregateID, event.AggregateVersion, event.OperationID, event.StepID,
		event.ExpectedVersion, string(payload),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func (s *OperationStore) CreateStep(opID string, spec engine.StepSpec, idempotencyKey string) (string, error) {
	inputsJSON, err := json.Marshal(spec.Inputs)
	if err != nil {
		return "", fmt.Errorf("marshal step inputs: %w", err)
	}

	var id string
	err = s.db.QueryRow(`
		INSERT INTO operation_steps (
			operation_id, plan_step_id, step_name, step_type, provider_id, status,
			idempotency_key, depends_on, optional,
			retry_count, max_retries, timeout_seconds,
			step_input
		) VALUES ($1, NULLIF($2, ''), $3, $4, NULLIF($5, ''), 'pending', $6, $7, $8, 0, $9, $10, $11)
		RETURNING id`,
		opID, spec.ID, spec.Name, spec.StepType, spec.ProviderID,
		idempotencyKey, pq.Array(spec.DependsOn), spec.Optional,
		spec.Retry.MaxRetries, spec.TimeoutS, string(inputsJSON),
	).Scan(&id)
	return id, err
}

func (s *OperationStore) GetSteps(opID string) ([]engine.StepResult, error) {
	rows, err := s.db.Query(`
		SELECT id, step_name, step_type, status, idempotency_key,
			step_output, COALESCE(error_message, ''), started_at, completed_at
		FROM operation_steps WHERE operation_id = $1 ORDER BY created_at`, opID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []engine.StepResult
	for rows.Next() {
		var r engine.StepResult
		var stepName, stepType, idempotencyKey string
		var outputsJSON []byte
		var startedAt, completedAt sql.NullTime
		err := rows.Scan(&r.StepID, &stepName, &stepType,
			&r.Status, &idempotencyKey, &outputsJSON,
			&r.ErrorMessage, &startedAt, &completedAt)
		if err != nil {
			return nil, err
		}
		if startedAt.Valid {
			r.StartedAt = startedAt.Time
		}
		if completedAt.Valid {
			r.CompletedAt = completedAt.Time
		}
		if err := json.Unmarshal(outputsJSON, &r.Outputs); err != nil {
			return nil, fmt.Errorf("decode step output: %w", err)
		}
		r.OperationID = opID
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *OperationStore) SaveCompensation(rec *engine.CompensationRecord) error {
	dataJSON, err := json.Marshal(rec.Data)
	if err != nil {
		return fmt.Errorf("marshal compensation data: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO compensation_records (
			operation_id, step_id, resource_type, resource_id,
			compensation_type, status, compensation_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rec.OperationID, rec.StepID, rec.ResourceType, rec.ResourceID,
		string(rec.CompensationType), rec.Status, string(dataJSON),
	)
	return err
}

func toJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

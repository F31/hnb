package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

func TestCommitStepSuccessIsFencedAndAtomic(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	operationID := uuid.NewString()
	stepID := uuid.NewString()
	idempotencyKey := "step:" + stepID
	_, err = db.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			idempotency_key, total_steps
		) VALUES ($1, 'tenant-test', 'deploy', 'queued', 'test', $2, 1)`,
		operationID, "operation:"+operationID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			idempotency_key, total_steps
		) VALUES ($1, 'tenant-test', 'deploy', 'queued', 'test', $2, 1)`,
		uuid.NewString(), "operation:"+operationID,
	); err == nil {
		t.Fatal("duplicate tenant-scoped operation idempotency key was accepted")
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, step_name, step_type, idempotency_key
		) VALUES ($1, $2, 'test-step', 'deploy', $3)`,
		stepID, operationID, idempotencyKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertFencingSchema(t, db, stepID)
	prerequisiteID := uuid.NewString()
	dependentID := uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, plan_step_id, step_name, step_type, idempotency_key
		) VALUES
			($1, $3, 'plan-prerequisite', 'prerequisite', 'deploy', $4),
			($2, $3, 'plan-dependent', 'dependent', 'deploy', $5)`,
		prerequisiteID, dependentID, operationID,
		"step:"+prerequisiteID, "step:"+dependentID,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE operation_steps SET depends_on = ARRAY['plan-prerequisite']
		WHERE id = $1`, dependentID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE operation_id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operation_read_model WHERE operation_id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM worker_leases WHERE step_id = $1", stepID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operations WHERE id = $1", operationID)
	})

	store := NewOperationStore(db)
	satisfied, err := store.DependenciesSatisfied(ctx, operationID, []string{"plan-prerequisite"})
	if err != nil {
		t.Fatal(err)
	}
	if satisfied {
		t.Fatal("pending prerequisite was treated as satisfied")
	}
	if _, err := db.ExecContext(ctx, "UPDATE operation_steps SET status = 'succeeded' WHERE id = $1", prerequisiteID); err != nil {
		t.Fatal(err)
	}
	satisfied, err = store.DependenciesSatisfied(ctx, operationID, []string{"plan-prerequisite"})
	if err != nil {
		t.Fatal(err)
	}
	if !satisfied {
		t.Fatal("succeeded prerequisite was not recognized")
	}
	if _, err := store.AcquireLease(ctx, operationID, stepID, "worker-a", 1, time.Minute); !errors.Is(err, ErrStepVersionMismatch) {
		t.Fatalf("lease acquisition accepted wrong StepRequested version: %v", err)
	}
	leaseA, err := store.AcquireLease(ctx, operationID, stepID, "worker-a", 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if leaseA.ID == "" || leaseA.Generation <= 0 {
		t.Fatalf("invalid lease returned: %+v", leaseA)
	}
	if _, err := store.AcquireLease(ctx, operationID, stepID, "worker-b", 0, time.Minute); !errors.Is(err, ErrLeaseNotAcquired) {
		t.Fatalf("second worker acquired active lease: %v", err)
	}
	assertStepFence(t, db, stepID, leaseA)
	assertCount(t, db, "SELECT version FROM operation_steps WHERE id = $1", stepID, 0)
	if _, err := db.ExecContext(ctx, `
		UPDATE operation_steps SET fencing_generation = $2 WHERE id = $1`,
		stepID, leaseA.Generation+1,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.RenewLease(ctx, stepID, "worker-a", leaseA, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("renewal ignored the retained Step fence: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE operation_steps SET fencing_generation = $2 WHERE id = $1`,
		stepID, leaseA.Generation,
	); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	result := engine.StepResult{
		StepID:      stepID,
		OperationID: operationID,
		Status:      engine.StepSucceeded,
		Outputs:     map[string]any{"status": "ok"},
		StartedAt:   now,
		CompletedAt: now,
	}
	event := OutboxEvent{
		MessageID:       uuid.NewString(),
		MessageType:     "hnb.event.operation.step-completed.v1",
		SchemaVersion:   "1.0.0",
		Subject:         "hnb.event.operation.step-completed.v1",
		OccurredAt:      now,
		TenantID:        "tenant-test",
		ActorID:         "worker-a",
		CorrelationID:   uuid.NewString(),
		CausationID:     uuid.NewString(),
		IdempotencyKey:  "step-completed:" + stepID,
		AggregateID:     operationID,
		OperationID:     operationID,
		StepID:          stepID,
		ExpectedVersion: 1,
		Payload: map[string]any{
			"operationId": operationID,
			"stepId":      stepID,
			"status":      "succeeded",
		},
	}

	err = store.CommitStepSuccess(
		ctx, operationID, stepID, idempotencyKey, "worker-a",
		Lease{ID: uuid.NewString(), Generation: leaseA.Generation}, 0, result, event,
	)
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong lease ID was not rejected: %v", err)
	}
	err = store.CommitStepSuccess(
		ctx, operationID, stepID, idempotencyKey, "worker-a",
		Lease{ID: leaseA.ID, Generation: leaseA.Generation + 1}, 0, result, event,
	)
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong fencing generation was not rejected: %v", err)
	}
	assertCount(t, db, "SELECT count(*) FROM outbox_events WHERE operation_id = $1", operationID, 0)
	assertCount(t, db, "SELECT completed_steps FROM operations WHERE id = $1", operationID, 0)
	if err := store.RenewLease(ctx, stepID, "worker-a", leaseA, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	leaseB, err := store.AcquireLease(ctx, operationID, stepID, "worker-b", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if leaseB.Generation <= leaseA.Generation+1 {
		t.Fatalf("active conflict did not burn a generation: first=%d takeover=%d", leaseA.Generation, leaseB.Generation)
	}
	if leaseB.ID == leaseA.ID {
		t.Fatal("expiry takeover reused the lease ID")
	}
	assertStepFence(t, db, stepID, leaseB)
	if err := store.RenewLease(ctx, stepID, "worker-a", leaseA, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale lease renewed after takeover: %v", err)
	}
	if err := store.CommitStepSuccess(
		ctx, operationID, stepID, idempotencyKey, "worker-a", leaseA, 0, result, event,
	); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale lease committed after takeover: %v", err)
	}

	if err := store.CommitStepSuccess(
		ctx, operationID, stepID, idempotencyKey, "worker-b", leaseB, 0, result, event,
	); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, "SELECT completed_steps FROM operations WHERE id = $1", operationID, 1)
	assertCount(t, db, "SELECT count(*) FROM outbox_events WHERE operation_id = $1", operationID, 1)
	assertCount(t, db, "SELECT count(*) FROM operation_audit WHERE operation_id = $1", operationID, 1)
	assertCount(t, db, "SELECT count(*) FROM operation_read_model WHERE operation_id = $1", operationID, 1)
	assertCount(t, db, "SELECT count(*) FROM worker_leases WHERE step_id = $1", stepID, 0)
	assertStepFence(t, db, stepID, leaseB)
	var auditLeaseID string
	var auditGeneration int64
	if err := db.QueryRowContext(ctx, `
		SELECT detail->>'lease_id', (detail->>'fencing_generation')::bigint
		FROM operation_audit
		WHERE operation_id = $1 AND event_type = 'step_completed'`, operationID,
	).Scan(&auditLeaseID, &auditGeneration); err != nil {
		t.Fatal(err)
	}
	if auditLeaseID != leaseB.ID || auditGeneration != leaseB.Generation {
		t.Fatalf("completion audit has wrong fence: id=%s generation=%d", auditLeaseID, auditGeneration)
	}

	var operationStatus, stepStatus string
	if err := db.QueryRowContext(ctx, "SELECT status FROM operations WHERE id = $1", operationID).Scan(&operationStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT status FROM operation_steps WHERE id = $1", stepID).Scan(&stepStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "succeeded" || stepStatus != "succeeded" {
		t.Fatalf("unexpected final states operation=%s step=%s", operationStatus, stepStatus)
	}
}

func TestCommitStepFailureIsAtomic(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	operationID := uuid.NewString()
	stepID := uuid.NewString()
	idempotencyKey := "step:" + stepID
	_, err = db.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			idempotency_key, total_steps
		) VALUES ($1, 'tenant-test', 'deploy', 'queued', 'test', $2, 1)`,
		operationID, "operation:"+operationID,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, step_name, step_type, idempotency_key
		) VALUES ($1, $2, 'test-step', 'deploy', $3)`,
		stepID, operationID, idempotencyKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE operation_id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operation_read_model WHERE operation_id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operations WHERE id = $1", operationID)
	})

	store := NewOperationStore(db)
	if _, err := db.ExecContext(ctx, "UPDATE operations SET status = 'paused' WHERE id = $1", operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLease(ctx, operationID, stepID, "worker-a", 0, time.Minute); !errors.Is(err, ErrOperationNotRunnable) {
		t.Fatalf("lease acquired for paused operation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE operations SET status = 'queued' WHERE id = $1", operationID); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireLease(ctx, operationID, stepID, "worker-a", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.SaveStepRetry(
		ctx, operationID, stepID, "worker-a", lease, 0,
		engine.StepResult{
			StepID:       stepID,
			OperationID:  operationID,
			Outputs:      map[string]any{"partial": "resource-created"},
			Checkpoint:   "resource-created",
			ErrorMessage: "provider timeout",
			StartedAt:    now,
		},
	); err != nil {
		t.Fatal(err)
	}
	var retryCount int
	var checkpoint string
	if err := db.QueryRow(
		"SELECT retry_count, checkpoint FROM operation_steps WHERE id = $1", stepID,
	).Scan(&retryCount, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if retryCount != 1 || checkpoint != "resource-created" {
		t.Fatalf("retry state was not persisted: count=%d checkpoint=%q", retryCount, checkpoint)
	}
	assertCount(t, db, "SELECT version FROM operation_steps WHERE id = $1", stepID, 0)
	assertCount(t, db, "SELECT count(*) FROM worker_leases WHERE step_id = $1", stepID, 0)
	assertStepFence(t, db, stepID, lease)
	var retryLeaseID string
	var retryGeneration int64
	if err := db.QueryRowContext(ctx, `
		SELECT detail->>'lease_id', (detail->>'fencing_generation')::bigint
		FROM operation_audit
		WHERE operation_id = $1 AND event_type = 'step_failed'`, operationID,
	).Scan(&retryLeaseID, &retryGeneration); err != nil {
		t.Fatal(err)
	}
	if retryLeaseID != lease.ID || retryGeneration != lease.Generation {
		t.Fatalf("retry audit has wrong fence: id=%s generation=%d", retryLeaseID, retryGeneration)
	}
	secondLease, err := store.AcquireLease(ctx, operationID, stepID, "worker-a", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if secondLease.Generation <= lease.Generation {
		t.Fatalf("reacquisition generation did not increase: first=%d second=%d", lease.Generation, secondLease.Generation)
	}
	err = store.CommitStepFailure(
		ctx, operationID, stepID, idempotencyKey, "worker-a", secondLease, 0,
		engine.StepResult{
			StepID:       stepID,
			OperationID:  operationID,
			ErrorMessage: "provider unavailable",
			StartedAt:    now,
			CompletedAt:  now,
		},
		OutboxEvent{
			MessageID:       uuid.NewString(),
			MessageType:     "hnb.event.operation.step-completed.v1",
			SchemaVersion:   "1.0.0",
			Subject:         "hnb.event.operation.step-completed.v1",
			OccurredAt:      now,
			TenantID:        "tenant-test",
			ActorID:         "worker-a",
			CorrelationID:   uuid.NewString(),
			CausationID:     uuid.NewString(),
			IdempotencyKey:  "step-failed:" + stepID,
			AggregateID:     operationID,
			OperationID:     operationID,
			StepID:          stepID,
			ExpectedVersion: 1,
			Payload: map[string]any{
				"operationId": operationID,
				"stepId":      stepID,
				"status":      "failed",
				"error":       "provider unavailable",
			},
		},
		OutboxEvent{
			MessageID:       uuid.NewString(),
			MessageType:     "hnb.failed.command.operation.step-requested.v1",
			SchemaVersion:   "1.0.0",
			Subject:         "hnb.failed.command.operation.step-requested.v1",
			OccurredAt:      now,
			TenantID:        "tenant-test",
			ActorID:         "worker-a",
			CorrelationID:   uuid.NewString(),
			CausationID:     uuid.NewString(),
			IdempotencyKey:  "failed-message:" + stepID,
			AggregateID:     operationID,
			OperationID:     operationID,
			StepID:          stepID,
			ExpectedVersion: 1,
			Payload:         map[string]any{"failureReason": "provider unavailable"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, "SELECT failed_steps FROM operations WHERE id = $1", operationID, 1)
	assertCount(t, db, "SELECT count(*) FROM outbox_events WHERE operation_id = $1", operationID, 2)
	var operationStatus, stepStatus string
	if err := db.QueryRow("SELECT status FROM operations WHERE id = $1", operationID).Scan(&operationStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT status FROM operation_steps WHERE id = $1", stepID).Scan(&stepStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "failed" || stepStatus != "failed" {
		t.Fatalf("unexpected failure states operation=%s step=%s", operationStatus, stepStatus)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, arg any, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, arg).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query %q returned %d, want %d", query, got, want)
	}
}

func assertStepFence(t *testing.T, db *sql.DB, stepID string, want Lease) {
	t.Helper()
	var leaseID string
	var generation int64
	if err := db.QueryRow(`
		SELECT last_lease_id::text, fencing_generation
		FROM operation_steps WHERE id = $1`, stepID,
	).Scan(&leaseID, &generation); err != nil {
		t.Fatal(err)
	}
	if leaseID != want.ID || generation != want.Generation {
		t.Fatalf("Step fence = {%s %d}, want {%s %d}", leaseID, generation, want.ID, want.Generation)
	}
}

func assertFencingSchema(t *testing.T, db *sql.DB, stepID string) {
	t.Helper()
	var dataType string
	var cycle bool
	if err := db.QueryRow(`
		SELECT data_type, cycle
		FROM pg_sequences
		WHERE schemaname = current_schema()
			AND sequencename = 'operation_fencing_generation_seq'`,
	).Scan(&dataType, &cycle); err != nil {
		t.Fatal(err)
	}
	if dataType != "bigint" || cycle {
		t.Fatalf("unexpected fencing sequence: data_type=%s cycle=%t", dataType, cycle)
	}
	var indexCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = current_schema()
			AND indexname = 'idx_operation_steps_last_lease_id'`,
	).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("renamed Step lease index count = %d, want 1", indexCount)
	}
	if _, err := db.Exec(`
		INSERT INTO worker_leases (
			step_id, owner_id, lease_id, fencing_generation, expires_at
		) VALUES ($1, 'schema-test', gen_random_uuid(), 1, now() + interval '1 minute')`,
		uuid.NewString(),
	); err == nil {
		t.Fatal("worker lease accepted a nonexistent Step")
	}
	if _, err := db.Exec(`
		INSERT INTO worker_leases (
			step_id, owner_id, lease_id, fencing_generation, expires_at
		) VALUES ($1, 'schema-test', gen_random_uuid(), 0, now() + interval '1 minute')`,
		stepID,
	); err == nil {
		t.Fatal("worker lease accepted a non-positive fencing generation")
	}
	if _, err := db.Exec(`
		UPDATE operation_steps SET fencing_generation = -1 WHERE id = $1`, stepID,
	); err == nil {
		t.Fatal("Step accepted a negative fencing generation")
	}
}

package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

// A terminal application delivery Operation (tagged with applicationDeliveryId)
// records the execution/observation dimensions of the readiness ledger in the
// same transaction; external probe and volume evidence stay operator-supplied.
func TestCommitStepSuccessRecordsApplicationVerification(t *testing.T) {
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
	targetID := uuid.NewString()
	bindingID := uuid.NewString()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, display_name) VALUES ('tenant-test', 'tenant-test', 'tenant-test')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (
			id, tenant_id, name, target_type, freshness_state,
			observation_generation, observation_revision
		) VALUES ($1, 'tenant-test', $2, 'kubernetes', 'FRESH', 3, 7)`,
		targetID, "target-"+targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO application_delivery_bindings (
			id, tenant_id, target_id, namespace, release_id, release_version,
			revision, resource_uid, status, observation_generation, observation_revision
		) VALUES ($1, 'tenant-test', $2, 'hnb-e2e', 'demo', 'v1', 1, 'uid-1', 'Deploying', 3, 7)`,
		bindingID, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, total_steps, tags
		) VALUES ($1, 'tenant-test', 'deploy', 'queued', 'test', $2, $3, 1,
			$4::jsonb)`,
		operationID, uuid.NewString(), "operation:"+operationID,
		`{"applicationDeliveryId":"`+bindingID+`","applicationRevision":"v1"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, step_name, step_type, idempotency_key
		) VALUES ($1, $2, 'test-step', 'deploy', $3)`,
		stepID, operationID, idempotencyKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM application_delivery_verifications WHERE binding_id = $1", bindingID)
		_, _ = db.ExecContext(ctx, "DELETE FROM application_delivery_bindings WHERE id = $1", bindingID)
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE operation_id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operation_read_model WHERE operation_id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM worker_leases WHERE step_id = $1", stepID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operations WHERE id = $1", operationID)
		_, _ = db.ExecContext(ctx, "DELETE FROM runtime_targets WHERE id = $1", targetID)
	})

	store := NewOperationStore(db)
	lease, err := store.AcquireLease(ctx, operationID, stepID, "worker-a", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	result := engine.StepResult{
		StepID: stepID, OperationID: operationID, Status: engine.StepSucceeded,
		Outputs: map[string]any{"status": "ok"}, StartedAt: now, CompletedAt: now,
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
		Payload:         map[string]any{"operationId": operationID, "stepId": stepID, "status": "succeeded"},
	}
	if err := store.CommitStepSuccess(ctx, operationID, stepID, idempotencyKey, "worker-a", lease, 0, result, event); err != nil {
		t.Fatal(err)
	}

	var (
		executionStatus, freshness, externalProbe, volume string
		ready                                             bool
		reasonsRaw                                        []byte
		recordedOperation, recordedUID                    string
		generation, revision                              int64
	)
	err = db.QueryRowContext(ctx, `
		SELECT execution_status, observation_freshness, external_probe_status, volume_status,
			ready, reasons, operation_id::text, resource_uid, observation_generation, observation_revision
		FROM application_delivery_verifications WHERE binding_id = $1 AND revision = 1`,
		bindingID,
	).Scan(&executionStatus, &freshness, &externalProbe, &volume, &ready,
		&reasonsRaw, &recordedOperation, &recordedUID, &generation, &revision)
	if err != nil {
		t.Fatal(err)
	}
	if executionStatus != "succeeded" || freshness != "FRESH" {
		t.Fatalf("execution/observation = %s/%s", executionStatus, freshness)
	}
	if externalProbe != "not_run" || volume != "not_run" {
		t.Fatalf("external/volume = %s/%s", externalProbe, volume)
	}
	if ready {
		t.Fatal("record without external/volume evidence must not be ready")
	}
	if recordedOperation != operationID || recordedUID != "uid-1" || generation != 3 || revision != 7 {
		t.Fatalf("linkage = %s/%s gen=%d rev=%d", recordedOperation, recordedUID, generation, revision)
	}
	if !strings.Contains(string(reasonsRaw), "EXTERNAL_PROBE_FAILED") ||
		!strings.Contains(string(reasonsRaw), "VOLUME_READ_WRITE_FAILED") {
		t.Fatalf("reasons = %s", reasonsRaw)
	}
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func redriveFixture(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	operationID, stepID := uuid.NewString(), uuid.NewString()
	correlationID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO operations (id, tenant_id, operation_type, status, initiated_by, correlation_id, idempotency_key, total_steps)
		VALUES ($1, 'tenant-redrive', 'deploy', 'queued', 'test', $2, $3, 1)`,
		operationID, correlationID, "operation:"+operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO operation_steps (id, operation_id, step_name, step_type, idempotency_key, max_retries)
		VALUES ($1, $2, 'redrive-step', 'deploy', $3, 2)`,
		stepID, operationID, "step:"+stepID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM outbox_events WHERE step_id = $1`, stepID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM operation_steps WHERE id = $1`, stepID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM operations WHERE id = $1`, operationID)
	})
	return db, operationID, stepID
}

func addStepRequest(t *testing.T, db *sql.DB, operationID, stepID, status string) string {
	t.Helper()
	messageID := uuid.NewString()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO outbox_events (message_id, message_type, schema_version, subject, tenant_id, correlation_id,
			aggregate_id, operation_id, step_id, idempotency_key, payload, status, published_at)
		VALUES ($1, $2, '2.0.0', 'hnb.command.operation.step-requested.v1', 'tenant-redrive', $3,
			$4::uuid, $4::uuid, $5::uuid, $7::text, jsonb_build_object('stepId', $5::text, 'idempotencyKey', $7::text), $6,
			CASE WHEN $6 = 'published' THEN now() ELSE NULL END)`,
		messageID, stepRequestedMessageType, uuid.NewString(), operationID, stepID, status,
		"step:"+stepID); err != nil {
		t.Fatal(err)
	}
	return messageID
}

func markIdle(t *testing.T, db *sql.DB, stepID string, ago time.Duration) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`UPDATE operation_steps SET updated_at = now() - ($2 * interval '1 millisecond') WHERE id = $1`,
		stepID, ago.Milliseconds()); err != nil {
		t.Fatal(err)
	}
}

func publishPending(t *testing.T, db *sql.DB, stepID string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`UPDATE outbox_events SET status = 'published', published_at = now()
		 WHERE step_id = $1 AND status <> 'published'`, stepID); err != nil {
		t.Fatal(err)
	}
}

func sameJSON(t *testing.T, a, b string) bool {
	t.Helper()
	var left, right any
	if err := json.Unmarshal([]byte(a), &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(b), &right); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(left, right)
}

func requestAttempt(t *testing.T, db *sql.DB, stepID string) int {
	t.Helper()
	var attempt int
	if err := db.QueryRow(`SELECT attempt FROM outbox_events WHERE step_id = $1`, stepID).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	return attempt
}

func countRequests(t *testing.T, db *sql.DB, stepID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM outbox_events WHERE step_id = $1 AND message_type = $2`,
		stepID, stepRequestedMessageType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// The idle threshold is expressed in milliseconds; a seconds/milliseconds mix-up
// would make every step look either permanently idle or never idle.
func TestRequeueOrphanedStepsUsesMillisecondThreshold(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	addStepRequest(t, db, operationID, stepID, "published")
	store := NewOperationStore(db)

	// Idle for 10s with a 2-minute threshold: must not be selected.
	markIdle(t, db, stepID, 10*time.Second)
	result, err := store.RequeueOrphanedSteps(context.Background(), 2*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 0 {
		t.Fatalf("a step idle for 10s was re-driven under a 2m threshold: %+v", result)
	}

	// Idle for 3 minutes: must be selected.
	markIdle(t, db, stepID, 3*time.Minute)
	result, err = store.RequeueOrphanedSteps(context.Background(), 2*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 1 || result.Requeued[0] != stepID {
		t.Fatalf("a step idle for 3m was not re-driven under a 2m threshold: %+v", result)
	}
}

func TestRequeueOrphanedStepsRequeuesLostRequest(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	original := addStepRequest(t, db, operationID, stepID, "published")
	markIdle(t, db, stepID, 5*time.Minute)
	store := NewOperationStore(db)

	result, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 1 || result.Requeued[0] != stepID || len(result.Exhausted) != 0 {
		t.Fatalf("unexpected result %+v", result)
	}
	var messageCount int
	var payload, key, statusText string
	var attempt int
	if err := db.QueryRow(`SELECT count(*), min(payload::text), min(idempotency_key), min(status), min(attempt)
		FROM outbox_events WHERE step_id = $1`, stepID).Scan(&messageCount, &payload, &key, &statusText, &attempt); err != nil {
		t.Fatal(err)
	}
	// The request is re-armed in place: one row, one idempotency key that the worker
	// can still match against operation_steps, and one more delivery attempt.
	if messageCount != 1 {
		t.Fatalf("re-drive created %d rows; a copy would break the worker's key match", messageCount)
	}
	if key != "step:"+stepID {
		t.Fatalf("idempotency key changed on re-drive: %s", key)
	}
	if statusText != "pending" || attempt != 1 {
		t.Fatalf("row was not re-armed: status=%s attempt=%d", statusText, attempt)
	}
	if !sameJSON(t, payload, `{"stepId":"`+stepID+`","idempotencyKey":"step:`+stepID+`"}`) {
		t.Fatalf("payload changed on re-drive: %s", payload)
	}
	if _, err := db.Exec(`SELECT 1 FROM outbox_events WHERE message_id = $1`, original); err != nil {
		t.Fatal(err)
	}

	// A fresh step is not idle yet, so the next sweep leaves it alone.
	again, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Requeued) != 0 {
		t.Fatalf("sweep re-queued a step that was just re-driven: %+v", again)
	}
}

func TestRequeueOrphanedStepsSkipsUnpublishedAndBusySteps(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	addStepRequest(t, db, operationID, stepID, "pending")
	markIdle(t, db, stepID, 5*time.Minute)
	store := NewOperationStore(db)

	result, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 0 {
		t.Fatalf("re-queued a step whose request was still unpublished: %+v", result)
	}
	if countRequests(t, db, stepID) != 1 || requestAttempt(t, db, stepID) != 0 {
		t.Fatal("an unpublished request was re-armed")
	}
}

func TestRequeueOrphanedStepsRespectsRetryBudget(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	addStepRequest(t, db, operationID, stepID, "published")
	store := NewOperationStore(db)

	// max_retries is 2 for the fixture: the original plus two re-drives, then stop.
	// Each round also plays the relay, otherwise the pending copy suppresses the sweep.
	for round := 0; round < 2; round++ {
		publishPending(t, db, stepID)
		markIdle(t, db, stepID, 5*time.Minute)
		result, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Requeued) != 1 {
			t.Fatalf("round %d: %+v", round, result)
		}
	}
	publishPending(t, db, stepID)
	markIdle(t, db, stepID, 5*time.Minute)
	result, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 0 || len(result.Exhausted) != 1 || result.Exhausted[0] != stepID {
		t.Fatalf("budget was not enforced: %+v", result)
	}
	if countRequests(t, db, stepID) != 1 || requestAttempt(t, db, stepID) != 2 {
		t.Fatalf("expected one re-armed row at attempt 2, got %d rows at attempt %d",
			countRequests(t, db, stepID), requestAttempt(t, db, stepID))
	}
}

func TestRequeueOrphanedStepsIsSafeUnderConcurrency(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	addStepRequest(t, db, operationID, stepID, "published")
	markIdle(t, db, stepID, 5*time.Minute)
	store := NewOperationStore(db)

	var wg sync.WaitGroup
	results := make([]RedriveResult, 4)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			out, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
			if err != nil {
				t.Errorf("sweep %d: %v", index, err)
			}
			results[index] = out
		}(i)
	}
	wg.Wait()

	total := 0
	for _, result := range results {
		total += len(result.Requeued)
	}
	if total != 1 {
		t.Fatalf("concurrent sweeps produced %d re-drives for one step, want exactly 1", total)
	}
	if countRequests(t, db, stepID) != 1 || requestAttempt(t, db, stepID) != 1 {
		t.Fatalf("expected a single re-armed row at attempt 1, got %d rows at attempt %d",
			countRequests(t, db, stepID), requestAttempt(t, db, stepID))
	}
}

func TestRequeueOrphanedStepsIgnoresTerminalOperations(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	addStepRequest(t, db, operationID, stepID, "published")
	if _, err := db.ExecContext(context.Background(),
		`UPDATE operations SET status = 'succeeded' WHERE id = $1`, operationID); err != nil {
		t.Fatal(err)
	}
	markIdle(t, db, stepID, 5*time.Minute)
	store := NewOperationStore(db)

	result, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 0 {
		t.Fatalf("re-queued a step of a terminal operation: %+v", result)
	}
}

func TestRequeueOrphanedStepsSkipsPublishedWithUnpublishedRequest(t *testing.T) {
	for _, status := range []string{"pending", "failed"} {
		t.Run(status, func(t *testing.T) {
			db, operationID, stepID := redriveFixture(t)
			original := addStepRequest(t, db, operationID, stepID, "published")
			if _, err := db.Exec(`INSERT INTO outbox_events
				(message_id, message_type, schema_version, subject, tenant_id, correlation_id,
				 operation_id, step_id, idempotency_key, payload, status)
				SELECT $2, message_type, schema_version, subject, tenant_id, correlation_id,
				 operation_id, step_id, idempotency_key || '#legacy', payload, $3
				FROM outbox_events WHERE message_id = $1`, original, uuid.NewString(), status); err != nil {
				t.Fatal(err)
			}
			markIdle(t, db, stepID, 5*time.Minute)
			result, err := NewOperationStore(db).RequeueOrphanedSteps(context.Background(), time.Minute, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Requeued) != 0 || len(result.Exhausted) != 0 {
				t.Fatalf("selected step with an unpublished request: %+v", result)
			}
		})
	}
}

func TestRequeueOrphanedStepsUsesCanonicalRequest(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	original := addStepRequest(t, db, operationID, stepID, "published")
	legacy := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO outbox_events
		(message_id, message_type, schema_version, subject, tenant_id, correlation_id,
		 operation_id, step_id, idempotency_key, payload, status, occurred_at)
		SELECT $2, message_type, schema_version, subject, tenant_id, correlation_id,
		 operation_id, step_id, idempotency_key || '#redrive-1', payload, status, occurred_at + interval '1 second'
		FROM outbox_events WHERE message_id = $1`, original, legacy); err != nil {
		t.Fatal(err)
	}
	markIdle(t, db, stepID, 5*time.Minute)
	result, err := NewOperationStore(db).RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 1 {
		t.Fatalf("canonical request was not re-armed: %+v", result)
	}
	var status string
	var attempt int
	if err := db.QueryRow(`SELECT status, attempt FROM outbox_events WHERE message_id = $1`, original).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempt != 1 {
		t.Fatalf("canonical request: status=%s attempt=%d", status, attempt)
	}
	if err := db.QueryRow(`SELECT status, attempt FROM outbox_events WHERE message_id = $1`, legacy).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "published" || attempt != 0 {
		t.Fatalf("legacy request was changed: status=%s attempt=%d", status, attempt)
	}
}

func TestRequeueOrphanedStepsRespectsOutboxBudget(t *testing.T) {
	db, operationID, stepID := redriveFixture(t)
	addStepRequest(t, db, operationID, stepID, "published")
	if _, err := db.Exec(`UPDATE outbox_events SET max_attempts = 2 WHERE step_id = $1`, stepID); err != nil {
		t.Fatal(err)
	}
	store := NewOperationStore(db)
	markIdle(t, db, stepID, 5*time.Minute)
	result, err := store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 1 || requestAttempt(t, db, stepID) != 1 {
		t.Fatalf("last publishable attempt not re-armed: %+v", result)
	}
	publishPending(t, db, stepID)
	markIdle(t, db, stepID, 5*time.Minute)
	result, err = store.RequeueOrphanedSteps(context.Background(), time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requeued) != 0 || len(result.Exhausted) != 1 || requestAttempt(t, db, stepID) != 1 {
		t.Fatalf("created an attempt the relay cannot publish: %+v", result)
	}
}

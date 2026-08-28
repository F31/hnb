package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestStorageReconcileRecheckOptimisticConcurrencyFencingAndTenantScope(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID := integrationTenant(t, db, "storage-reconcile")
	otherTenant := integrationTenant(t, db, "storage-reconcile-other")
	for _, tenant := range []string{tenantID, otherTenant} {
		if _, err := db.Exec(`INSERT INTO tenants(id,name,display_name) VALUES($1,$1,$1) ON CONFLICT(id) DO NOTHING`, tenant); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM storage_class_bindings WHERE tenant_id=$1`, tenantID)
		_, _ = db.Exec(`DELETE FROM workload_storage_offerings WHERE tenant_id=$1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_targets WHERE tenant_id=$1`, tenantID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id IN ($1,$2)`, tenantID, otherTenant)
	})
	targetID, offeringID, bindingID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO runtime_targets(id,tenant_id,name,target_type,projection_version) VALUES($1,$2,$3,'kubernetes',7)`, targetID, tenantID, "storage-target-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO workload_storage_offerings(id,tenant_id,name,service_mode,access_modes,volume_expansion,snapshots,clones,protection_class)
		VALUES($1,$2,$3,'Block','["ReadWriteOnce"]','Unknown','Unknown','Unknown','standard')`, offeringID, tenantID, "storage-offering-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO storage_class_bindings(id,tenant_id,offering_id,offering_version,target_id,storage_class_name,storage_class_uid,storage_class_resource_version,sync_state,source,observed_at,freshness)
		VALUES($1,$2,$3,1,$4,'fast','desired-uid','1','Drifted','runtime_target_storage_inventory',now(),'Fresh')`, bindingID, tenantID, offeringID, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runtime_target_storage_inventory(tenant_id,target_id,resource_kind,resource_uid,resource_version,name,source,observed_at,stale_after,observation_source,observation_source_id,observation_generation,observation_revision,attributes)
		VALUES($1,$2,'StorageClass','observed-uid','9','fast','kubernetes.storage.k8s.io/v1',now(),now()+interval '5 minutes','agent','agent-a',1,3,'{}')`, tenantID, targetID); err != nil {
		t.Fatal(err)
	}

	base := IntentSubmitCommand{TenantID: tenantID, Intent: &engine.RuntimeIntent{Kind: engine.IntentReconcileStorageClassBinding, Spec: engine.IntentSpec{
		BindingID: bindingID, BindingVersion: 1, OfferingID: offeringID, OfferingVersion: 1,
		TargetID: targetID, StorageClassName: "fast", StorageClassUID: "desired-uid", StorageClassResourceVersion: "1",
	}}}
	check := func(cmd IntentSubmitCommand) error {
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		return recheckStorageReconcile(context.Background(), tx, cmd)
	}
	if err := check(base); err != nil {
		t.Fatalf("fresh matching fence: %v", err)
	}
	staleVersion := base
	staleVersion.Intent = &engine.RuntimeIntent{Kind: base.Intent.Kind, Spec: base.Intent.Spec}
	staleVersion.Intent.Spec.BindingVersion = 2
	if err := check(staleVersion); !errors.Is(err, ErrStorageBindingConflict) {
		t.Fatalf("binding version err=%v", err)
	}
	changedResourceVersion := base
	changedResourceVersion.Intent = &engine.RuntimeIntent{Kind: base.Intent.Kind, Spec: base.Intent.Spec}
	changedResourceVersion.Intent.Spec.StorageClassResourceVersion = "8"
	if err := check(changedResourceVersion); !errors.Is(err, ErrStorageBindingConflict) {
		t.Fatalf("resourceVersion fence err=%v", err)
	}
	crossTenant := base
	crossTenant.TenantID = otherTenant
	if err := check(crossTenant); !errors.Is(err, ErrStorageBindingConflict) {
		t.Fatalf("cross-tenant fence err=%v", err)
	}
	if _, err := db.Exec(`UPDATE storage_class_bindings SET freshness='Stale' WHERE tenant_id=$1 AND id=$2`, tenantID, bindingID); err != nil {
		t.Fatal(err)
	}
	if err := check(base); !errors.Is(err, ErrStorageObservationConflict) {
		t.Fatalf("stale observation err=%v", err)
	}
}

// Integration tests for PGStore against a real PostgreSQL database. They are
// gated on HNB_TEST_POSTGRES_DSN, mirroring
// cmd/operation-worker/internal/store/operations_integration_test.go.
//
// Every test uses its own tenant_id (tenant-itest-pag-*) and removes the rows
// it inserted when finished.

func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// integrationTenant returns a unique tenant ID and registers cleanup for all
// rows the test leaves behind under that tenant.
func integrationTenant(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	tenantID := fmt.Sprintf("tenant-itest-pag-%s-%s", name, uuid.NewString()[:8])
	t.Cleanup(func() {
		ctx := context.Background()
		// operations cascades into operation_steps and operation_audit;
		// execution_plans must go last because operations.plan_id references it.
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operation_read_model WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operations WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM execution_plans WHERE tenant_id = $1", tenantID)
	})
	return tenantID
}

func countWhere(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var got int
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return got
}

func submitCommand(tenantID, operationType string, steps ...StepInput) SubmitCommand {
	return SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  operationType,
		IdempotencyKey: "itest-" + uuid.NewString(),
		InitiatedBy:    "itest-user",
		CorrelationID:  uuid.NewString(),
		Tags:           map[string]string{"suite": "integration"},
		Steps:          steps,
	}
}

func TestSubmitOperationTransactionIsAtomic(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "submit")
	store := NewPGStore(db)

	cmd := submitCommand(tenantID, "deploy",
		StepInput{Name: "apply", StepType: "deploy", MaxRetries: 2, TimeoutSeconds: 120},
		StepInput{Name: "verify", StepType: "verify", DependsOn: []string{"step-1"}, MaxRetries: 1, TimeoutSeconds: 60},
	)
	op, created, err := store.SubmitOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first submit reported created=false")
	}
	if op.Status != StatusQueued {
		t.Fatalf("deploy initial status = %s, want queued", op.Status)
	}
	if op.PlanID == "" {
		t.Fatalf("operation missing plan linkage: %+v", op)
	}
	if len(op.Steps) != 2 {
		t.Fatalf("GetOperation returned %d steps, want 2", len(op.Steps))
	}

	// The write side links operations.plan_id to a matching execution_plans row
	// with a consistent digest (GetOperation reads the projection, which by
	// design does not carry plan_digest — KERNEL-003).
	var planDigest, planRowDigest string
	if err := db.QueryRowContext(ctx,
		"SELECT plan_digest FROM operations WHERE id = $1", op.ID,
	).Scan(&planDigest); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT plan_digest FROM execution_plans WHERE id = $1 AND tenant_id = $2",
		op.PlanID, tenantID,
	).Scan(&planRowDigest); err != nil {
		t.Fatalf("execution_plans row missing: %v", err)
	}
	if planDigest == "" || planDigest != planRowDigest {
		t.Fatalf("plan digest mismatch: operations=%q execution_plans=%q", planDigest, planRowDigest)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM operations WHERE id = $1 AND status = 'queued' AND total_steps = 2",
		op.ID); got != 1 {
		t.Fatalf("operations row missing or wrong: %d", got)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM operation_steps WHERE operation_id = $1 AND status = 'pending'",
		op.ID); got != 2 {
		t.Fatalf("operation_steps rows = %d, want 2", got)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM operation_audit WHERE operation_id = $1 AND event_type = 'created' AND new_state = 'queued'",
		op.ID); got != 1 {
		t.Fatalf("operation_audit rows = %d, want 1", got)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM operation_read_model WHERE operation_id = $1 AND tenant_id = $2 AND status = 'queued'",
		op.ID, tenantID); got != 1 {
		t.Fatalf("operation_read_model rows = %d, want 1", got)
	}

	// Exactly one ready step (no depends_on) produces one outbox event.
	var subject, messageType, payloadOpID, payloadStepID, stepID string
	err = db.QueryRowContext(ctx, `
		SELECT subject, message_type, payload->>'operationId', payload->>'stepId', step_id::text
		FROM outbox_events WHERE operation_id = $1`, op.ID,
	).Scan(&subject, &messageType, &payloadOpID, &payloadStepID, &stepID)
	if err != nil {
		t.Fatalf("expected exactly one outbox event: %v", err)
	}
	if subject != StepRequestedSubject || messageType != StepRequestedSubject {
		t.Fatalf("outbox subject/message_type = %s/%s, want %s", subject, messageType, StepRequestedSubject)
	}
	if payloadOpID != op.ID || payloadStepID != stepID {
		t.Fatalf("outbox payload mismatch: operationId=%s stepId=%s step_id=%s", payloadOpID, payloadStepID, stepID)
	}
	// The event must belong to the dependency-free first step.
	if got := countWhere(t, db,
		"SELECT count(*) FROM operation_steps WHERE id = $1 AND (depends_on IS NULL OR depends_on = '{}')",
		stepID); got != 1 {
		t.Fatalf("outbox event was emitted for a dependent step %s", stepID)
	}
}

func TestSubmitOperationIdempotentReplay(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "replay")
	store := NewPGStore(db)

	cmd := submitCommand(tenantID, "deploy",
		StepInput{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
	)
	first, created, err := store.SubmitOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first submit reported created=false")
	}

	second, created, err := store.SubmitOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("idempotent replay reported created=true")
	}
	if second.ID != first.ID {
		t.Fatalf("replay returned different operation ID: %s vs %s", second.ID, first.ID)
	}

	checks := []struct {
		query string
		arg   any
		want  int
	}{
		{"SELECT count(*) FROM operations WHERE tenant_id = $1", tenantID, 1},
		{"SELECT count(*) FROM operation_steps WHERE operation_id = $1", first.ID, 1},
		{"SELECT count(*) FROM operation_audit WHERE operation_id = $1", first.ID, 1},
		{"SELECT count(*) FROM operation_read_model WHERE operation_id = $1", first.ID, 1},
		{"SELECT count(*) FROM outbox_events WHERE operation_id = $1", first.ID, 1},
	}
	for _, c := range checks {
		if got := countWhere(t, db, c.query, c.arg); got != c.want {
			t.Fatalf("after replay query %q = %d, want %d", c.query, got, c.want)
		}
	}
}

func TestHighRiskOperationApprovalLifecycle(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "approval")
	store := NewPGStore(db)

	cmd := submitCommand(tenantID, "delete",
		StepInput{Name: "teardown", StepType: "delete", MaxRetries: 0, TimeoutSeconds: 30},
	)
	op, created, err := store.SubmitOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("submit reported created=false")
	}
	if op.Status != StatusPendingApproval {
		t.Fatalf("high-risk initial status = %s, want pending_approval", op.Status)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM outbox_events WHERE operation_id = $1", op.ID); got != 0 {
		t.Fatalf("pending_approval operation emitted %d outbox events, want 0", got)
	}

	approved, err := store.ApproveOperation(ctx, op.ID, tenantID, "itest-approver", "looks safe")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != StatusQueued {
		t.Fatalf("status after approve = %s, want queued", approved.Status)
	}
	if approved.ApprovedBy != "itest-approver" {
		t.Fatalf("approved_by = %q, want itest-approver", approved.ApprovedBy)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND subject = $2",
		op.ID, StepRequestedSubject); got != 1 {
		t.Fatalf("approve emitted %d outbox events, want 1", got)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM operation_audit WHERE operation_id = $1 AND event_type = 'approved' AND new_state = 'queued'",
		op.ID); got != 1 {
		t.Fatalf("approval audit rows = %d, want 1", got)
	}

	// Approving a second time must be rejected by the state guard.
	if _, err := store.ApproveOperation(ctx, op.ID, tenantID, "itest-approver", "again"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second approve: err = %v, want ErrInvalidState", err)
	}

	// Reject path: a separate high-risk operation goes to cancelled.
	rejectCmd := submitCommand(tenantID, "rollback",
		StepInput{Name: "rollback", StepType: "rollback", MaxRetries: 0, TimeoutSeconds: 30},
	)
	rejectOp, _, err := store.SubmitOperation(ctx, rejectCmd)
	if err != nil {
		t.Fatal(err)
	}
	if rejectOp.Status != StatusPendingApproval {
		t.Fatalf("rollback initial status = %s, want pending_approval", rejectOp.Status)
	}
	rejected, err := store.RejectOperation(ctx, rejectOp.ID, tenantID, "itest-approver", "too risky")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != StatusCancelled {
		t.Fatalf("status after reject = %s, want cancelled", rejected.Status)
	}
	if rejected.CompletedAt == nil {
		t.Fatal("rejected operation has no completed_at")
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM operation_audit WHERE operation_id = $1 AND event_type = 'rejected' AND new_state = 'cancelled'",
		rejectOp.ID); got != 1 {
		t.Fatalf("rejection audit rows = %d, want 1", got)
	}
	if got := countWhere(t, db,
		"SELECT count(*) FROM outbox_events WHERE operation_id = $1", rejectOp.ID); got != 0 {
		t.Fatalf("rejected operation emitted %d outbox events, want 0", got)
	}
}

func TestCancelOperationStateGuards(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "cancel")
	otherTenant := integrationTenant(t, db, "cancel-other")
	store := NewPGStore(db)

	cmd := submitCommand(tenantID, "deploy",
		StepInput{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
	)
	op, _, err := store.SubmitOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}

	// queued -> cancelled is a legal transition.
	cancelled, err := store.CancelOperation(ctx, op.ID, tenantID, "itest-user", "no longer needed")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status after cancel = %s, want cancelled", cancelled.Status)
	}
	if cancelled.CompletedAt == nil {
		t.Fatal("cancelled operation has no completed_at")
	}

	// Terminal states reject further transitions (409 semantics at the API layer).
	if _, err := store.CancelOperation(ctx, op.ID, tenantID, "itest-user", "again"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cancel of cancelled operation: err = %v, want ErrInvalidState", err)
	}
	if _, err := store.ApproveOperation(ctx, op.ID, tenantID, "itest-approver", "late"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("approve of cancelled operation: err = %v, want ErrInvalidState", err)
	}

	// succeeded is terminal as well.
	succeededCmd := submitCommand(tenantID, "deploy",
		StepInput{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
	)
	succeededOp, _, err := store.SubmitOperation(ctx, succeededCmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE operations SET status = 'succeeded' WHERE id = $1", succeededOp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CancelOperation(ctx, succeededOp.ID, tenantID, "itest-user", "too late"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cancel of succeeded operation: err = %v, want ErrInvalidState", err)
	}

	// Unknown IDs and foreign tenants both surface as not found.
	if _, err := store.CancelOperation(ctx, uuid.NewString(), tenantID, "itest-user", "ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel of unknown operation: err = %v, want ErrNotFound", err)
	}
	if _, err := store.CancelOperation(ctx, op.ID, otherTenant, "itest-user", "cross-tenant"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant cancel: err = %v, want ErrNotFound", err)
	}
}

func TestListOperationsTenantScopingFiltersAndPagination(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "list")
	otherTenant := integrationTenant(t, db, "list-other")
	store := NewPGStore(db)

	submit := func(operationType string) *Operation {
		t.Helper()
		op, _, err := store.SubmitOperation(ctx, submitCommand(tenantID, operationType,
			StepInput{Name: "step", StepType: operationType, MaxRetries: 1, TimeoutSeconds: 60},
		))
		if err != nil {
			t.Fatal(err)
		}
		return op
	}
	deployA := submit("deploy")
	deployB := submit("deploy")
	deleteOp := submit("delete")
	// An approved delete lands in queued for the status filter.
	if _, err := store.ApproveOperation(ctx, deleteOp.ID, tenantID, "itest-approver", "ok"); err != nil {
		t.Fatal(err)
	}
	otherOp, _, err := store.SubmitOperation(ctx, submitCommand(otherTenant, "deploy",
		StepInput{Name: "step", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
	))
	if err != nil {
		t.Fatal(err)
	}

	// Tenant filter is mandatory: only this tenant's rows are visible.
	summaries, total, err := store.ListOperations(ctx, ListQuery{TenantID: tenantID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(summaries) != 3 {
		t.Fatalf("tenant list: total=%d len=%d, want 3/3", total, len(summaries))
	}
	for _, s := range summaries {
		if s.TenantID != tenantID {
			t.Fatalf("list leaked operation %s of tenant %s", s.ID, s.TenantID)
		}
	}

	// Status filter.
	summaries, total, err = store.ListOperations(ctx, ListQuery{TenantID: tenantID, Status: StatusQueued, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(summaries) != 3 {
		t.Fatalf("status=queued list: total=%d len=%d, want 3/3", total, len(summaries))
	}

	// Operation type filter.
	summaries, total, err = store.ListOperations(ctx, ListQuery{TenantID: tenantID, OperationType: "deploy", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(summaries) != 2 {
		t.Fatalf("type=deploy list: total=%d len=%d, want 2/2", total, len(summaries))
	}
	seen := map[string]bool{}
	for _, s := range summaries {
		seen[s.ID] = true
	}
	if !seen[deployA.ID] || !seen[deployB.ID] {
		t.Fatalf("type=deploy list missing expected operations: %v", seen)
	}

	// limit/offset pagination over a stable created_at DESC ordering.
	page1, total, err := store.ListOperations(ctx, ListQuery{TenantID: tenantID, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page1) != 2 {
		t.Fatalf("page 1: total=%d len=%d, want 3/2", total, len(page1))
	}
	page2, _, err := store.ListOperations(ctx, ListQuery{TenantID: tenantID, Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Fatalf("page 2: len=%d, want 1", len(page2))
	}
	for _, a := range page1 {
		for _, b := range page2 {
			if a.ID == b.ID {
				t.Fatalf("operation %s appears on both pages", a.ID)
			}
		}
	}

	// Cross-tenant single-row reads must not find the operation.
	if _, err := store.GetOperation(ctx, otherOp.ID, tenantID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get: err = %v, want ErrNotFound", err)
	}
	if _, err := store.GetOperation(ctx, deployA.ID, otherTenant); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get (reverse): err = %v, want ErrNotFound", err)
	}
	got, err := store.GetOperation(ctx, deployA.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != deployA.ID || got.TenantID != tenantID {
		t.Fatalf("get returned wrong operation: %+v", got)
	}
}

func TestSubmitOperationConcurrentIdempotency(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "concurrent-idem")
	store := NewPGStore(db)

	sharedKey := "concurrent-itest-" + uuid.NewString()
	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "rel-" + uuid.NewString(),
		OperationType:  "deploy",
		IdempotencyKey: sharedKey,
		InitiatedBy:    "itest-user",
		CorrelationID:  uuid.NewString(),
		Tags:           map[string]string{"suite": "concurrent"},
		Steps: []StepInput{
			{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
		},
	}

	const goroutines = 20
	type result struct {
		op      *Operation
		created bool
		err     error
	}
	ch := make(chan result, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			op, created, err := store.SubmitOperation(ctx, cmd)
			ch <- result{op, created, err}
		}()
	}

	var firstID string
	createdCount := 0
	for i := 0; i < goroutines; i++ {
		r := <-ch
		if r.err != nil {
			t.Fatalf("concurrent submit: %v", r.err)
		}
		if r.op == nil {
			t.Fatal("concurrent submit returned nil operation")
		}
		if firstID == "" {
			firstID = r.op.ID
		}
		if r.op.ID != firstID {
			t.Fatalf("concurrent submit returned different operation IDs: %s vs %s", firstID, r.op.ID)
		}
		if r.created {
			createdCount++
		}
	}

	if createdCount != 1 {
		t.Fatalf("expected exactly 1 created=true, got %d", createdCount)
	}
}

func TestSubmitOperationConflictWithDifferentPayload(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := integrationTenant(t, db, "idem-conflict")
	store := NewPGStore(db)

	sharedKey := "conflict-itest-" + uuid.NewString()

	cmd1 := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "rel-1",
		OperationType:  "deploy",
		IdempotencyKey: sharedKey,
		InitiatedBy:    "itest-user",
		CorrelationID:  uuid.NewString(),
		Tags:           map[string]string{"suite": "conflict"},
		Steps: []StepInput{
			{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
		},
	}

	cmd2 := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "rel-2",
		OperationType:  "deploy",
		IdempotencyKey: sharedKey,
		InitiatedBy:    "itest-user",
		CorrelationID:  uuid.NewString(),
		Tags:           map[string]string{"suite": "conflict"},
		Steps: []StepInput{
			{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
		},
	}

	op1, created, err := store.SubmitOperation(ctx, cmd1)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if !created {
		t.Fatal("first submit should be created=true")
	}

	_, _, err = store.SubmitOperation(ctx, cmd2)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("second submit with different payload: err = %v, want ErrIdempotencyConflict", err)
	}

	// First operation must still exist and be unchanged.
	got, err := store.GetOperation(ctx, op1.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != op1.ID {
		t.Fatalf("operation ID changed: %s vs %s", got.ID, op1.ID)
	}
}

func TestSubmitOperationCrossTenantIsolation(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantA := integrationTenant(t, db, "cross-a")
	tenantB := integrationTenant(t, db, "cross-b")
	store := NewPGStore(db)

	sharedKey := "cross-itest-" + uuid.NewString()

	cmd := func(tenantID string) SubmitCommand {
		return SubmitCommand{
			TenantID:       tenantID,
			ReleaseID:      "rel-1",
			OperationType:  "deploy",
			IdempotencyKey: sharedKey,
			InitiatedBy:    "itest-user",
			CorrelationID:  uuid.NewString(),
			Tags:           map[string]string{"suite": "cross"},
			Steps: []StepInput{
				{Name: "apply", StepType: "deploy", MaxRetries: 1, TimeoutSeconds: 60},
			},
		}
	}

	opA, createdA, err := store.SubmitOperation(ctx, cmd(tenantA))
	if err != nil {
		t.Fatalf("tenant A submit: %v", err)
	}
	if !createdA {
		t.Fatal("tenant A first submit should be created=true")
	}

	opB, createdB, err := store.SubmitOperation(ctx, cmd(tenantB))
	if err != nil {
		t.Fatalf("tenant B submit: %v", err)
	}
	if !createdB {
		t.Fatal("tenant B first submit should be created=true (different tenant)")
	}

	if opA.ID == opB.ID {
		t.Fatal("cross-tenant operations should have different IDs")
	}
}

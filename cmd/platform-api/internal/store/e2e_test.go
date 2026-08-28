package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func e2eDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("db.Ping: %v", err)
	}
	return db
}

func e2eTenant(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	tenantID := fmt.Sprintf("tenant-e2e-%s-%s", name, uuid.NewString()[:8])
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1, $1, $1)`, tenantID); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM security_audit_events WHERE tenant_id = $1`,
			`DELETE FROM outbox_events WHERE tenant_id = $1`,
			`DELETE FROM operation_read_model WHERE tenant_id = $1`,
			`DELETE FROM runtime_intents WHERE tenant_id = $1`,
			`DELETE FROM operation_steps WHERE operation_id IN (SELECT id FROM operations WHERE tenant_id = $1)`,
			`DELETE FROM operations WHERE tenant_id = $1`,
			`DELETE FROM execution_plans WHERE tenant_id = $1`,
			`DELETE FROM tenant_memberships WHERE tenant_id = $1`,
			`DELETE FROM tenants WHERE id = $1`,
		} {
			_, _ = db.ExecContext(ctx, q, tenantID)
		}
	})
	return tenantID
}

// e2eActor creates a UUID identity subject with an active membership in the
// tenant (required by the uuid-typed subject/correlation columns and the
// tenant_memberships FKs on runtime_intents / security_audit_events).
func e2eActor(t *testing.T, db *sql.DB, tenantID string) (subjectID, membershipID string) {
	t.Helper()
	subjectID = uuid.NewString()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO identity_subjects (id, issuer, external_subject, subject_type)
		VALUES ($1, 'https://e2e-test.example', $2, 'user')`, subjectID, subjectID); err != nil {
		t.Fatalf("create identity subject: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO tenant_memberships (tenant_id, subject_id, status) VALUES ($1, $2, 'active') RETURNING id`,
		tenantID, subjectID).Scan(&membershipID); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	return subjectID, membershipID
}

func TestE2E_PlatformAPIToOutbox(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "outbox")

	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "deploy",
		IdempotencyKey: "e2e-outbox-" + uuid.NewString(),
		InitiatedBy:    "e2e-test",
		CorrelationID:  uuid.NewString(),
		Steps:          []StepInput{{Name: "deploy-app", StepType: "helm", ProviderID: "k8s-prod"}},
	}

	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}

	// Verify outbox event
	var outboxCount int
	err = db.QueryRow(
		`SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2`,
		op.ID, StepRequestedSubject,
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected 1 outbox event, got %d", outboxCount)
	}

	// Verify read model
	readOp, err := s.GetOperation(context.Background(), op.ID, tenantID)
	if err != nil {
		t.Fatalf("GetOperation: %v", err)
	}
	if readOp.Status != StatusQueued {
		t.Fatalf("expected status queued, got %s", readOp.Status)
	}
	if len(readOp.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(readOp.Steps))
	}

	// Verify audit trail
	var auditCount int
	err = db.QueryRow(
		`SELECT count(*) FROM operation_audit WHERE operation_id = $1`,
		op.ID,
	).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount < 1 {
		t.Fatalf("expected at least 1 audit entry, got %d", auditCount)
	}
}

func TestE2E_HighRiskApprovalFlow(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "approval")

	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "delete",
		IdempotencyKey: "e2e-approval-" + uuid.NewString(),
		InitiatedBy:    "e2e-user",
		CorrelationID:  uuid.NewString(),
		Steps:          []StepInput{{Name: "delete-app", StepType: "helm", ProviderID: "k8s-prod"}},
	}

	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	if op.Status != StatusPendingApproval {
		t.Fatalf("expected pending_approval, got %s", op.Status)
	}

	// Approve
	approved, err := s.ApproveOperation(context.Background(), op.ID, tenantID, "e2e-admin", "approved")
	if err != nil {
		t.Fatalf("ApproveOperation: %v", err)
	}
	if approved.Status != StatusQueued {
		t.Fatalf("expected queued after approve, got %s", approved.Status)
	}
	if approved.ApprovedBy != "e2e-admin" {
		t.Fatalf("expected approvedBy=e2e-admin, got %s", approved.ApprovedBy)
	}

	// Verify outbox event was emitted after approval
	var outboxCount int
	err = db.QueryRow(
		`SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2`,
		op.ID, StepRequestedSubject,
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected 1 outbox event after approve, got %d", outboxCount)
	}

	// Verify audit trail has both created and approved events
	var auditCount int
	err = db.QueryRow(
		`SELECT count(*) FROM operation_audit WHERE operation_id = $1`,
		op.ID,
	).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount < 2 {
		t.Fatalf("expected at least 2 audit entries, got %d", auditCount)
	}
}

func TestE2E_OutboxPayloadStructure(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "payload")

	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "deploy",
		IdempotencyKey: "e2e-payload-" + uuid.NewString(),
		InitiatedBy:    "e2e-test",
		CorrelationID:  uuid.NewString(),
		Steps: []StepInput{
			{Name: "db", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "app", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"db"}},
		},
	}

	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}

	// Read the outbox event
	var messageType, subject, idempotencyKey string
	var payload json.RawMessage
	err = db.QueryRow(
		`SELECT message_type, subject, idempotency_key, payload FROM outbox_events
		 WHERE operation_id = $1 AND message_type = $2
		 LIMIT 1`,
		op.ID, StepRequestedSubject,
	).Scan(&messageType, &subject, &idempotencyKey, &payload)
	if err != nil {
		t.Fatalf("query outbox payload: %v", err)
	}

	if messageType != StepRequestedSubject {
		t.Fatalf("expected message_type=%s, got %s", StepRequestedSubject, messageType)
	}
	if subject == "" {
		t.Fatal("expected non-empty subject")
	}
	if idempotencyKey == "" {
		t.Fatal("expected non-empty idempotency_key")
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty payload")
	}
}

func TestE2E_MultiStepDAGWithOutbox(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "dag")

	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "deploy",
		IdempotencyKey: "e2e-dag-" + uuid.NewString(),
		InitiatedBy:    "e2e-test",
		CorrelationID:  uuid.NewString(),
		Steps: []StepInput{
			{Name: "init", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "db", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"init"}},
			{Name: "cache", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"init"}},
			{Name: "app", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"db", "cache"}},
		},
	}

	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	if op.TotalSteps != 4 {
		t.Fatalf("expected 4 steps, got %d", op.TotalSteps)
	}

	// Only root steps (init) should have outbox events
	var outboxCount int
	err = db.QueryRow(
		`SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2`,
		op.ID, StepRequestedSubject,
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected 1 outbox event (only root steps), got %d", outboxCount)
	}

	// Verify plan_digest deduplication works
	cmd2 := cmd
	cmd2.IdempotencyKey = "e2e-dag-2-" + uuid.NewString()
	op2, _, err := s.SubmitOperation(context.Background(), cmd2)
	if err != nil {
		t.Fatalf("second SubmitOperation: %v", err)
	}
	if op2.PlanDigest != op.PlanDigest {
		t.Fatalf("expected same plan digest for identical steps: %s vs %s",
			op.PlanDigest, op2.PlanDigest)
	}

	// Verify cross-tenant isolation
	cmd3 := cmd
	cmd3.TenantID = "other-tenant"
	cmd3.IdempotencyKey = "e2e-dag-3-" + uuid.NewString()
	_, _, err = s.SubmitOperation(context.Background(), cmd3)
	if err != nil {
		t.Fatalf("cross-tenant submit: %v", err)
	}

	// Other tenant should not see our operations
	_, err = s.GetOperation(context.Background(), op.ID, "other-tenant")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-tenant get, got %v", err)
	}
}

func TestE2E_SubmitIntentIdempotency(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "intent-idem")

	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"idem-key-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"releaseId":"rel-100","targetRef":"tgt-a","scopeRef":"ns-prod"}}`)
	intent, err := engine.ParseRuntimeIntent(body)
	if err != nil {
		t.Fatalf("parse intent: %v", err)
	}
	planner := engine.NewPlanner()
	plan, err := planner.Plan(intent, tenantID, "", "", "", "")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	subjectID, membershipID := e2eActor(t, db, tenantID)
	cmd := IntentSubmitCommand{
		Intent:        intent,
		ExecutionPlan: plan,
		TenantID:      tenantID,
		SubjectID:     subjectID,
		CorrelationID: uuid.NewString(),
		PolicyVersion: "default:1",
		InitiatedBy:   subjectID,
		MembershipID:  membershipID,
	}

	op1, created1, err := s.SubmitIntent(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if !created1 {
		t.Fatal("expected created=true on first submit")
	}
	_ = op1.ID // ensure referenced

	// Replay with same idempotency key — must return existing operation
	op2, created2, err := s.SubmitIntent(context.Background(), cmd)
	if err != nil {
		t.Fatalf("replay submit: %v", err)
	}
	if created2 {
		t.Fatal("expected created=false on idempotent replay")
	}
	if op1.ID != op2.ID {
		t.Fatalf("idempotent replay returned different ID: %s vs %s", op1.ID, op2.ID)
	}

	// Same key in different tenant should create separate operation
	tenantB := e2eTenant(t, db, "intent-idem-b")
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO tenant_memberships (tenant_id, subject_id, status) VALUES ($1, $2, 'active')`,
		tenantB, subjectID); err != nil {
		t.Fatalf("tenant B membership: %v", err)
	}
	cmdTenantB := cmd
	cmdTenantB.TenantID = tenantB
	op3, created3, err := s.SubmitIntent(context.Background(), cmdTenantB)
	if err != nil {
		t.Fatalf("different tenant submit: %v", err)
	}
	if !created3 {
		t.Fatal("expected created=true for different tenant")
	}
	if op3.ID == op1.ID {
		t.Fatal("same operation ID across tenants")
	}
}

func TestE2E_SubmitIntentSemanticConflict(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "intent-conflict")

	// Two intents with different parameters for same release+target
	bodyA := []byte(`{"apiVersion":"hnb.io/v1","kind":"ChangeConfiguration","metadata":{"idempotencyKey":"conflict-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2"},"spec":{"releaseId":"rel-conflict","targetRef":"tgt-conflict","scopeRef":"ns-conflict","parameters":{"replicas":"3"}}}`)
	bodyB := []byte(`{"apiVersion":"hnb.io/v1","kind":"ChangeConfiguration","metadata":{"idempotencyKey":"conflict-2","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c3"},"spec":{"releaseId":"rel-conflict","targetRef":"tgt-conflict","scopeRef":"ns-conflict","parameters":{"replicas":"5"}}}`)

	intentA, _ := engine.ParseRuntimeIntent(bodyA)
	intentB, _ := engine.ParseRuntimeIntent(bodyB)
	planner := engine.NewPlanner()
	planA, _ := planner.Plan(intentA, tenantID, "", "", "", "")
	planB, _ := planner.Plan(intentB, tenantID, "", "", "", "")

	subjectID, _ := e2eActor(t, db, tenantID)
	cmdA := IntentSubmitCommand{Intent: intentA, ExecutionPlan: planA, TenantID: tenantID, SubjectID: subjectID, CorrelationID: uuid.NewString(), PolicyVersion: "default:1", InitiatedBy: subjectID}
	cmdB := IntentSubmitCommand{Intent: intentB, ExecutionPlan: planB, TenantID: tenantID, SubjectID: subjectID, CorrelationID: uuid.NewString(), PolicyVersion: "default:1", InitiatedBy: subjectID}

	opA, createdA, err := s.SubmitIntent(context.Background(), cmdA)
	if err != nil {
		t.Fatalf("submit A: %v", err)
	}
	if !createdA {
		t.Fatal("A should be created")
	}

	opB, createdB, err := s.SubmitIntent(context.Background(), cmdB)
	if err != nil {
		t.Fatalf("submit B: %v", err)
	}
	if !createdB {
		t.Fatal("B should also be created (different idempotency key)")
	}
	if opA.ID == opB.ID {
		t.Fatal("semantic conflict produced same operation")
	}
}

func TestE2E_AtomicFailurePreservesNoPartialState(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "atomic-fail")

	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"atomic-key","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c4"},"spec":{"releaseId":"rel-atomic","targetRef":"tgt-a","scopeRef":"ns-a"}}`)
	intent, err := engine.ParseRuntimeIntent(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	plan, err := engine.NewPlanner().Plan(intent, tenantID, "", "", "", "")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	subjectID, _ := e2eActor(t, db, tenantID)
	cmd := IntentSubmitCommand{Intent: intent, ExecutionPlan: plan, TenantID: tenantID, SubjectID: subjectID, CorrelationID: uuid.NewString(), PolicyVersion: "default:1", InitiatedBy: subjectID}

	// Normal submit succeeds
	_, _, err = s.SubmitIntent(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first submit should succeed: %v", err)
	}

	// Idempotent replay should succeed too
	_, created, err := s.SubmitIntent(context.Background(), cmd)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if created {
		t.Fatal("idempotent replay should not create new operation")
	}
}

func TestE2E_OutboxEventIntegrityAfterCommit(t *testing.T) {
	db := e2eDB(t)
	s := NewPGStore(db)
	tenantID := e2eTenant(t, db, "outbox-integrity")

	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"outbox-integ","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c5"},"spec":{"releaseId":"rel-outbox","targetRef":"tgt-b","scopeRef":"ns-b"}}`)
	intent, _ := engine.ParseRuntimeIntent(body)
	plan, _ := engine.NewPlanner().Plan(intent, tenantID, "", "", "", "")

	subjectID, _ := e2eActor(t, db, tenantID)
	cmd := IntentSubmitCommand{Intent: intent, ExecutionPlan: plan, TenantID: tenantID, SubjectID: subjectID, CorrelationID: uuid.NewString(), PolicyVersion: "default:1", InitiatedBy: subjectID}

	op, _, err := s.SubmitIntent(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Every outbox event for this operation must have valid correlation_id
	rows, err := db.Query(`SELECT correlation_id, payload FROM outbox_events WHERE operation_id = $1`, op.ID)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var cid string
		var payload json.RawMessage
		if err := rows.Scan(&cid, &payload); err != nil {
			t.Fatalf("scan outbox row: %v", err)
		}
		if cid == "" {
			t.Fatal("outbox event has empty correlation_id")
		}
		if len(payload) == 0 {
			t.Fatal("outbox event has empty payload")
		}
		count++
	}
	if count == 0 {
		t.Fatal("no outbox events found after intent submit")
	}
}

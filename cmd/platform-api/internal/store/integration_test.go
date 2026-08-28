package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
)

func openPGIntDB(t *testing.T) *sql.DB {
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

func itenant(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	tenantID := fmt.Sprintf("tenant-pgint-%s-%s", name, uuid.NewString()[:8])
	if _, err := db.Exec(`
		INSERT INTO tenants (id, name, display_name)
		VALUES ($1, $1, $1)`, tenantID); err != nil {
		t.Fatalf("create integration tenant: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.ExecContext(ctx, "DELETE FROM outbox_events WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operation_read_model WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM operations WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM execution_plans WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM runtime_targets WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM workspaces WHERE tenant_id = $1", tenantID)
		_, _ = db.ExecContext(ctx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})
	return tenantID
}

func isteps(names ...string) []StepInput {
	s := make([]StepInput, len(names))
	for i, n := range names {
		s[i] = StepInput{Name: n, StepType: "helm", ProviderID: "k8s-prod"}
	}
	return s
}

func TestPGStore_Ping(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPGStore_SubmitOperation_lowRisk(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "lowrisk")

	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "deploy",
		IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy:    "user-1",
		Steps:          isteps("deploy-app"),
	}

	op, created, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first submit")
	}
	if op.Status != StatusQueued {
		t.Fatalf("expected status=queued for low-risk type, got %s", op.Status)
	}
	if op.TenantID != tenantID {
		t.Fatalf("expected tenantId=%s, got %s", tenantID, op.TenantID)
	}
	if op.TotalSteps != 1 {
		t.Fatalf("expected totalSteps=1, got %d", op.TotalSteps)
	}
	if op.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
	if len(op.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(op.Steps))
	}
	if op.Steps[0].Status != "pending" {
		t.Fatalf("expected step status=pending, got %s", op.Steps[0].Status)
	}
}

func TestPGStore_provisionRuntimeTargetTx(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "provision")
	ctx := context.Background()

	newCmd := func(kind engine.IntentKind, targetID, targetType, connType, state, source string) IntentSubmitCommand {
		return IntentSubmitCommand{
			Intent:        &engine.RuntimeIntent{Kind: kind},
			TenantID:      tenantID,
			CorrelationID: "corr-test",
			ProvisionTarget: &ProvisionRuntimeTarget{
				TargetID: targetID, Name: targetID, DisplayName: "cluster-a",
				TargetType: targetType, ConnectionType: connType,
				LifecycleState: state, Source: source,
			},
		}
	}

	run := func(cmd IntentSubmitCommand) {
		t.Helper()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := s.provisionRuntimeTargetTx(ctx, tx, cmd, "intent-1"); err != nil {
			tx.Rollback()
			t.Fatalf("provisionRuntimeTargetTx: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	// Create (PROVISIONING, kubernetes)
	createID := uuid.New().String()
	createCmd := newCmd(engine.IntentCreateKubernetesTarget, createID, "kubernetes", "agent", "PROVISIONING", "created")
	createCmd.Intent.Spec.CredentialSecretRef = &engine.SecretReferenceEntry{
		Provider: "local-aes", Scope: tenantID, Name: "cluster-a-credential",
	}
	run(createCmd)
	// Import (REGISTERING, edge)
	importID := uuid.New().String()
	run(newCmd(engine.IntentImportRuntimeTarget, importID, "edge_runtime", "cloudhub", "REGISTERING", "imported"))
	// Idempotent re-provision of the same target must not fail.
	run(newCmd(engine.IntentCreateKubernetesTarget, createID, "kubernetes", "agent", "PROVISIONING", "created"))

	var state, name, dispName, targetType, connType string
	var credentialRefJSON []byte
	err := db.QueryRowContext(ctx,
		`SELECT lifecycle_state, name, COALESCE(display_name,''), target_type, connection_type, credential_ref FROM runtime_targets WHERE id=$1 AND tenant_id=$2`,
		createID, tenantID).Scan(&state, &name, &dispName, &targetType, &connType, &credentialRefJSON)
	if err != nil {
		t.Fatalf("lookup created target: %v", err)
	}
	if state != "PROVISIONING" || targetType != "kubernetes" || connType != "agent" || dispName != "cluster-a" {
		t.Fatalf("unexpected created target row: state=%s type=%s conn=%s disp=%q", state, targetType, connType, dispName)
	}
	if name == "" {
		t.Fatal("expected non-empty name")
	}

	// The credential reference from the create intent must be persisted on the
	// target for exact kubeconfig resolution.
	var ref struct {
		Provider string `json:"provider"`
		Scope    string `json:"scope"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(credentialRefJSON, &ref); err != nil {
		t.Fatalf("decode credential_ref: %v (raw=%s)", err, credentialRefJSON)
	}
	if ref.Name != "cluster-a-credential" || ref.Scope != tenantID || ref.Provider != "local-aes" {
		t.Fatalf("unexpected credential_ref: %+v", ref)
	}

	// Imported target carried no credential reference → column stays NULL.
	var importRefJSON []byte
	if err := db.QueryRowContext(ctx,
		`SELECT credential_ref FROM runtime_targets WHERE id=$1 AND tenant_id=$2`, importID, tenantID).
		Scan(&importRefJSON); err != nil {
		t.Fatalf("lookup imported target credential_ref: %v", err)
	}
	if importRefJSON != nil {
		t.Fatalf("expected NULL credential_ref for imported target, got %s", importRefJSON)
	}

	var eState, eType string
	err = db.QueryRowContext(ctx,
		`SELECT lifecycle_state, target_type FROM runtime_targets WHERE id=$1 AND tenant_id=$2`, importID, tenantID).
		Scan(&eState, &eType)
	if err != nil {
		t.Fatalf("lookup imported target: %v", err)
	}
	if eState != "REGISTERING" || eType != "edge_runtime" {
		t.Fatalf("unexpected imported target row: state=%s type=%s", eState, eType)
	}
}

func TestPGStore_SubmitOperation_highRisk(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "highrisk")

	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "delete",
		IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy:    "user-2",
		Steps:          isteps("delete-app"),
	}

	op, created, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation high-risk: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first submit")
	}
	if op.Status != StatusPendingApproval {
		t.Fatalf("expected status=pending_approval for high-risk type, got %s", op.Status)
	}
}

func TestPGStore_SubmitOperation_idempotent(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "idempotent")

	key := "ikey-" + uuid.NewString()
	cmd := SubmitCommand{
		TenantID:       tenantID,
		ReleaseID:      "release-" + uuid.NewString(),
		OperationType:  "deploy",
		IdempotencyKey: key,
		InitiatedBy:    "user-3",
		Steps:          isteps("step-1"),
	}

	op1, created1, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if !created1 {
		t.Fatal("expected created=true on first submit")
	}

	op2, created2, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second submit (idempotent): %v", err)
	}
	if created2 {
		t.Fatal("expected created=false on idempotent replay")
	}
	if op1.ID != op2.ID {
		t.Fatalf("expected same operation ID on idempotent replay: %s vs %s", op1.ID, op2.ID)
	}
}

func TestPGStore_SubmitOperation_differentTenantSameKey(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantA := itenant(t, db, "diffkey-a")
	tenantB := itenant(t, db, "diffkey-b")

	keyA := "ikey-" + uuid.NewString()
	keyB := "ikey-" + uuid.NewString()
	cmdA := SubmitCommand{
		TenantID: tenantA, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: keyA, InitiatedBy: "user-1",
		Steps: isteps("s1"),
	}
	cmdB := SubmitCommand{
		TenantID: tenantB, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: keyB, InitiatedBy: "user-2",
		Steps: isteps("s1"),
	}

	opA, createdA, err := s.SubmitOperation(context.Background(), cmdA)
	if err != nil {
		t.Fatalf("tenant A submit: %v", err)
	}
	if !createdA {
		t.Fatal("expected created=true for tenant A")
	}
	opB, createdB, err := s.SubmitOperation(context.Background(), cmdB)
	if err != nil {
		t.Fatalf("tenant B submit: %v", err)
	}
	if !createdB {
		t.Fatal("expected created=true for tenant B (different key)")
	}
	if opA.ID == opB.ID {
		t.Fatal("expected different operation IDs for different tenants")
	}
}

func TestPGStore_ApproveOperation(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "approve")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "delete", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user-4", Steps: isteps("del-app"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if op.Status != StatusPendingApproval {
		t.Fatalf("expected pending_approval, got %s", op.Status)
	}

	approved, err := s.ApproveOperation(context.Background(), op.ID, tenantID, "admin-1", "approved")
	if err != nil {
		t.Fatalf("ApproveOperation: %v", err)
	}
	if approved.Status != StatusQueued {
		t.Fatalf("expected status=queued after approve, got %s", approved.Status)
	}
	if approved.ApprovedBy != "admin-1" {
		t.Fatalf("expected approvedBy=admin-1, got %s", approved.ApprovedBy)
	}
}

func TestPGStore_RejectOperation(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "reject")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "delete", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user-5", Steps: isteps("del-app"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	rejected, err := s.RejectOperation(context.Background(), op.ID, tenantID, "admin-1", "not approved")
	if err != nil {
		t.Fatalf("RejectOperation: %v", err)
	}
	if rejected.Status != StatusCancelled {
		t.Fatalf("expected status=cancelled after reject, got %s", rejected.Status)
	}
}

func TestPGStore_CancelOperation(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "cancel")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user-6", Steps: isteps("deploy-app"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if op.Status != StatusQueued {
		t.Fatalf("expected queued, got %s", op.Status)
	}

	cancelled, err := s.CancelOperation(context.Background(), op.ID, tenantID, "user-6", "cancelled")
	if err != nil {
		t.Fatalf("CancelOperation: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("expected status=cancelled, got %s", cancelled.Status)
	}
}

func TestPGStore_CancelOperation_invalidState(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "cancel-invalid")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user-7", Steps: isteps("deploy-app"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	_, err = s.CancelOperation(context.Background(), op.ID, tenantID, "user-7", "first")
	if err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	_, err = s.CancelOperation(context.Background(), op.ID, tenantID, "user-7", "second")
	if err == nil {
		t.Fatal("expected error on cancelling already-cancelled operation")
	}
}

func TestPGStore_GetOperation(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "getop")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user-8",
		Steps: []StepInput{
			{Name: "step-1", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "step-2", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"step-1"}},
		},
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	got, err := s.GetOperation(context.Background(), op.ID, tenantID)
	if err != nil {
		t.Fatalf("GetOperation: %v", err)
	}
	if got.ID != op.ID {
		t.Fatalf("expected ID %s, got %s", op.ID, got.ID)
	}
	if got.Status != StatusQueued {
		t.Fatalf("expected status queued, got %s", got.Status)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(got.Steps))
	}
	if got.LastStateChangedAt.IsZero() {
		t.Fatal("expected non-zero last_state_changed_at")
	}
}

func TestPGStore_GetOperation_crossTenantIsolation(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "cross-tenant")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user-9", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	_, err = s.GetOperation(context.Background(), op.ID, "other-tenant")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-tenant access, got %v", err)
	}
}

func TestPGStore_ListOperations(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "list")

	for range 3 {
		cmd := SubmitCommand{
			TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
			OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
			InitiatedBy: "user-list", Steps: isteps("s1"),
		}
		if _, _, err := s.SubmitOperation(context.Background(), cmd); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}

	summaries, total, err := s.ListOperations(context.Background(), ListQuery{
		TenantID: tenantID, Limit: 50, Offset: 0,
	})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}
}

func TestPGStore_ListOperations_filterByStatus(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "list-filter")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	_, err = s.CancelOperation(context.Background(), op.ID, tenantID, "user", "cancel")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	summaries, total, err := s.ListOperations(context.Background(), ListQuery{
		TenantID: tenantID, Status: StatusCancelled, Limit: 50, Offset: 0,
	})
	if err != nil {
		t.Fatalf("ListOperations filtered: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 cancelled operation, got %d", total)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Status != StatusCancelled {
		t.Fatalf("expected status=cancelled, got %s", summaries[0].Status)
	}
}

func TestPGStore_ListOperations_pagination(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "list-page")

	for range 5 {
		cmd := SubmitCommand{
			TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
			OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
			InitiatedBy: "user", Steps: isteps("s1"),
		}
		if _, _, err := s.SubmitOperation(context.Background(), cmd); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}

	s1, total1, err := s.ListOperations(context.Background(), ListQuery{
		TenantID: tenantID, Limit: 2, Offset: 0,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total1 != 5 {
		t.Fatalf("expected total=5, got %d", total1)
	}
	if len(s1) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(s1))
	}

	s2, total2, err := s.ListOperations(context.Background(), ListQuery{
		TenantID: tenantID, Limit: 2, Offset: 2,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if total2 != 5 {
		t.Fatalf("expected total=5, got %d", total2)
	}
	if len(s2) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(s2))
	}
}

func TestPGStore_SubmitOperation_withTags(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "tags")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Tags: map[string]string{"env": "prod", "team": "platform"},
		Steps: isteps("deploy-app"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	var tagsJSON []byte
	err = db.QueryRow(`SELECT tags FROM operations WHERE id = $1`, op.ID).Scan(&tagsJSON)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	var tags map[string]string
	if err := json.Unmarshal(tagsJSON, &tags); err != nil {
		t.Fatalf("unmarshal tags: %v", err)
	}
	if tags["env"] != "prod" || tags["team"] != "platform" {
		t.Fatalf("expected tags {env:prod, team:platform}, got %v", tags)
	}
}

func TestPGStore_SubmitOperation_withSecretReference(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "secretref")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user",
		Steps: []StepInput{
			{Name: "deploy-with-secret", StepType: "helm", ProviderID: "k8s-prod",
				SecretReference: "ref://secrets/db-password",
				Inputs:          map[string]string{"foo": "bar"}},
		},
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	if len(op.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(op.Steps))
	}
}

func TestPGStore_SubmitOperation_withDependencies(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "deps")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user",
		Steps: []StepInput{
			{Name: "db", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "cache", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "app", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"db", "cache"}},
		},
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	if op.TotalSteps != 3 {
		t.Fatalf("expected 3 steps, got %d", op.TotalSteps)
	}
}

func TestPGStore_SubmitOperation_planReuse(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "planreuse")

	steps := isteps("deploy-app")
	cmd1 := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: steps,
	}
	cmd2 := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: steps,
	}

	op1, _, err := s.SubmitOperation(context.Background(), cmd1)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	op2, _, err := s.SubmitOperation(context.Background(), cmd2)
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if op1.PlanDigest != op2.PlanDigest {
		t.Fatalf("expected same plan digest: %s vs %s", op1.PlanDigest, op2.PlanDigest)
	}
}

func TestPGStore_CreateRuntimeTarget(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "rt-create")

	rt := &RuntimeTarget{
		TenantID: tenantID, Name: "k8s-prod", TargetType: "kubernetes",
	}
	if err := s.CreateRuntimeTarget(context.Background(), rt); err != nil {
		t.Fatalf("CreateRuntimeTarget: %v", err)
	}
	if rt.ID == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestPGStore_GetRuntimeTarget(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "rt-get")

	rt := &RuntimeTarget{
		TenantID: tenantID, Name: "k8s-staging", TargetType: "kubernetes",
		Labels: map[string]string{"env": "staging"},
	}
	if err := s.CreateRuntimeTarget(context.Background(), rt); err != nil {
		t.Fatalf("CreateRuntimeTarget: %v", err)
	}

	got, err := s.GetRuntimeTarget(context.Background(), rt.ID, rt.TenantID)
	if err != nil {
		t.Fatalf("GetRuntimeTarget: %v", err)
	}
	if got.Name != "k8s-staging" {
		t.Fatalf("expected name k8s-staging, got %s", got.Name)
	}
	if got.TargetType != "kubernetes" {
		t.Fatalf("expected targetType kubernetes, got %s", got.TargetType)
	}
	if got.Labels["env"] != "staging" {
		t.Fatalf("expected label env=staging, got %s", got.Labels["env"])
	}
}

func TestPGStore_ListRuntimeTargets(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "rt-list")

	for _, name := range []string{"k8s-1", "k8s-2"} {
		rt := &RuntimeTarget{
			TenantID: tenantID, Name: name, TargetType: "kubernetes",
		}
		if err := s.CreateRuntimeTarget(context.Background(), rt); err != nil {
			t.Fatalf("CreateRuntimeTarget %s: %v", name, err)
		}
	}

	targets, err := s.ListRuntimeTargets(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("ListRuntimeTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
}

func TestPGStore_DeleteRuntimeTarget(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "rt-delete")

	rt := &RuntimeTarget{
		TenantID: tenantID, Name: "to-delete", TargetType: "kubernetes",
	}
	if err := s.CreateRuntimeTarget(context.Background(), rt); err != nil {
		t.Fatalf("CreateRuntimeTarget: %v", err)
	}
	if err := s.DeleteRuntimeTarget(context.Background(), rt.ID, rt.TenantID); err != nil {
		t.Fatalf("DeleteRuntimeTarget: %v", err)
	}
	_, err := s.GetRuntimeTarget(context.Background(), rt.ID, rt.TenantID)
	if err != ErrTargetNotFound {
		t.Fatalf("expected ErrTargetNotFound after delete, got %v", err)
	}
}

func TestPGStore_UpdateRuntimeTargetStatus(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "rt-status")

	rt := &RuntimeTarget{
		TenantID: tenantID, Name: "k8s-with-status", TargetType: "kubernetes",
	}
	if err := s.CreateRuntimeTarget(context.Background(), rt); err != nil {
		t.Fatalf("CreateRuntimeTarget: %v", err)
	}

	now := time.Now().UTC()
	if err := s.UpdateRuntimeTargetStatus(context.Background(), rt.ID, rt.TenantID, "online", now); err != nil {
		t.Fatalf("UpdateRuntimeTargetStatus: %v", err)
	}
	got, err := s.GetRuntimeTarget(context.Background(), rt.ID, rt.TenantID)
	if err != nil {
		t.Fatalf("GetRuntimeTarget: %v", err)
	}
	if got.Status != "online" {
		t.Fatalf("expected status=online, got %s", got.Status)
	}
}

func TestPGStore_RuntimeTargetTenantPredicates(t *testing.T) {
	db := openPGIntDB(t)
	ctx := context.Background()
	tenantA, tenantB := "target-a-"+uuid.NewString(), "target-b-"+uuid.NewString()
	workspaceA := uuid.NewString()
	targetID := uuid.NewString()
	for _, tenantID := range []string{tenantA, tenantB} {
		if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, targetID)
		_, _ = db.ExecContext(ctx, `DELETE FROM workspaces WHERE id=$1`, workspaceA)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id IN ($1,$2)`, tenantA, tenantB)
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES ($1,$2,'workspace')`, workspaceA, tenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime_targets (id, tenant_id, workspace_id, name, target_type) VALUES ($1,$2,$3,'target','kubernetes')`, targetID, tenantA, workspaceA); err != nil {
		t.Fatal(err)
	}

	store := NewPGStore(db)
	if _, err := store.GetRuntimeTarget(ctx, targetID, tenantB); !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("foreign get error = %v", err)
	}
	if err := store.UpdateRuntimeTargetStatus(ctx, targetID, tenantB, "online", time.Now()); !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("foreign update error = %v", err)
	}
	if err := store.DeleteRuntimeTarget(ctx, targetID, tenantB); !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("foreign delete error = %v", err)
	}
	target, err := store.GetRuntimeTarget(ctx, targetID, tenantA)
	if err != nil || target.Status != "unknown" {
		t.Fatalf("foreign mutation changed target: %+v, %v", target, err)
	}
}

func TestPGStore_SubmitOperation_correlationID(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "corr")

	correlationID := uuid.NewString()
	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", CorrelationID: correlationID,
		Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	var persisted string
	err = db.QueryRow(`SELECT correlation_id FROM operations WHERE id = $1`, op.ID).Scan(&persisted)
	if err != nil {
		t.Fatalf("query correlation_id: %v", err)
	}
	if persisted != correlationID {
		t.Fatalf("expected correlation_id %s, got %s", correlationID, persisted)
	}
}

func TestPGStore_SubmitOperation_autoGenerateCorrelationID(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "auto-corr")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitOperation: %v", err)
	}
	var persisted string
	err = db.QueryRow(`SELECT correlation_id FROM operations WHERE id = $1`, op.ID).Scan(&persisted)
	if err != nil {
		t.Fatalf("query correlation_id: %v", err)
	}
	if persisted == "" {
		t.Fatal("expected auto-generated correlation_id")
	}
}

func TestPGStore_ApproveOperation_nonExistent(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	_, err := s.ApproveOperation(context.Background(), uuid.NewString(), "tenant-x", "admin", "test")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPGStore_CancelOperation_nonExistent(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	_, err := s.CancelOperation(context.Background(), uuid.NewString(), "tenant-x", "user", "test")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPGStore_ApproveOperation_wrongTenant(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "wrong-tenant-approve")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "delete", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	_, err = s.ApproveOperation(context.Background(), op.ID, "other-tenant", "admin", "should fail")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-tenant approve, got %v", err)
	}
}

func TestPGStore_SubmitOperation_outboxEvents(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "outbox")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	var count int
	err = db.QueryRow(
		`SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2`,
		op.ID, StepRequestedSubject,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 step-requested outbox event, got %d", count)
	}
}

func TestPGStore_ApproveOperation_outboxEvents(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "approve-outbox")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "delete", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	_, err = s.ApproveOperation(context.Background(), op.ID, tenantID, "admin", "approve")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	var count int
	err = db.QueryRow(
		`SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2`,
		op.ID, StepRequestedSubject,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 step-requested outbox event after approve, got %d", count)
	}
}

func TestPGStore_SubmitOperation_multiStepOutbox(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "multi-outbox")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "deploy", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user",
		Steps: []StepInput{
			{Name: "db", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "cache", StepType: "helm", ProviderID: "k8s-prod"},
			{Name: "app", StepType: "helm", ProviderID: "k8s-prod", DependsOn: []string{"db", "cache"}},
		},
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	var count int
	err = db.QueryRow(
		`SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2`,
		op.ID, StepRequestedSubject,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 step-requested events (only ready steps), got %d", count)
	}
}

func TestPGStore_operationAudit(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "audit")

	cmd := SubmitCommand{
		TenantID: tenantID, ReleaseID: "release-" + uuid.NewString(),
		OperationType: "delete", IdempotencyKey: "ikey-" + uuid.NewString(),
		InitiatedBy: "user", Steps: isteps("s1"),
	}
	op, _, err := s.SubmitOperation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	_, err = s.ApproveOperation(context.Background(), op.ID, tenantID, "admin", "approved")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	var auditCount int
	err = db.QueryRow(
		`SELECT count(*) FROM operation_audit WHERE operation_id = $1`, op.ID,
	).Scan(&auditCount)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount < 2 {
		t.Fatalf("expected at least 2 audit entries (created + approved), got %d", auditCount)
	}
}

func TestPGStore_HeartbeatDoesNotIncrementVersion(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "hbversion")

	cluster, err := s.CreateCluster(context.Background(), CreateClusterRequest{
		Name:        "hb-test",
		TenantID:    tenantID,
		ClusterType: "kubernetes",
		APIEndpoint: "https://example.com:6443",
	})
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if cluster.Version != 1 {
		t.Fatalf("expected initial version=1, got %d", cluster.Version)
	}

	for i := 0; i < 10; i++ {
		if err := s.HeartbeatCluster(context.Background(), cluster.ID, tenantID); err != nil {
			t.Fatalf("heartbeat %d: %v", i, err)
		}
	}

	got, err := s.GetCluster(context.Background(), cluster.ID, tenantID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("expected version=1 after 10 heartbeats, got %d; heartbeats must not increment version", got.Version)
	}
}

func TestPGStore_UpdateClusterPointerFields(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "patch-cluster")

	cluster, err := s.CreateCluster(context.Background(), CreateClusterRequest{
		Name:        "patch-test",
		TenantID:    tenantID,
		ClusterType: "kubernetes",
		APIEndpoint: "https://example.com:6443",
		Region:      "cn-east",
		Zone:        "zone-a",
	})
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	// nil field → no change
	updated, err := s.UpdateCluster(context.Background(), cluster.ID, tenantID, UpdateClusterRequest{})
	if err != nil {
		t.Fatalf("UpdateCluster with nil fields: %v", err)
	}
	if updated.Region != "cn-east" {
		t.Fatalf("nil Region should not change: got %q, want cn-east", updated.Region)
	}
	if updated.Zone != "zone-a" {
		t.Fatalf("nil Zone should not change: got %q, want zone-a", updated.Zone)
	}
	if updated.Status != "pending" {
		t.Fatalf("nil Status should not change: got %q, want pending", updated.Status)
	}

	// ptr("new-value") → update
	region := "cn-west"
	status := "healthy"
	updated, err = s.UpdateCluster(context.Background(), cluster.ID, tenantID, UpdateClusterRequest{
		Region: &region,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("UpdateCluster with new values: %v", err)
	}
	if updated.Region != "cn-west" {
		t.Fatalf("Region should be cn-west, got %q", updated.Region)
	}
	if updated.Status != "healthy" {
		t.Fatalf("Status should be healthy, got %q", updated.Status)
	}
	if updated.Zone != "zone-a" {
		t.Fatalf("Zone should be unchanged (zone-a), got %q", updated.Zone)
	}

	// ptr("") → clear
	empty := ""
	updated, err = s.UpdateCluster(context.Background(), cluster.ID, tenantID, UpdateClusterRequest{
		Region: &empty,
		Zone:   &empty,
	})
	if err != nil {
		t.Fatalf("UpdateCluster with empty fields: %v", err)
	}
	if updated.Region != "" {
		t.Fatalf("Region should be empty, got %q", updated.Region)
	}
	if updated.Zone != "" {
		t.Fatalf("Zone should be empty, got %q", updated.Zone)
	}

	// Optimistic version continuity: create starts at 1 and every accepted
	// update bumps it by exactly one (3 updates above → version 4). The
	// UPDATE ... WHERE version = ? clause is what enforces this against
	// concurrent writers.
	got, err := s.GetCluster(context.Background(), cluster.ID, tenantID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if got.Version != 4 {
		t.Fatalf("expected version=4 after 3 updates, got %d", got.Version)
	}

	// cross-tenant: wrong tenant fails
	_, err = s.UpdateCluster(context.Background(), cluster.ID, "other-tenant", UpdateClusterRequest{
		Region: &region,
	})
	if err == nil {
		t.Fatal("expected error for cross-tenant update")
	}
}

func TestPGStore_HeartbeatAtomicity(t *testing.T) {
	db := openPGIntDB(t)
	s := NewPGStore(db)
	tenantID := itenant(t, db, "hb-atomic")

	cluster, err := s.CreateCluster(context.Background(), CreateClusterRequest{
		Name:        "hb-atomic",
		TenantID:    tenantID,
		ClusterType: "kubernetes",
		APIEndpoint: "https://example.com:6443",
	})
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	// Normal heartbeat succeeds.
	if err := s.HeartbeatCluster(context.Background(), cluster.ID, tenantID); err != nil {
		t.Fatalf("first heartbeat: %v", err)
	}

	// Heartbeat with wrong tenant returns ErrClusterNotFound.
	if err := s.HeartbeatCluster(context.Background(), cluster.ID, "other-tenant"); err != ErrClusterNotFound {
		t.Fatalf("cross-tenant heartbeat: err = %v, want ErrClusterNotFound", err)
	}

	// Check cluster status is still healthy (no partial state from the failed heartbeat).
	got, err := s.GetCluster(context.Background(), cluster.ID, tenantID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if got.Status != "healthy" {
		t.Fatalf("expected status=healthy after heartbeat, got %q", got.Status)
	}
}

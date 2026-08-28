package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

func TestSubmitIntentConcurrentCommitment(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := "intent-commitment-" + uuid.NewString()
	subjectID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id,name,display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO identity_subjects (id,issuer,external_subject,subject_type) VALUES ($1,$2,$3,'user')`, subjectID, "https://intent-test.example", subjectID); err != nil {
		t.Fatal(err)
	}
	var membershipID string
	if err := db.QueryRowContext(ctx, `INSERT INTO tenant_memberships (tenant_id,subject_id,status) VALUES ($1,$2,'active') RETURNING id`, tenantID, subjectID).Scan(&membershipID); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"concurrent-commitment","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"releaseId":"release-a","targetRef":"target-a","scopeRef":"scope-a"}}`)
	intent, err := engine.ParseRuntimeIntent(body)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := engine.NewPlanner().Plan(intent, tenantID, "", "", "", subjectID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := IntentSubmitCommand{
		Intent: intent, ExecutionPlan: plan, TenantID: tenantID, SubjectID: subjectID,
		CorrelationID: intent.Metadata.CorrelationID, InitiatedBy: subjectID, CommitmentAction: "create",
		MembershipID: membershipID, ServiceSubject: "hnb-apiserver", DelegationTokenID: "delegation-jti",
		DelegationKeyID: "delegation-key", TraceID: "11111111111111111111111111111111",
		AuthorizationScope: iam.DelegationScope{ResourceKind: "cluster", ProjectID: "project-a"},
	}

	const workers = 8
	type result struct {
		op      *Operation
		created bool
		err     error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op, created, err := NewPGStore(db).SubmitIntent(ctx, cmd)
			results <- result{op: op, created: created, err: err}
		}()
	}
	wg.Wait()
	close(results)
	createdCount := 0
	var intentID, planID, operationID string
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent submit: %v", result.err)
		}
		if result.created {
			createdCount++
		}
		if intentID == "" {
			intentID, planID, operationID = result.op.IntentID, result.op.PlanID, result.op.ID
		}
		if result.op.IntentID != intentID || result.op.PlanID != planID || result.op.ID != operationID {
			t.Fatalf("commitment IDs diverged: %+v", result.op)
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count=%d, want 1", createdCount)
	}
	for table, want := range map[string]int{"runtime_intents": 1, "execution_plans": 1, "operations": 1, "security_audit_events": 1, "operation_read_model": 1} {
		if got := countWhere(t, db, "SELECT count(*) FROM "+table+" WHERE tenant_id=$1", tenantID); got != want {
			t.Fatalf("%s rows=%d want=%d", table, got, want)
		}
	}
	if got := countWhere(t, db, "SELECT count(*) FROM operation_steps WHERE operation_id=$1", operationID); got != len(plan.Steps) {
		t.Fatalf("operation_steps rows=%d want=%d", got, len(plan.Steps))
	}
	rootSteps := 0
	for _, step := range plan.Steps {
		if len(step.DependsOn) == 0 {
			rootSteps++
		}
	}
	if got := countWhere(t, db, "SELECT count(*) FROM outbox_events WHERE tenant_id=$1", tenantID); got != rootSteps {
		t.Fatalf("outbox rows=%d want=%d", got, rootSteps)
	}
	if got := countWhere(t, db, "SELECT count(*) FROM operation_audit WHERE operation_id=$1", operationID); got != 1 {
		t.Fatalf("operation_audit rows=%d want=1", got)
	}
	var serviceSubject, actorMembership, delegationTokenID, delegationKeyID, action, traceID, scope string
	if err := db.QueryRowContext(ctx, `
		SELECT service_subject, actor_membership_id, delegation_token_id, delegation_key_id,
		       action, trace_id, scope::text
		FROM security_audit_events WHERE tenant_id=$1`, tenantID).Scan(
		&serviceSubject, &actorMembership, &delegationTokenID, &delegationKeyID, &action, &traceID, &scope,
	); err != nil {
		t.Fatal(err)
	}
	if serviceSubject != "hnb-apiserver" || actorMembership != membershipID || delegationTokenID != "delegation-jti" ||
		delegationKeyID != "delegation-key" || action != "create" || traceID != "11111111111111111111111111111111" ||
		!strings.Contains(scope, `"resourceKind": "cluster"`) || !strings.Contains(scope, `"projectId": "project-a"`) {
		t.Fatalf("unexpected delegated audit evidence: service=%q membership=%q jti=%q kid=%q action=%q trace=%q scope=%s",
			serviceSubject, actorMembership, delegationTokenID, delegationKeyID, action, traceID, scope)
	}
}

func TestSubmitIntentConcurrentSemanticConflict(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := "intent-conflict-" + uuid.NewString()
	subjectID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id,name,display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO identity_subjects (id,issuer,external_subject,subject_type) VALUES ($1,$2,$3,'user')`, subjectID, "https://intent-test.example", subjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tenant_memberships (tenant_id,subject_id,status) VALUES ($1,$2,'active')`, tenantID, subjectID); err != nil {
		t.Fatal(err)
	}
	makeCommand := func(target string) IntentSubmitCommand {
		body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"concurrent-conflict"},"spec":{"releaseId":"release-a","targetRef":"` + target + `","scopeRef":"scope-a"}}`)
		intent, err := engine.ParseRuntimeIntent(body)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := engine.NewPlanner().Plan(intent, tenantID, "", "", "", subjectID)
		if err != nil {
			t.Fatal(err)
		}
		return IntentSubmitCommand{Intent: intent, ExecutionPlan: plan, TenantID: tenantID, SubjectID: subjectID, InitiatedBy: subjectID, CorrelationID: uuid.NewString(), CommitmentAction: "create"}
	}
	commands := []IntentSubmitCommand{makeCommand("target-a"), makeCommand("target-b")}
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, 2)
	for _, command := range commands {
		command := command
		go func() {
			_, created, err := NewPGStore(db).SubmitIntent(ctx, command)
			results <- result{created: created, err: err}
		}()
	}
	created, conflicts := 0, 0
	for range 2 {
		result := <-results
		if result.created {
			created++
		}
		if errors.Is(result.err, ErrIdempotencyConflict) {
			conflicts++
		} else if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("created=%d conflicts=%d", created, conflicts)
	}
	for table, want := range map[string]int{"runtime_intents": 1, "execution_plans": 1, "operations": 1, "security_audit_events": 1, "operation_read_model": 1} {
		if got := countWhere(t, db, "SELECT count(*) FROM "+table+" WHERE tenant_id=$1", tenantID); got != want {
			t.Fatalf("%s rows=%d want=%d", table, got, want)
		}
	}
}

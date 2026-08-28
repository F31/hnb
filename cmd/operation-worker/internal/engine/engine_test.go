package engine

import (
	"testing"

	"github.com/google/uuid"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from OperationStatus
		to   OperationStatus
		want bool
	}{
		{StatusPending, StatusQueued, true},
		{StatusPending, StatusPendingApproval, true},
		{StatusPending, StatusCancelled, true},
		{StatusPending, StatusInProgress, false},
		{StatusPending, StatusSucceeded, false},
		{StatusPendingApproval, StatusQueued, true},
		{StatusPendingApproval, StatusCancelled, true},
		{StatusPendingApproval, StatusInProgress, false},
		{StatusQueued, StatusInProgress, true},
		{StatusQueued, StatusQueuedOffline, true},
		{StatusQueued, StatusCancelled, true},
		{StatusQueuedOffline, StatusInProgress, true},
		{StatusQueuedOffline, StatusFailed, true},
		{StatusInProgress, StatusSucceeded, true},
		{StatusInProgress, StatusFailed, true},
		{StatusInProgress, StatusPaused, true},
		{StatusInProgress, StatusCompensating, true},
		{StatusPaused, StatusInProgress, true},
		{StatusPaused, StatusCancelled, true},
		{StatusCompensating, StatusFailed, true},
		{StatusCompensating, StatusCancelled, true},
		{StatusSucceeded, StatusInProgress, false},
		{StatusFailed, StatusSucceeded, false},
		{StatusCancelled, StatusQueued, false},
	}
	for _, tt := range tests {
		got := CanTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(StatusSucceeded) {
		t.Error("Succeeded should be terminal")
	}
	if !IsTerminal(StatusFailed) {
		t.Error("Failed should be terminal")
	}
	if !IsTerminal(StatusCancelled) {
		t.Error("Cancelled should be terminal")
	}
	if IsTerminal(StatusPending) {
		t.Error("Pending should not be terminal")
	}
	if IsTerminal(StatusInProgress) {
		t.Error("InProgress should not be terminal")
	}
}

func TestStateMachine_CreateOperation(t *testing.T) {
	sm := NewOperationStateMachine()
	op := sm.CreateOperation(OpDeploy, "tenant-1", "proj-1", "env-1", "", "alice", "corr-1", "key-1", nil)
	if _, err := uuid.Parse(op.ID); err != nil {
		t.Fatalf("operation ID is not a UUID: %q", op.ID)
	}
	if op.Status != StatusPending {
		t.Errorf("expected Pending, got %s", op.Status)
	}
	if op.Type != OpDeploy {
		t.Errorf("expected deploy, got %s", op.Type)
	}
	if op.InitiatedBy != "alice" {
		t.Errorf("expected alice, got %s", op.InitiatedBy)
	}
}

func TestStateMachine_Transition(t *testing.T) {
	sm := NewOperationStateMachine()
	op := sm.CreateOperation(OpDeploy, "t1", "p1", "e1", "", "alice", "c1", "k1", nil)

	if err := sm.Transition(op, StatusQueued, "alice", "approved"); err != nil {
		t.Fatal(err)
	}
	if op.Status != StatusQueued {
		t.Errorf("expected queued, got %s", op.Status)
	}

	if err := sm.Transition(op, StatusInProgress, "worker-1", "started"); err != nil {
		t.Fatal(err)
	}
	if op.StartedAt == nil {
		t.Error("started_at should be set")
	}

	if err := sm.Transition(op, StatusSucceeded, "worker-1", "done"); err != nil {
		t.Fatal(err)
	}
	if op.CompletedAt == nil {
		t.Error("completed_at should be set")
	}
}

func TestStateMachine_InvalidTransition(t *testing.T) {
	sm := NewOperationStateMachine()
	op := sm.CreateOperation(OpDeploy, "t1", "p1", "e1", "", "alice", "c1", "k1", nil)

	if err := sm.Transition(op, StatusSucceeded, "alice", ""); err == nil {
		t.Error("expected error for invalid transition Pending -> Succeeded")
	}
}

func TestStateMachine_Approve(t *testing.T) {
	sm := NewOperationStateMachine()
	op := sm.CreateOperation(OpDeploy, "t1", "p1", "e1", "", "alice", "c1", "k1", nil)
	op.Status = StatusPendingApproval

	if err := sm.Approve(op, "bob"); err != nil {
		t.Fatal(err)
	}
	if op.Status != StatusQueued {
		t.Errorf("expected queued, got %s", op.Status)
	}
	if op.ApprovedBy != "bob" {
		t.Errorf("expected bob, got %s", op.ApprovedBy)
	}
}

func TestPlanGenerator_Validate(t *testing.T) {
	pg := NewPlanGenerator()
	steps := []StepSpec{
		{ID: "step-1", Name: "deploy-db", StepType: "deploy", DependsOn: nil},
		{ID: "step-2", Name: "deploy-app", StepType: "deploy", DependsOn: []string{"step-1"}},
	}

	plan, err := pg.GeneratePlan("rel-1", "t1", "p1", "e1", steps, &PolicyResult{Passed: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanDigest == "" {
		t.Error("plan digest should be set")
	}
	if len(plan.ID) == 0 {
		t.Error("plan ID should be set")
	}
	if _, err := uuid.Parse(plan.ID); err != nil {
		t.Fatalf("plan ID is not a UUID: %q", plan.ID)
	}
	if _, err := uuid.Parse(plan.ID); err != nil {
		t.Fatalf("plan ID is not a UUID: %q", plan.ID)
	}

	if err := pg.ValidatePlan(plan); err != nil {
		t.Errorf("valid plan should pass: %v", err)
	}
}

func TestPlanGenerator_ValidateCycle(t *testing.T) {
	pg := NewPlanGenerator()
	steps := []StepSpec{
		{ID: "step-1", DependsOn: []string{"step-3"}},
		{ID: "step-2", DependsOn: []string{"step-1"}},
		{ID: "step-3", DependsOn: []string{"step-2"}},
	}
	plan, err := pg.GeneratePlan("rel-1", "t1", "p1", "e1", steps, &PolicyResult{Passed: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.ValidatePlan(plan); err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestDAGResolver_Resolve(t *testing.T) {
	steps := []StepSpec{
		{ID: "a", DependsOn: nil},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}
	r := NewDAGResolver(steps)
	order, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 {
		t.Errorf("expected 4 steps, got %d", len(order))
	}

	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	if pos["a"] > pos["b"] || pos["a"] > pos["c"] {
		t.Error("a should come before b and c")
	}
	if pos["b"] > pos["d"] || pos["c"] > pos["d"] {
		t.Error("b and c should come before d")
	}
}

func TestDAGResolver_ExecutionLevels(t *testing.T) {
	steps := []StepSpec{
		{ID: "a", DependsOn: nil},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}
	r := NewDAGResolver(steps)
	levels, err := r.ExecutionLevels()
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != 3 {
		t.Errorf("expected 3 levels, got %d", len(levels))
	}
}

func TestDAGResolver_ReadySteps(t *testing.T) {
	steps := []StepSpec{
		{ID: "a", DependsOn: nil},
		{ID: "b", DependsOn: []string{"a"}},
	}
	r := NewDAGResolver(steps)

	ready := r.ReadySteps(map[string]bool{})
	if len(ready) != 1 || ready[0] != "a" {
		t.Errorf("expected only step a ready, got %v", ready)
	}

	ready = r.ReadySteps(map[string]bool{"a": true})
	if len(ready) != 1 || ready[0] != "b" {
		t.Errorf("expected step b ready, got %v", ready)
	}

	ready = r.ReadySteps(map[string]bool{"a": true, "b": true})
	if len(ready) != 0 {
		t.Errorf("expected no ready steps, got %v", ready)
	}
}

func TestOutputResolver(t *testing.T) {
	r := NewOutputResolver()
	r.SetStepOutput("step-1", map[string]string{"host": "db.example.com", "port": "5432"})

	val, err := r.ResolveBinding(OutputBinding{FromStep: "step-1", Expression: "$.host"})
	if err != nil {
		t.Fatal(err)
	}
	if val != "db.example.com" {
		t.Errorf("expected db.example.com, got %s", val)
	}
}

func TestCompensationEngine_Defaults(t *testing.T) {
	ce := NewCompensationEngine()

	s := ce.GetStrategy("database")
	if s.Compensation != CompRetainMark {
		t.Errorf("expected retain_mark for database, got %s", s.Compensation)
	}
	if !s.RetainData {
		t.Error("database should retain data")
	}

	s = ce.GetStrategy("deployment")
	if s.Compensation != CompRollback {
		t.Errorf("expected rollback for deployment, got %s", s.Compensation)
	}

	s = ce.GetStrategy("unknown-type")
	if s.Compensation != CompDelete {
		t.Errorf("expected delete for unknown type, got %s", s.Compensation)
	}
}

func TestCompensationEngine_Register(t *testing.T) {
	ce := NewCompensationEngine()
	ce.RegisterStrategy("custom", CompensationStrategy{
		ResourceType:     "custom",
		Compensation:     CompRetainNofiy,
		RetainData:       true,
		RequiresApproval: true,
	})
	s := ce.GetStrategy("custom")
	if s.Compensation != CompRetainNofiy {
		t.Errorf("expected retain_notify, got %s", s.Compensation)
	}
}

func TestNewIdempotencyKey(t *testing.T) {
	k1 := NewIdempotencyKey("op-1", "step-1")
	k2 := NewIdempotencyKey("op-1", "step-1")
	k3 := NewIdempotencyKey("op-1", "step-2")

	if k1 != k2 {
		t.Error("idempotency keys should be deterministic")
	}
	if k1 == k3 {
		t.Error("different steps should have different keys")
	}
}

func TestComputePlanDigest(t *testing.T) {
	plan := &ExecutionPlan{
		ReleaseID: "rel-1",
		Steps:     []StepSpec{{ID: "a"}},
	}
	d1, err := ComputePlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := ComputePlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Error("digests should be deterministic")
	}
}

func TestEvaluateStepCompletion(t *testing.T) {
	steps := []StepSpec{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}

	results := []StepResult{
		{StepID: "a", Status: StepSucceeded},
		{StepID: "b", Status: StepSucceeded},
		{StepID: "c", Status: StepSucceeded},
	}
	d := EvaluateStepCompletion(steps, results)
	if !d.AllSucceeded {
		t.Error("all steps succeeded")
	}
	if d.HasFailed {
		t.Error("no failures expected")
	}

	results = []StepResult{
		{StepID: "a", Status: StepSucceeded},
		{StepID: "b", Status: StepFailed},
	}
	d = EvaluateStepCompletion(steps, results)
	if d.AllSucceeded {
		t.Error("not all succeeded")
	}
	if !d.HasFailed {
		t.Error("has failures expected")
	}
	if !d.ShouldCompensate {
		t.Error("should compensate")
	}
}

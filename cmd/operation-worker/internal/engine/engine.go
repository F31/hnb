package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type OperationStateMachine struct {
	Compensation *CompensationEngine
}

func NewOperationStateMachine() *OperationStateMachine {
	return &OperationStateMachine{
		Compensation: NewCompensationEngine(),
	}
}

func (sm *OperationStateMachine) CreateOperation(
	opType OperationType,
	tenantID, projectID, environmentID, namespaceID string,
	initiatedBy string,
	correlationID string,
	idempotencyKey string,
	tags map[string]string,
) *Operation {
	now := time.Now().UTC()
	return &Operation{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		EnvironmentID:  environmentID,
		NamespaceID:    namespaceID,
		Type:           opType,
		Status:         StatusPending,
		InitiatedBy:    initiatedBy,
		CorrelationID:  correlationID,
		IdempotencyKey: idempotencyKey,
		Tags:           tags,
		CreatedAt:      now,
	}
}

func (sm *OperationStateMachine) Transition(op *Operation, to OperationStatus, actorID, reason string) error {
	if !CanTransition(op.Status, to) {
		return fmt.Errorf("invalid transition %s -> %s", op.Status, to)
	}
	prev := op.Status
	op.Status = to
	now := time.Now().UTC()

	switch to {
	case StatusInProgress:
		if op.StartedAt == nil {
			op.StartedAt = &now
		}
	case StatusSucceeded, StatusFailed, StatusCancelled:
		op.CompletedAt = &now
	}

	_ = prev
	_ = actorID
	_ = reason
	return nil
}

func (sm *OperationStateMachine) Approve(op *Operation, approvedBy string) error {
	if op.Status != StatusPendingApproval {
		return fmt.Errorf("cannot approve operation in %s state", op.Status)
	}
	op.ApprovedBy = approvedBy
	return sm.Transition(op, StatusQueued, approvedBy, "approved")
}

func ComputePlanDigest(plan *ExecutionPlan) (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:]), nil
}

type PlanGenerator struct{}

func NewPlanGenerator() *PlanGenerator {
	return &PlanGenerator{}
}

func (pg *PlanGenerator) GeneratePlan(
	releaseID, tenantID, projectID, environmentID string,
	steps []StepSpec,
	policyResult *PolicyResult,
	nodeGroupAffinity []string,
) (*ExecutionPlan, error) {
	plan := &ExecutionPlan{
		ReleaseID:         releaseID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		EnvironmentID:     environmentID,
		Steps:             steps,
		Outputs:           make(map[string]string),
		PolicyResult:      policyResult,
		NodeGroupAffinity: nodeGroupAffinity,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	}

	digest, err := ComputePlanDigest(plan)
	if err != nil {
		return nil, err
	}
	plan.PlanDigest = digest
	plan.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(digest)).String()

	return plan, nil
}

func (pg *PlanGenerator) ValidatePlan(plan *ExecutionPlan) error {
	if len(plan.Steps) == 0 {
		return fmt.Errorf("plan must have at least one step")
	}
	resolver := NewDAGResolver(plan.Steps)
	if _, err := resolver.Resolve(); err != nil {
		return fmt.Errorf("invalid step DAG: %w", err)
	}
	if plan.PolicyResult != nil && !plan.PolicyResult.Passed {
		return fmt.Errorf("plan failed policy check: %v", plan.PolicyResult.Decisions)
	}
	return nil
}

type StepCompleteDecision struct {
	AllSucceeded     bool
	HasFailed        bool
	ShouldCompensate bool
	ShouldPause      bool
}

func EvaluateStepCompletion(steps []StepSpec, stepResults []StepResult) StepCompleteDecision {
	allDone := true
	hasFailures := false

	for _, spec := range steps {
		found := false
		for _, result := range stepResults {
			if result.StepID == spec.ID {
				found = true
				if result.Status == StepFailed {
					hasFailures = true
				}
				break
			}
		}
		if !found {
			allDone = false
		}
	}

	return StepCompleteDecision{
		AllSucceeded:     allDone && !hasFailures,
		HasFailed:        hasFailures,
		ShouldCompensate: hasFailures,
		ShouldPause:      hasFailures,
	}
}

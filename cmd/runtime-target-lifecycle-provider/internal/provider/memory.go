package provider

import (
	"context"
	"fmt"
	"sync"
)

type targetRecord struct {
	TenantID           string
	TargetID           string
	TargetKind         string
	Action             string
	StepID             string
	OperationID        string
	IdempotencyKey     string
	ExecutionAttemptID string
	FencingGeneration  int64
	Managed            bool
	DesiredVersion     string
	Outputs            map[string]string
	Checkpoint         string
}

type MemoryManager struct {
	profile Profile
	mu      sync.Mutex
	targets map[string]targetRecord
}

func NewMemoryManager(profile Profile) *MemoryManager {
	return &MemoryManager{profile: profile, targets: make(map[string]targetRecord)}
}

func (m *MemoryManager) Apply(ctx context.Context, execution ExecutionContext, input LifecycleInput) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, targetError("apply lifecycle action", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.targets[input.TargetID]
	if ok {
		if existing.TenantID != execution.TenantID || existing.TargetKind != input.TargetKind {
			return Result{}, fail(409, ErrorResourceConflict, false, "target management relation belongs to a different tenant or kind")
		}
		if existing.FencingGeneration > execution.FencingGeneration {
			return Result{}, fail(409, ErrorFenced, false, "target fencing generation %d is newer than request generation %d", existing.FencingGeneration, execution.FencingGeneration)
		}
		if existing.FencingGeneration == execution.FencingGeneration {
			if existing.StepID != execution.StepID || existing.OperationID != execution.OperationID || existing.IdempotencyKey != execution.IdempotencyKey || existing.ExecutionAttemptID != execution.ExecutionAttemptID || existing.Action != input.Action {
				return Result{}, fail(409, ErrorResourceConflict, false, "equal-generation lifecycle action is not an exact replay")
			}
			return Result{Outputs: copyMap(existing.Outputs), Checkpoint: existing.Checkpoint}, nil
		}
	}
	if input.Action == "unmanage" && (!ok || !existing.Managed) {
		return Result{}, fail(409, ErrorResourceConflict, false, "target is not managed by this lifecycle provider")
	}
	managed := input.Action != "unmanage"
	outputs := map[string]string{
		"targetId":        input.TargetID,
		"targetKind":      input.TargetKind,
		"action":          input.Action,
		"managed":         fmt.Sprintf("%t", managed),
		"observationKind": m.profile.ObservationKind,
	}
	if input.DesiredVersion != "" {
		outputs["desiredVersion"] = input.DesiredVersion
	}
	checkpoint := fmt.Sprintf("%s:%s:%d", input.TargetKind, input.TargetID, execution.FencingGeneration)
	m.targets[input.TargetID] = targetRecord{
		TenantID: execution.TenantID, TargetID: input.TargetID, TargetKind: input.TargetKind,
		Action: input.Action, StepID: execution.StepID, OperationID: execution.OperationID,
		IdempotencyKey: execution.IdempotencyKey, ExecutionAttemptID: execution.ExecutionAttemptID,
		FencingGeneration: execution.FencingGeneration, Managed: managed, DesiredVersion: input.DesiredVersion,
		Outputs: outputs, Checkpoint: checkpoint,
	}
	return Result{Outputs: outputs, Checkpoint: checkpoint}, nil
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type MemoryObserverRegistry struct {
	mu        sync.Mutex
	observers map[string]string
}

func NewMemoryObserverRegistry() *MemoryObserverRegistry {
	return &MemoryObserverRegistry{observers: make(map[string]string)}
}

func (r *MemoryObserverRegistry) Register(ctx context.Context, tenantID, targetID, targetKind, observerKind string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observers[tenantID+":"+targetID] = targetKind + ":" + observerKind
	return nil
}

func (r *MemoryObserverRegistry) Unregister(ctx context.Context, tenantID, targetID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.observers, tenantID+":"+targetID)
	return nil
}

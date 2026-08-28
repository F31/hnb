package engine

import (
	"context"
	"fmt"
)

// Engine ties intent parsing/validation and server-side planning together.
type Engine struct {
	validator *IntentValidator
	planner   *Planner
}

func NewEngine(providers ...LifecycleProviderResolver) *Engine {
	planner := NewPlanner()
	if len(providers) > 0 {
		planner = NewPlanner(providers[0])
	}
	return &Engine{
		validator: NewIntentValidator(),
		planner:   planner,
	}
}

// Process executes the full intent pipeline: validate → plan → return ExecutionPlan.
func (e *Engine) Process(intent *RuntimeIntent) (*ExecutionPlan, error) {
	if err := e.validator.Validate(intent); err != nil {
		return nil, fmt.Errorf("intent validation failed: %w", err)
	}
	return e.planner.Plan(intent, "", "", "", "", "")
}

// ProcessWithScope is like Process but also validates scope references.
func (e *Engine) ProcessWithScope(intent *RuntimeIntent, tenantID string) (*ExecutionPlan, error) {
	return e.ProcessWithContext(context.Background(), intent, tenantID)
}

func (e *Engine) ProcessWithContext(ctx context.Context, intent *RuntimeIntent, tenantID string) (*ExecutionPlan, error) {
	if err := e.validator.ValidateWithScope(intent, tenantID); err != nil {
		return nil, fmt.Errorf("intent scope validation failed: %w", err)
	}
	return e.planner.PlanContext(ctx, intent, tenantID, "", "", "", "")
}

func (e *Engine) AvailableLifecycleDecisions(ctx context.Context) []CompatibilityDecision {
	if e.planner.matrix == nil || e.planner.providers == nil {
		return nil
	}
	var available []CompatibilityDecision
	for _, decision := range e.planner.matrix.Decisions() {
		if decision.Status != "REQUIRED" {
			continue
		}
		if _, err := e.planner.providers.ResolveLifecycleProvider(ctx, decision); err == nil {
			available = append(available, decision)
		}
	}
	return available
}

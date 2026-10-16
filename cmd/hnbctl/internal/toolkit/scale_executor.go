package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ScaleSteps are the caller-supplied reconcile steps for one scale run.
type ScaleSteps struct {
	Apply  func(ctx context.Context) error
	Verify func(ctx context.Context) error
}

// ScaleExecutionInput drives a gated, resumable scale run.
type ScaleExecutionInput struct {
	Store   RunStore
	Writer  string
	Epoch   int64
	Plan    ScalePlan
	Timeout time.Duration
	Steps   ScaleSteps
}

// ScaleExecutionResult reports what the run recorded.
type ScaleExecutionResult struct {
	RunID   string      `json:"runId"`
	Status  string      `json:"status"`
	Applied []PhaseName `json:"applied"`
}

// RunScale executes a scale plan through the run store. It refuses an
// unauthorized or empty plan before any mutation and never finalises succeeded
// unless every executed phase succeeded. Resume skips already-succeeded phases.
func RunScale(ctx context.Context, in ScaleExecutionInput) (ScaleExecutionResult, error) {
	var res ScaleExecutionResult
	if !in.Plan.Authorized {
		if len(in.Plan.Blocked) > 0 {
			return res, fmt.Errorf("SCALE_PLAN_BLOCKED: %s", strings.Join(in.Plan.Blocked, "; "))
		}
		return res, fmt.Errorf("SCALE_PLAN_BLOCKED: plan is not authorized")
	}
	if in.Steps.Apply == nil {
		return res, fmt.Errorf("SCALE_STEP_MISSING: apply step is required")
	}
	if state, err := LoadRunState(in.Store.Dir); err == nil {
		res.RunID = state.RunID
	}
	var ran []PhaseName
	step := func(name PhaseName, fn func(context.Context) error) error {
		if fn == nil {
			return nil
		}
		didRun, err := runUpgradePhase(ctx, in.Store, in.Writer, in.Epoch, name, in.Timeout, fn)
		if err != nil {
			return err
		}
		if didRun {
			ran = append(ran, name)
		}
		return nil
	}
	fail := func(err error) (ScaleExecutionResult, error) {
		_, _ = in.Store.Finalize(in.Writer, in.Epoch, PhaseBlockedState, nil)
		res.Status, res.Applied = PhaseBlockedState, ran
		return res, err
	}
	if err := step(PhasePreflight, func(context.Context) error { return nil }); err != nil {
		return fail(err)
	}
	if err := step(PhaseInfraDelta, in.Steps.Apply); err != nil {
		return fail(err)
	}
	if err := step(PhasePlatformVerified, in.Steps.Verify); err != nil {
		return fail(err)
	}
	if len(ran) == 0 {
		return fail(fmt.Errorf("SCALE_NO_STEPS: the scale recorded no executed phase"))
	}
	if _, err := in.Store.Finalize(in.Writer, in.Epoch, PhaseSucceededState, ran); err != nil {
		return fail(err)
	}
	res.Status, res.Applied = PhaseSucceededState, ran
	return res, nil
}

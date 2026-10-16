package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// UpgradeSteps are the caller-supplied reconcile steps for one upgrade run. A
// nil step with no required work is skipped and not recorded; a required step
// that is missing or fails fails the run closed.
type UpgradeSteps struct {
	Preflight func(ctx context.Context) error
	Backup    func(ctx context.Context) error
	Infra     func(ctx context.Context) error
	// Rebacking provisions the target data-plane backing and verifies it in a
	// dual-run window before cutover. It is only executed when the plan changes
	// the backing kind; the original backing is kept until it succeeds.
	Rebacking func(ctx context.Context) error
	Migrate   func(ctx context.Context) error
	Release   func(ctx context.Context) error
	Verify    func(ctx context.Context) error
}

// UpgradeExecutionInput drives a gated, resumable profile upgrade (or rollback).
type UpgradeExecutionInput struct {
	Store  RunStore
	Writer string
	Epoch  int64
	Plan   UpgradePlan
	// Observed facts supplied by the caller; the executor assumes nothing.
	BackupVerified    bool
	RequiresBackup    bool
	AllowIrreversible bool
	Timeout           time.Duration
	Steps             UpgradeSteps
}

// UpgradeExecutionResult reports what the run recorded.
type UpgradeExecutionResult struct {
	RunID   string      `json:"runId"`
	Status  string      `json:"status"`
	Applied []PhaseName `json:"applied"`
}

// RunUpgrade executes a profile upgrade through the run store. It refuses a
// blocked plan before any mutation, enforces the backup prerequisite and the
// irreversible-migration acknowledgement, records each phase in order and never
// finalises succeeded unless every executed phase succeeded. One-shot phases are
// never re-executed on resume.
func RunUpgrade(ctx context.Context, in UpgradeExecutionInput) (UpgradeExecutionResult, error) {
	var res UpgradeExecutionResult
	if reasons := planBlockedReasons(in.Plan); len(reasons) > 0 {
		return res, fmt.Errorf("UPGRADE_PLAN_BLOCKED: %s", strings.Join(reasons, "; "))
	}
	irreversible := 0
	for _, step := range in.Plan.Steps {
		if step.Kind == "migration" && !step.Reversible {
			irreversible++
		}
	}
	if irreversible > 0 && !in.AllowIrreversible {
		return res, fmt.Errorf("UPGRADE_IRREVERSIBLE_UNACKNOWLEDGED: %d irreversible migration(s) require explicit acknowledgement", irreversible)
	}
	requiresBackup := in.RequiresBackup || irreversible > 0

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

	fail := func(err error) (UpgradeExecutionResult, error) {
		_, _ = in.Store.Finalize(in.Writer, in.Epoch, PhaseBlockedState, nil)
		res.Status = PhaseBlockedState
		res.Applied = ran
		return res, err
	}

	if err := step(PhasePreflight, func(ctx context.Context) error {
		if in.Steps.Preflight != nil {
			return in.Steps.Preflight(ctx)
		}
		return nil
	}); err != nil {
		return fail(err)
	}
	if err := step(PhaseBackup, func(ctx context.Context) error {
		if requiresBackup && !in.BackupVerified {
			return fmt.Errorf("BACKUP_REQUIRED: a verified backup is required before migrating")
		}
		if in.Steps.Backup != nil {
			return in.Steps.Backup(ctx)
		}
		return nil
	}); err != nil {
		return fail(err)
	}
	if err := step(PhaseInfraDelta, in.Steps.Infra); err != nil {
		return fail(err)
	}
	if planHasBackingStep(in.Plan) {
		if err := step(PhaseDataPlaneRebacking, in.Steps.Rebacking); err != nil {
			return fail(err)
		}
	}
	if err := step(PhaseDatabaseMigrate, in.Steps.Migrate); err != nil {
		return fail(err)
	}
	if err := step(PhasePlatformRelease, in.Steps.Release); err != nil {
		return fail(err)
	}
	if err := step(PhasePlatformVerified, in.Steps.Verify); err != nil {
		return fail(err)
	}
	if len(ran) == 0 {
		return fail(fmt.Errorf("UPGRADE_NO_STEPS: the upgrade recorded no executed phase"))
	}
	if _, err := in.Store.Finalize(in.Writer, in.Epoch, PhaseSucceededState, ran); err != nil {
		return fail(err)
	}
	res.Status = PhaseSucceededState
	res.Applied = ran
	return res, nil
}

// planBlockedReasons returns the blocked reasons of a plan (nil when authorized).
func planBlockedReasons(plan UpgradePlan) []string {
	if plan.Authorized {
		return nil
	}
	if len(plan.Blocked) > 0 {
		return plan.Blocked
	}
	return []string{"UPGRADE_PLAN_NOT_AUTHORIZED"}
}

// planHasBackingStep reports whether a plan changes the data-plane backing kind.
func planHasBackingStep(plan UpgradePlan) bool {
	for _, step := range plan.Steps {
		if step.Kind == "backings" {
			return true
		}
	}
	return false
}

// runUpgradePhase records one phase, skipping an already-succeeded phase on
// resume and refusing to re-run a one-shot phase that already started.
func runUpgradePhase(ctx context.Context, store RunStore, writer string, epoch int64, name PhaseName, budget time.Duration, fn func(context.Context) error) (bool, error) {
	if state, err := LoadRunState(store.Dir); err == nil {
		if p := state.Phases[string(name)]; p != nil {
			switch p.State {
			case PhaseSucceededState:
				return false, nil
			case PhaseRunningState, PhaseFailedState, PhaseBlockedState:
				if phasesNeverRerun[name] {
					return false, fmt.Errorf("UPGRADE_PHASE_NOT_RERUNNABLE: %s is %s and is never re-executed", name, p.State)
				}
			}
		}
	}
	return true, store.RunPhaseWithTimeout(ctx, writer, epoch, name, budget, nil, nil, fn)
}

// PlanRollback derives the rollback plan for an applied upgrade plan. It refuses
// when any applied migration or managed-claim growth cannot be reversed.
func PlanRollback(plan UpgradePlan) (UpgradePlan, []string) {
	var blocks []string
	for _, step := range plan.Steps {
		switch {
		case step.Kind == "migration" && !step.Reversible:
			blocks = append(blocks, fmt.Sprintf("ROLLBACK_IRREVERSIBLE: migration %s cannot be rolled back", step.Target))
		case step.Kind == "storage":
			blocks = append(blocks, fmt.Sprintf("ROLLBACK_STORAGE_IRREVERSIBLE: managed claim %s cannot shrink", step.Target))
		case step.Kind == "backings":
			blocks = append(blocks, fmt.Sprintf("ROLLBACK_REBACKING_UNSUPPORTED: data-plane backing %s -> %s was cut over and is not automatically reversible", step.From, step.To))
		}
	}
	rollback := plan
	rollback.From, rollback.To = plan.To, plan.From
	rollback.FromDigest, rollback.ToDigest = plan.ToDigest, plan.FromDigest
	rollback.Steps = reverseUpgradeSteps(plan.Steps)
	rollback.Blocked = blocks
	rollback.Authorized = len(blocks) == 0
	return rollback, blocks
}

// reverseUpgradeSteps swaps each step's from/to and reverses execution order so a
// rollback undoes the applied steps last-first.
func reverseUpgradeSteps(steps []UpgradeStep) []UpgradeStep {
	out := make([]UpgradeStep, 0, len(steps))
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		step.From, step.To = steps[i].To, steps[i].From
		out = append(out, step)
	}
	return out
}

// RunUpgradeRollback replays the previous lock by executing the rollback plan.
func RunUpgradeRollback(ctx context.Context, in UpgradeExecutionInput) (UpgradeExecutionResult, error) {
	rollback, blocks := PlanRollback(in.Plan)
	if len(blocks) > 0 {
		return UpgradeExecutionResult{}, fmt.Errorf("ROLLBACK_REFUSED: %s", strings.Join(blocks, "; "))
	}
	in.Plan = rollback
	return RunUpgrade(ctx, in)
}

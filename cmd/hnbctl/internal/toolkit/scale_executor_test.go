package toolkit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scaleRunFixture(t *testing.T) (RunStore, string, int64) {
	t.Helper()
	store := RunStore{Dir: filepath.Join(t.TempDir(), "run")}
	if _, err := store.CreateRun("scale", "scale-1", "sha256:"+strings.Repeat("a", 64), "", "external-cluster", nil); err != nil {
		t.Fatal(err)
	}
	active, err := store.AcquireWriter("w", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return store, "w", active.Epoch
}

func authorizedScalePlan() ScalePlan {
	return ScalePlan{SchemaVersion: ScalePlanSchema, Authorized: true, Steps: []UpgradeStep{
		{Kind: "components", Target: "apiserver", From: "1", To: "3", Reversible: true},
	}}
}

func TestRunScaleRefusesBlockedPlan(t *testing.T) {
	store, writer, epoch := scaleRunFixture(t)
	plan := authorizedScalePlan()
	plan.Authorized, plan.Blocked = false, []string{"SCALE_BELOW_QUORUM: apiserver"}
	if _, err := RunScale(context.Background(), ScaleExecutionInput{Store: store, Writer: writer, Epoch: epoch, Plan: plan, Steps: ScaleSteps{Apply: func(context.Context) error { return nil }}}); err == nil || !strings.Contains(err.Error(), "SCALE_PLAN_BLOCKED") {
		t.Fatalf("blocked scale executed: %v", err)
	}
}

func TestRunScaleRequiresApplyStep(t *testing.T) {
	store, writer, epoch := scaleRunFixture(t)
	if _, err := RunScale(context.Background(), ScaleExecutionInput{Store: store, Writer: writer, Epoch: epoch, Plan: authorizedScalePlan()}); err == nil || !strings.Contains(err.Error(), "SCALE_STEP_MISSING") {
		t.Fatalf("missing apply step accepted: %v", err)
	}
}

func TestRunScaleSuccessRecordsOrderedPhases(t *testing.T) {
	store, writer, epoch := scaleRunFixture(t)
	applied, verified := 0, 0
	res, err := RunScale(context.Background(), ScaleExecutionInput{
		Store: store, Writer: writer, Epoch: epoch, Plan: authorizedScalePlan(),
		Steps: ScaleSteps{
			Apply:  func(context.Context) error { applied++; return nil },
			Verify: func(context.Context) error { verified++; return nil },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PhaseSucceededState || applied != 1 || verified != 1 {
		t.Fatalf("scale did not succeed: %+v apply=%d verify=%d", res, applied, verified)
	}
	state, _ := LoadRunState(store.Dir)
	if state.Status != PhaseSucceededState || state.Kind != "scale" {
		t.Fatalf("run not finalized as scale: %+v", state)
	}
	for _, name := range []PhaseName{PhasePreflight, PhaseInfraDelta, PhasePlatformVerified} {
		if p := state.Phases[string(name)]; p == nil || p.State != PhaseSucceededState {
			t.Fatalf("phase %s not succeeded: %+v", name, p)
		}
	}
}

func TestRunScaleResumeSkipsSucceededApply(t *testing.T) {
	store, writer, epoch := scaleRunFixture(t)
	if _, err := store.BeginPhase(writer, epoch, PhaseInfraDelta, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase(writer, epoch, PhaseInfraDelta, nil, nil); err != nil {
		t.Fatal(err)
	}
	applied := 0
	if _, err := RunScale(context.Background(), ScaleExecutionInput{
		Store: store, Writer: writer, Epoch: epoch, Plan: authorizedScalePlan(),
		Steps: ScaleSteps{Apply: func(context.Context) error { applied++; return nil }, Verify: func(context.Context) error { return nil }},
	}); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("succeeded apply must be skipped on resume, ran %d", applied)
	}
}

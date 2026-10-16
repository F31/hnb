package toolkit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// 集群 Ready/平台失败时不能输出完整交付成功.
func TestClusterReadyWithPlatformFailureIsNotCompleteDelivery(t *testing.T) {
	outcomes := map[DeliveryLayer]LayerOutcome{
		LayerNode:     {Layer: LayerNode, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
		LayerCluster:  {Layer: LayerCluster, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
		LayerPlatform: {Layer: LayerPlatform, State: LayerFailed, ReasonCode: "PLATFORM_LOGIN_FAILED"},
		LayerManaged:  {Layer: LayerManaged, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
	}
	r := NewDeliveryReport("run-1", outcomes)
	if r.CompleteDelivery || r.Overall == DeliverySuccess {
		t.Fatalf("platform failure was reported as complete delivery: %+v", r)
	}
	if r.Overall != DeliveryFailed {
		t.Fatalf("expected failed overall, got %q", r.Overall)
	}
	// The cluster pass must remain visible next to the platform failure.
	if len(r.FailedLayers) != 1 || r.FailedLayers[0] != LayerPlatform {
		t.Fatalf("failing layer not identified: %+v", r.FailedLayers)
	}
	var sb strings.Builder
	FormatDeliveryHuman(&sb, r)
	if !strings.Contains(sb.String(), "cluster  passed") || !strings.Contains(sb.String(), "platform failed") {
		t.Fatalf("human output must show both cluster pass and platform failure: %s", sb.String())
	}
}

// A missing layer is unverified, never silently passed.
func TestUnverifiedLayerBlocksDelivery(t *testing.T) {
	r := NewDeliveryReport("run-1", map[DeliveryLayer]LayerOutcome{
		LayerNode:    {Layer: LayerNode, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
		LayerCluster: {Layer: LayerCluster, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
	})
	if r.CompleteDelivery {
		t.Fatal("missing platform/managed layers produced complete delivery")
	}
	if r.Overall != DeliveryPartial {
		t.Fatalf("expected partial, got %q", r.Overall)
	}
	if len(r.Layers) != 4 {
		t.Fatalf("report must always cover all four layers: %+v", r.Layers)
	}
	for _, layer := range r.Layers {
		if layer.Layer == LayerPlatform || layer.Layer == LayerManaged {
			if layer.State != LayerSkipped || layer.ReasonCode != "LAYER_NOT_VERIFIED" {
				t.Fatalf("unverified layer mislabeled: %+v", layer)
			}
		}
	}
}

// Only all-pass yields complete delivery success.
func TestOnlyAllLayersPassedIsCompleteDelivery(t *testing.T) {
	outcomes := map[DeliveryLayer]LayerOutcome{}
	for _, layer := range DeliveryLayers {
		outcomes[layer] = LayerOutcome{Layer: layer, State: LayerPassed, ReasonCode: "CHECK_PASSED"}
	}
	r := NewDeliveryReport("run-1", outcomes)
	if !r.CompleteDelivery || r.Overall != DeliverySuccess || len(r.FailedLayers) != 0 {
		t.Fatalf("all-pass run not reported as complete: %+v", r)
	}
}

// Blocked (vs failed) layers yield a blocked overall verdict.
func TestBlockedLayerYieldsBlockedDelivery(t *testing.T) {
	r := NewDeliveryReport("run-1", map[DeliveryLayer]LayerOutcome{
		LayerNode:     {Layer: LayerNode, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
		LayerCluster:  {Layer: LayerCluster, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
		LayerPlatform: {Layer: LayerPlatform, State: LayerBlocked, ReasonCode: "PROVIDER_NOT_READY"},
		LayerManaged:  {Layer: LayerManaged, State: LayerPassed, ReasonCode: "CHECK_PASSED"},
	})
	if r.Overall != DeliveryBlocked || r.CompleteDelivery {
		t.Fatalf("blocked layer did not yield blocked delivery: %+v", r)
	}
}

// Per-phase timeout: a phase that overruns fails closed and never passes.
func TestPhaseTimeoutFailsClosedAndIsIndependent(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	run, _ := store.AcquireWriter("writer-a", time.Hour)
	budgets := DefaultPhaseTimeouts(time.Millisecond, time.Millisecond)

	// First phase overruns its budget.
	err := store.RunPhaseWithTimeout(context.Background(), "writer-a", run.Epoch, PhasePreparing, budgets[PhasePreparing], always(true, ""), always(true, ""), func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err == nil || !strings.Contains(err.Error(), "RUN_PHASE_TIMEOUT") {
		t.Fatalf("overrunning phase did not fail with timeout: %v", err)
	}
	state, _ := store.read()
	if state.Phases[string(PhasePreparing)].State != PhaseFailedState {
		t.Fatalf("timed-out phase must be failed, got %+v", state.Phases[string(PhasePreparing)])
	}
	// The next phase gets its own full budget, not the leftover of the first.
	start := time.Now()
	if err := store.RunPhaseWithTimeout(context.Background(), "writer-a", run.Epoch, PhaseIdentity, 2*time.Second, always(true, ""), always(true, ""), func(ctx context.Context) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatalf("independent phase budget exhausted: %v", err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("phase executed before its work finished")
	}
	state, _ = store.read()
	if state.Phases[string(PhaseIdentity)].State != PhaseSucceededState {
		t.Fatalf("independent phase should succeed: %+v", state.Phases[string(PhaseIdentity)])
	}
}

// A phase whose postcondition fails never records success, even if exec is nil-error.
func TestPhaseExecSuccessWithFailedPostconditionIsNotSuccess(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	run, _ := store.AcquireWriter("writer-a", time.Hour)
	err := store.RunPhaseWithTimeout(context.Background(), "writer-a", run.Epoch, PhasePlatformRelease, time.Second, always(true, ""), always(false, "PLATFORM_NOT_READY"), func(context.Context) error { return nil })
	if err == nil {
		t.Fatal("failed postcondition reported success")
	}
	state, _ := store.read()
	if state.Phases[string(PhasePlatformRelease)].State != PhaseFailedState {
		t.Fatalf("failed postcondition must fail the phase: %+v", state.Phases[string(PhasePlatformRelease)])
	}
}

// An exec error fails the phase; a completed run is not produced.
func TestPhaseExecErrorFailsPhase(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	run, _ := store.AcquireWriter("writer-a", time.Hour)
	sentinel := errors.New("boom")
	err := store.RunPhaseWithTimeout(context.Background(), "writer-a", run.Epoch, PhasePreflight, time.Second, always(true, ""), always(true, ""), func(context.Context) error { return sentinel })
	if err == nil || !strings.Contains(err.Error(), "RUN_PHASE_EXECUTION_FAILED") {
		t.Fatalf("exec error not propagated: %v", err)
	}
	state, _ := store.read()
	if state.Phases[string(PhasePreflight)].State != PhaseFailedState {
		t.Fatalf("failed exec must fail the phase: %+v", state.Phases[string(PhasePreflight)])
	}
}

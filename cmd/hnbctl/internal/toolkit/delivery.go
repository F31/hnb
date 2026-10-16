package toolkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

// Delivery layers are ordered from lowest to highest. A lower layer passing
// never implies a higher layer passed, and delivery success requires every
// layer to be verified independently.
type DeliveryLayer string

const (
	LayerNode     DeliveryLayer = "node"
	LayerCluster  DeliveryLayer = "cluster"
	LayerPlatform DeliveryLayer = "platform"
	LayerManaged  DeliveryLayer = "managed"
)

// DeliveryLayers is the fixed verification order.
var DeliveryLayers = []DeliveryLayer{LayerNode, LayerCluster, LayerPlatform, LayerManaged}

// Layer states.
const (
	LayerPassed  = "passed"
	LayerFailed  = "failed"
	LayerBlocked = "blocked"
	LayerSkipped = "skipped"
)

// Overall delivery verdicts.
const (
	DeliverySuccess = "success"
	DeliveryPartial = "partial"
	DeliveryFailed  = "failed"
	DeliveryBlocked = "blocked"
)

type LayerOutcome struct {
	Layer      DeliveryLayer `json:"layer"`
	State      string        `json:"state"`
	ReasonCode string        `json:"reasonCode,omitempty"`
	Detail     string        `json:"detail,omitempty"`
	ObservedAt time.Time     `json:"observedAt"`
}

type DeliveryReport struct {
	SchemaVersion string          `json:"schemaVersion"`
	RunID         string          `json:"runId"`
	Layers        []LayerOutcome  `json:"layers"`
	Overall       string          `json:"overall"`
	FailedLayers  []DeliveryLayer `json:"failedLayers,omitempty"`
	// CompleteDelivery is true only when every layer is independently verified.
	CompleteDelivery bool      `json:"completeDelivery"`
	GeneratedAt      time.Time `json:"generatedAt"`
}

// NewDeliveryReport builds the report. The overall verdict is derived from the
// layers, never asserted by the caller.
func NewDeliveryReport(runID string, outcomes map[DeliveryLayer]LayerOutcome) *DeliveryReport {
	r := &DeliveryReport{SchemaVersion: "delivery-report/v1", RunID: runID, GeneratedAt: time.Now().UTC()}
	var failed, blocked, skipped int
	for _, layer := range DeliveryLayers {
		outcome, ok := outcomes[layer]
		if !ok {
			// An unverified layer is never silently treated as passed.
			outcome = LayerOutcome{Layer: layer, State: LayerSkipped, ReasonCode: "LAYER_NOT_VERIFIED", ObservedAt: r.GeneratedAt}
		}
		if outcome.ObservedAt.IsZero() {
			outcome.ObservedAt = r.GeneratedAt
		}
		switch outcome.State {
		case LayerPassed:
		case LayerFailed:
			failed++
			r.FailedLayers = append(r.FailedLayers, layer)
		case LayerBlocked:
			blocked++
			r.FailedLayers = append(r.FailedLayers, layer)
		case LayerSkipped:
			skipped++
			r.FailedLayers = append(r.FailedLayers, layer)
		default:
			outcome.State, outcome.ReasonCode = LayerFailed, "LAYER_STATE_UNKNOWN"
			failed++
			r.FailedLayers = append(r.FailedLayers, layer)
		}
		r.Layers = append(r.Layers, outcome)
	}
	switch {
	case failed == 0 && blocked == 0 && skipped == 0:
		r.Overall = DeliverySuccess
		r.CompleteDelivery = true
	case blocked > 0 && failed == 0:
		r.Overall = DeliveryBlocked
	case failed == 0:
		r.Overall = DeliveryPartial
	default:
		r.Overall = DeliveryFailed
	}
	return r
}

// FormatDeliveryHuman renders a stable, credential-free human view. A passing
// lower layer is always shown together with the failing higher layers so a
// partial delivery is never reported as complete.
func FormatDeliveryHuman(w interface{ Write([]byte) (int, error) }, r *DeliveryReport) {
	fmt.Fprintf(w, "run=%s overall=%s completeDelivery=%v\n", r.RunID, r.Overall, r.CompleteDelivery)
	for _, layer := range r.Layers {
		fmt.Fprintf(w, "  %-8s %-7s %s %s\n", layer.Layer, layer.State, layer.ReasonCode, layer.Detail)
	}
	if len(r.FailedLayers) > 0 {
		names := make([]string, 0, len(r.FailedLayers))
		for _, layer := range r.FailedLayers {
			names = append(names, string(layer))
		}
		fmt.Fprintf(w, "  incomplete layers: %v\n", names)
	}
}

// PhaseTimeoutPolicy assigns an independent budget per phase. A phase that
// exceeds its budget fails closed; it is never reported as succeeded, and no
// later phase inherits the leftover budget.
type PhaseTimeoutPolicy map[PhaseName]time.Duration

// DefaultPhaseTimeouts derives per-phase budgets from the two configured
// budgets: downloads use download, everything else uses readiness.
func DefaultPhaseTimeouts(download, readiness time.Duration) PhaseTimeoutPolicy {
	return PhaseTimeoutPolicy{
		PhasePreparing: download, PhaseNodePrereq: download, PhaseRuntime: download,
		PhaseIdentity: readiness, PhaseInfrastructure: readiness, PhaseDatabaseMigrate: readiness,
		PhaseNetwork: readiness, PhaseInit: readiness, PhaseJoin: readiness,
		PhaseClusterVerified: readiness, PhasePlatformRelease: readiness,
		PhasePlatformVerified: readiness, PhaseHandoff: readiness,
		PhaseValidating: readiness, PhasePreflight: download,
	}
}

// RunPhaseWithTimeout executes one phase under its own deadline. The phase is
// recorded failed (never succeeded) on timeout, and the error names the phase.
// The budget covers the precondition, the execution and the postcondition, so a
// check that answers after the deadline can never make a phase pass.
func (s RunStore) RunPhaseWithTimeout(ctx context.Context, writerID string, epoch int64, name PhaseName, budget time.Duration, pre, post Condition, exec func(ctx context.Context) error) error {
	phaseCtx := ctx
	var cancel context.CancelFunc
	if budget > 0 {
		phaseCtx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}
	// Run the caller's checks inside the phase deadline regardless of which
	// store transition invokes them, and never count a post-deadline "ok".
	bounded := func(condition Condition) Condition {
		return func(_ context.Context) (bool, string, error) {
			if condition == nil {
				return true, "", nil
			}
			ok, reason, err := condition(phaseCtx)
			if err != nil {
				return ok, reason, err
			}
			if phaseCtx.Err() != nil {
				return false, "RUN_PHASE_TIMEOUT", errors.New("phase deadline exceeded")
			}
			return ok, reason, nil
		}
	}
	if _, err := s.BeginPhase(writerID, epoch, name, bounded(pre)); err != nil {
		if phaseCtx.Err() != nil {
			_, _ = s.FailPhase(writerID, epoch, name, "RUN_PHASE_TIMEOUT")
			return fmt.Errorf("RUN_PHASE_TIMEOUT: %s precondition failed within its %s budget", name, budget)
		}
		return err
	}
	execErr := exec(phaseCtx)
	if execErr != nil {
		if phaseCtx.Err() == context.DeadlineExceeded {
			_, _ = s.FailPhase(writerID, epoch, name, "RUN_PHASE_TIMEOUT")
			return fmt.Errorf("RUN_PHASE_TIMEOUT: %s exceeded its %s budget", name, budget)
		}
		_, _ = s.FailPhase(writerID, epoch, name, "RUN_PHASE_EXECUTION_FAILED")
		return fmt.Errorf("RUN_PHASE_EXECUTION_FAILED: %s: %v", name, execErr)
	}
	if phaseCtx.Err() != nil {
		_, _ = s.FailPhase(writerID, epoch, name, "RUN_PHASE_TIMEOUT")
		return fmt.Errorf("RUN_PHASE_TIMEOUT: %s consumed its %s budget", name, budget)
	}
	if _, err := s.CompletePhase(writerID, epoch, name, bounded(post), nil); err != nil {
		if phaseCtx.Err() != nil {
			_, _ = s.FailPhase(writerID, epoch, name, "RUN_PHASE_TIMEOUT")
			return fmt.Errorf("RUN_PHASE_TIMEOUT: %s postcondition failed within its %s budget", name, budget)
		}
		return err
	}
	if phaseCtx.Err() != nil {
		_, _ = s.FailPhase(writerID, epoch, name, "RUN_PHASE_TIMEOUT")
		return fmt.Errorf("RUN_PHASE_TIMEOUT: %s postcondition consumed its %s budget", name, budget)
	}
	return nil
}

// SortedPhases returns phase names in a stable order for reporting.
func SortedPhases(run *InstallRun) []string {
	names := make([]string, 0, len(run.Phases))
	for name := range run.Phases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LayerOutcomesFromRun derives per-layer outcomes from recorded run phases.
// A layer passes only when every one of its phases is present and succeeded an
// earlier failure, a blocked prerequisite or a missing mandatory phase leaves the
// layer unverified rather than letting a later phase mask it.
func LayerOutcomesFromRun(run *InstallRun) map[DeliveryLayer]LayerOutcome {
	out := map[DeliveryLayer]LayerOutcome{}
	if run == nil {
		return out
	}
	now := time.Now().UTC()
	add := func(layer DeliveryLayer, phases ...PhaseName) LayerOutcome {
		allPresent := true
		for _, name := range phases {
			p := run.Phases[string(name)]
			if p == nil {
				allPresent = false
				continue
			}
			switch p.State {
			case PhaseFailedState:
				reason := p.ReasonCode
				if reason == "" {
					reason = "LAYER_FAILED"
				}
				return LayerOutcome{Layer: layer, State: LayerFailed, ReasonCode: reason, Detail: string(name), ObservedAt: p.FinishedAt}
			case PhaseBlockedState:
				reason := p.ReasonCode
				if reason == "" {
					reason = "LAYER_BLOCKED"
				}
				return LayerOutcome{Layer: layer, State: LayerBlocked, ReasonCode: reason, Detail: string(name), ObservedAt: p.FinishedAt}
			case PhaseRunningState:
				return LayerOutcome{Layer: layer, State: LayerBlocked, ReasonCode: "PHASE_IN_PROGRESS", Detail: string(name), ObservedAt: p.StartedAt}
			case PhaseSucceededState:
				// continue; success alone cannot mask an earlier missing or failed phase
			default:
				return LayerOutcome{Layer: layer, State: LayerFailed, ReasonCode: "LAYER_STATE_UNKNOWN", Detail: string(name), ObservedAt: now}
			}
		}
		if !allPresent {
			return LayerOutcome{Layer: layer, State: LayerSkipped, ReasonCode: "LAYER_NOT_VERIFIED", ObservedAt: now}
		}
		return LayerOutcome{Layer: layer, State: LayerPassed, ReasonCode: "CHECK_PASSED", ObservedAt: now}
	}
	out[LayerNode] = add(LayerNode, PhaseNodePrereq, PhaseRuntime)
	out[LayerCluster] = add(LayerCluster, PhaseInit, PhaseJoin, PhaseNetwork, PhaseClusterVerified)
	out[LayerPlatform] = add(LayerPlatform, PhaseIdentity, PhaseInfrastructure, PhaseDatabaseMigrate, PhasePlatformRelease, PhasePlatformVerified)
	out[LayerManaged] = add(LayerManaged, PhaseHandoff)
	return out
}

// PlanResumeFromState projects the recorded phases into a resume plan using the
// canonical phase order. It never offers a reset action.
func PlanResumeFromState(run *InstallRun) []ResumeAction {
	order := InstallPhaseOrder()
	store := RunStore{}
	return store.PlanResume(run, order)
}

// FormatResumeHuman renders the recovery plan without exposing credentials.
func FormatResumeHuman(w io.Writer, run *InstallRun, plan []ResumeAction) {
	fmt.Fprintf(w, "run=%s status=%s lock=%s\n", run.RunID, run.Status, run.LockDigest)
	for _, action := range plan {
		if action.Action == "none" {
			continue
		}
		fmt.Fprintf(w, "  %-18s %s %s\n", action.Phase, action.Action, action.Reason)
	}
}

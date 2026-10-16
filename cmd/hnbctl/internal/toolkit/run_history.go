package toolkit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type RunEvent struct {
	SchemaVersion string    `json:"schemaVersion"`
	Sequence      int64     `json:"sequence"`
	RunID         string    `json:"runId"`
	Time          time.Time `json:"time"`
	Kind          string    `json:"kind"`
	Phase         PhaseName `json:"phase,omitempty"`
	State         string    `json:"state"`
	ReasonCode    string    `json:"reasonCode,omitempty"`
}

func (s RunStore) recordTransitions(run *InstallRun) error {
	previous, err := s.read()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	run.Events = nil
	if previous != nil {
		run.Events = append(run.Events, previous.Events...)
	}
	add := func(kind string, phase PhaseName, state, reason string) {
		sequence := int64(1)
		if len(run.Events) > 0 {
			sequence = run.Events[len(run.Events)-1].Sequence + 1
		}
		run.Events = append(run.Events, RunEvent{SchemaVersion: "install-event/v1", Sequence: sequence,
			RunID: SafeReportText(run.RunID), Time: s.now(), Kind: kind,
			Phase: PhaseName(SafeReportText(string(phase))), State: SafeReportText(state), ReasonCode: SafeReportText(reason)})
	}
	if previous == nil {
		add("run.created", "", run.Status, "")
	}
	for _, name := range SortedPhases(run) {
		phase := run.Phases[name]
		if phase == nil {
			continue
		}
		if previous == nil || !reflect.DeepEqual(previous.Phases[name], phase) {
			add("phase.changed", phase.Name, phase.State, phase.ReasonCode)
		}
	}
	if previous != nil && (previous.Status != run.Status || !previous.FinishedAt.Equal(run.FinishedAt)) {
		add("run.changed", "", run.Status, "")
	}
	return nil
}

type ProgressPhase struct {
	Name       PhaseName `json:"name"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	ReasonCode string    `json:"reasonCode,omitempty"`
}

type RunProgress struct {
	SchemaVersion string          `json:"schemaVersion"`
	RunID         string          `json:"runId"`
	State         string          `json:"state"`
	Completed     int             `json:"completedStages"`
	Total         int             `json:"totalStages"`
	Current       PhaseName       `json:"currentPhase,omitempty"`
	Elapsed       int64           `json:"elapsedSeconds"`
	LastSequence  int64           `json:"lastSequence"`
	Phases        []ProgressPhase `json:"phases"`
}

func InstallPhaseOrder() []PhaseName {
	return []PhaseName{PhaseValidating, PhasePlanning, PhasePreflight, PhasePreparing, PhaseNodePrereq,
		PhaseRuntime, PhaseInit, PhaseJoin, PhaseNetwork, PhaseClusterVerified, PhaseIdentity,
		PhaseInfrastructure, PhaseDatabaseMigrate, PhasePlatformRelease, PhasePlatformVerified, PhaseHandoff}
}

// UpgradePhaseOrder is the ordered phase pipeline of a profile upgrade run. It
// reuses the install phases (so resume semantics are shared) with a backup gate,
// an infra-delta step and a data-plane re-backing step (only present when the
// plan changes the backing kind).
func UpgradePhaseOrder() []PhaseName {
	return []PhaseName{PhasePreflight, PhaseBackup, PhaseInfraDelta, PhaseDataPlaneRebacking, PhaseDatabaseMigrate,
		PhasePlatformRelease, PhasePlatformVerified}
}

func ProgressFromRun(run *InstallRun, now time.Time) RunProgress {
	progress := RunProgress{SchemaVersion: "install-progress/v1", RunID: SafeReportText(run.RunID), State: SafeReportText(run.Status)}
	end := now
	if !run.FinishedAt.IsZero() {
		end = run.FinishedAt
	}
	if !run.StartedAt.IsZero() && end.After(run.StartedAt) {
		progress.Elapsed = int64(end.Sub(run.StartedAt).Seconds())
	}
	if len(run.Events) > 0 {
		progress.LastSequence = run.Events[len(run.Events)-1].Sequence
	}
	seen := map[string]bool{}
	add := func(name PhaseName) {
		seen[string(name)] = true
		phase := ProgressPhase{Name: PhaseName(SafeReportText(string(name))), State: PhasePendingState}
		if p := run.Phases[string(name)]; p != nil {
			phase.State, phase.Attempts, phase.ReasonCode = SafeReportText(p.State), p.Attempts, SafeReportText(p.ReasonCode)
		}
		if phase.State == PhaseSucceededState {
			progress.Completed++
		}
		if phase.State == PhaseRunningState || (progress.Current == "" && (phase.State == PhaseFailedState || phase.State == PhaseBlockedState)) {
			progress.Current = phase.Name
		}
		progress.Phases = append(progress.Phases, phase)
	}
	existing := run.RoutingMode == "existing" || strings.HasPrefix(run.RoutingMode, "external-cluster")
	for _, phase := range InstallPhaseOrder() {
		if existing && (phase == PhaseNodePrereq || phase == PhaseRuntime || phase == PhaseInit || phase == PhaseJoin || phase == PhaseNetwork) {
			continue
		}
		add(phase)
	}
	for _, name := range SortedPhases(run) {
		if !seen[name] {
			add(PhaseName(name))
		}
	}
	progress.Total = len(progress.Phases)
	return progress
}

type RunSummary struct {
	RunID        string    `json:"runId"`
	Directory    string    `json:"directory"`
	Status       string    `json:"status"`
	RoutingMode  string    `json:"routingMode"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt,omitempty"`
	ConfigDigest string    `json:"configDigest"`
	LockDigest   string    `json:"lockDigest"`
}

func CreateHistoryRun(root, configDigest, lockDigest, routingMode string, ownership []string) (RunStore, *InstallRun, error) {
	return CreateHistoryRunKind("install", root, configDigest, lockDigest, routingMode, ownership)
}

// CreateHistoryRunKind creates a run of the given kind ("install" or "upgrade").
func CreateHistoryRunKind(kind, root, configDigest, lockDigest, routingMode string, ownership []string) (RunStore, *InstallRun, error) {
	if err := (RunStore{Dir: root}).ensureDir(); err != nil {
		return RunStore{}, nil, err
	}
	id, err := NewRunID()
	if err != nil {
		return RunStore{}, nil, err
	}
	store := RunStore{Dir: filepath.Join(root, id)}
	run, err := store.CreateRun(kind, id, configDigest, lockDigest, routingMode, ownership)
	return store, run, err
}

// ListRunHistory is read-only and only returns real private run directories.
// An unreadable record is an error rather than silently disappearing from history.
func ListRunHistory(root string) ([]RunSummary, error) {
	result := []RunSummary{}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return result, nil
	}
	if err := (RunStore{Dir: root}).validateDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("cannot read installation history")
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: symlink in install history", RunReasonStateUnsafe)
		}
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		run, err := LoadRunState(dir)
		if err != nil {
			return nil, fmt.Errorf("cannot read installation history entry %q: %w", SafeReportText(entry.Name()), err)
		}
		result = append(result, RunSummary{RunID: SafeReportText(run.RunID), Directory: dir,
			Status: SafeReportText(run.Status), RoutingMode: SafeReportText(run.RoutingMode),
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, ConfigDigest: run.ConfigDigest, LockDigest: run.LockDigest})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].RunID < result[j].RunID
		}
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result, nil
}

// FormatProgressHuman renders one progress snapshot without credentials.
func FormatProgressHuman(w io.Writer, progress RunProgress) {
	fmt.Fprintf(w, "run=%s state=%s completed=%d/%d elapsed=%ds\n", progress.RunID, progress.State, progress.Completed, progress.Total, progress.Elapsed)
	for _, phase := range progress.Phases {
		mark := "?"
		switch phase.State {
		case PhaseSucceededState:
			mark = "OK"
		case PhaseFailedState:
			mark = "FAIL"
		case PhaseBlockedState:
			mark = "BLOCKED"
		case PhaseRunningState:
			mark = "..."
		case PhasePendingState:
			mark = "pending"
		}
		reason := ""
		if phase.ReasonCode != "" {
			reason = " reason=" + phase.ReasonCode
		}
		fmt.Fprintf(w, "  %-20s %-7s%s\n", phase.Name, mark, reason)
	}
}

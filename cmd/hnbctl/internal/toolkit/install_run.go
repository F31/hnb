package toolkit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"
)

const InstallRunSchema = "install-run/v1"

// Phase states recorded by the run store. These are the *state strings* of a
// phase, distinct from the PhaseName identifiers in receipt.go.
const (
	PhasePendingState   = "pending"
	PhaseRunningState   = "running"
	PhaseSucceededState = "succeeded"
	PhaseBlockedState   = "blocked"
	PhaseFailedState    = "failed"
)

// Reason codes for run-store refusals. These are stable machine-readable
// outcomes, not free-form logs.
const (
	RunReasonLockChanged     = "RUN_LOCK_CHANGED"
	RunReasonConfigChange    = "RUN_CONFIG_CHANGED"
	RunReasonIdentityChanged = "RUN_IDENTITY_CHANGED"
	RunReasonStateUnsafe     = "RUN_STATE_UNSAFE"
	RunReasonWriterBusy      = "RUN_WRITER_BUSY"
	RunReasonFenced          = "RUN_WRITER_FENCED"
	RunReasonPhaseState      = "RUN_PHASE_STATE_INVALID"
	RunReasonPrecond         = "RUN_PRECONDITION_FAILED"
	RunReasonPostcond        = "RUN_POSTCONDITION_FAILED"
	RunReasonNoReset         = "RUN_NO_RESET_PATH"
	RunReasonNeverRerun      = "RUN_PHASE_NEVER_RERUN"
	RunReasonTerminalState   = "RUN_TERMINAL_STATE"
)

const (
	// Bounded wait for the run lock so a stalled pre/post condition surfaces as an
	// error instead of hanging every other writer.
	runLockTimeout      = 30 * time.Second
	runLockPollInterval = 25 * time.Millisecond
)

// Condition is a caller-supplied pre/post check. The store never interprets a
// remote fact itself; it only records the verified outcome. Returning a false
// ConditionResult with a reason makes the transition fail closed.
type Condition func(ctx context.Context) (ok bool, reason string, err error)

// RunPhase records one phase inside a run.
type RunPhase struct {
	Name       PhaseName `json:"name"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	ReasonCode string    `json:"reasonCode,omitempty"`
	Evidence   []string  `json:"evidence,omitempty"`
	Epoch      int64     `json:"epoch"`
}

// InstallRun is the durable, permission-restricted local record. It is an
// attachment for handoff, never the platform Operation store.
type InstallRun struct {
	SchemaVersion        string               `json:"schemaVersion"`
	Kind                 string               `json:"kind,omitempty"`
	RunID                string               `json:"runId"`
	ConfigDigest         string               `json:"configDigest"`
	LockDigest           string               `json:"lockDigest"`
	RoutingMode          string               `json:"routingMode"`
	Ownership            []string             `json:"ownership"`
	ManagementClusterUID string               `json:"managementClusterUID,omitempty"`
	Epoch                int64                `json:"epoch"`
	WriterID             string               `json:"writerId"`
	LeaseExpires         time.Time            `json:"leaseExpires"`
	Status               string               `json:"status"`
	StartedAt            time.Time            `json:"startedAt"`
	FinishedAt           time.Time            `json:"finishedAt,omitempty"`
	Phases               map[string]*RunPhase `json:"phases"`
	Events               []RunEvent           `json:"events,omitempty"`
}

// phasesNeverRerun are one-shot phases. Resume must never re-execute them; a
// resume that needs them but cannot prove their postcondition must refuse. Once
// such a phase has begun (or already completed) it can never be run again.
var phasesNeverRerun = map[PhaseName]bool{
	PhaseInit: true, PhaseJoin: true, PhaseClusterVerified: true, PhaseDatabaseMigrate: true,
}

// RunStore persists a run under a private directory (0700) with a private state
// file (0600). It refuses symlinked or world/group-readable state.
type RunStore struct {
	Dir     string
	NowFunc func() time.Time
}

func (s RunStore) now() time.Time {
	if s.NowFunc != nil {
		return s.NowFunc()
	}
	return time.Now().UTC()
}

func (s RunStore) statePath() string { return filepath.Join(s.Dir, "run-state.json") }

// ensureDir creates the private run directory and refuses anything else.
// It is used only by mutating operations that are allowed to create the store.
func (s RunStore) ensureDir() error {
	if s.Dir == "" {
		return fmt.Errorf("run directory is required")
	}
	if fi, err := os.Lstat(s.Dir); err == nil {
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm() != 0o700 {
			return fmt.Errorf("%s: run directory must be a private real directory (0700)", RunReasonStateUnsafe)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("run directory unavailable")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("cannot create run directory")
	}
	return nil
}

// validateDir is the read-only counterpart of ensureDir: it verifies a private
// directory exists but never creates it, so a status read has no side effect.
func (s RunStore) validateDir() error {
	if s.Dir == "" {
		return fmt.Errorf("run directory is required")
	}
	if _, err := os.Lstat(s.Dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("run directory does not exist")
		}
		return fmt.Errorf("run directory unavailable")
	}
	return s.ensureDir()
}

// withLock serialises every read-modify-write so concurrent consumers cannot
// both pass an in-memory check and both write. The state file is only ever
// mutated while the exclusive lock is held.
func (s RunStore) withLock(fn func() error) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, "run-state.lock")
	// A pre-planted symlink must not become the lock that guards every write, and a
	// laxer lock mode means the guard can be bypassed.
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
			return fmt.Errorf("%s: run lock must be a private regular file (0600)", RunReasonStateUnsafe)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("run lock unavailable")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("run lock unavailable")
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		return fmt.Errorf("%s: run lock must be a private regular file (0600)", RunReasonStateUnsafe)
	}
	if err := acquireRunLock(f); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// acquireRunLock takes the exclusive lock within a bounded wait. A blocking flock
// would let one slow pre/post condition hang every other writer with no output, so
// the wait is explicit and a timeout is reported instead of hanging.
func acquireRunLock(f *os.File) error {
	deadline := time.Now().Add(runLockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return fmt.Errorf("run lock unavailable")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: timed out after %s waiting for the run lock", RunReasonWriterBusy, runLockTimeout)
		}
		time.Sleep(runLockPollInterval)
	}
}

func (s RunStore) read() (*InstallRun, error) {
	fi, err := os.Lstat(s.statePath())
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("%s: run state must be a private regular file (0600)", RunReasonStateUnsafe)
	}
	data, err := os.ReadFile(s.statePath())
	if err != nil {
		return nil, err
	}
	var run InstallRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("%s: run state is unreadable", RunReasonStateUnsafe)
	}
	if run.Phases == nil {
		run.Phases = map[string]*RunPhase{}
	}
	return &run, nil
}

func (s RunStore) write(run *InstallRun) error {
	// State and its transition journal share one atomic checkpoint, so a crash
	// cannot commit a phase without its log (or a log without its phase).
	if err := s.recordTransitions(run); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	// Write through a freshly created private temp file and fsync it, so a crash
	// cannot leave a renamed-but-empty state file behind.
	tmp, err := os.CreateTemp(s.Dir, "run-state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.statePath()); err != nil {
		return err
	}
	// Persist the rename itself, otherwise a power loss can resurrect the old name.
	dir, err := os.Open(s.Dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Create starts a new run bound to immutable config/lock digests. If a run for
// the same location already exists, the entire identity (lock, config, run id,
// routing mode, ownership) is compared: any change is refused so a committed
// plan can never be silently swapped. Invalid state is never replaced by a new
// run; it must be repaired by the operator first.
func (s RunStore) Create(runID, configDigest, lockDigest, routingMode string, ownership []string) (*InstallRun, error) {
	return s.CreateRun("install", runID, configDigest, lockDigest, routingMode, ownership)
}

// CreateRun starts a new run of the given kind ("install" or "upgrade"). The
// kind is part of the run identity, so a resume cannot swap an upgrade for an
// install behind the same run id.
func (s RunStore) CreateRun(kind, runID, configDigest, lockDigest, routingMode string, ownership []string) (*InstallRun, error) {
	if kind == "" {
		kind = "install"
	}
	if runID == "" {
		return nil, fmt.Errorf("run id is required")
	}
	var result *InstallRun
	err := s.withLock(func() error {
		existing, readErr := s.read()
		if readErr == nil {
			if existing.Kind != "" && existing.Kind != kind {
				return fmt.Errorf("%s: resume requires the original run kind", RunReasonIdentityChanged)
			}
			if existing.LockDigest != lockDigest {
				return fmt.Errorf("%s: resume requires the original release lock", RunReasonLockChanged)
			}
			if existing.ConfigDigest != configDigest {
				return fmt.Errorf("%s: resume requires the original configuration", RunReasonConfigChange)
			}
			if existing.RunID != runID {
				return fmt.Errorf("%s: resume requires the original run id", RunReasonIdentityChanged)
			}
			if existing.RoutingMode != routingMode {
				return fmt.Errorf("%s: resume requires the original routing mode", RunReasonIdentityChanged)
			}
			if !slices.Equal(existing.Ownership, ownership) {
				return fmt.Errorf("%s: resume requires the original ownership boundary", RunReasonIdentityChanged)
			}
			result = existing
			return nil
		}
		if !os.IsNotExist(readErr) {
			return fmt.Errorf("%s: refusing to overwrite invalid run state: %v", RunReasonStateUnsafe, readErr)
		}
		run := &InstallRun{
			SchemaVersion: InstallRunSchema, Kind: kind, RunID: runID, ConfigDigest: configDigest,
			LockDigest: lockDigest, RoutingMode: routingMode, Ownership: ownership,
			Epoch: 0, Status: PhaseRunningState, StartedAt: s.now(), Phases: map[string]*RunPhase{},
		}
		if err := s.write(run); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AcquireWriter takes the single-writer lease and issues a fencing epoch. A
// second, concurrent writer is refused. The epoch advances whenever the writer
// changes OR the previous lease has expired, so a long-stalled writer can never
// reuse its token after another writer took over.
func (s RunStore) AcquireWriter(writerID string, ttl time.Duration) (*InstallRun, error) {
	var result *InstallRun
	err := s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		// Taking a lease is itself a state mutation, so a completed run refuses it.
		if err := s.assertMutable(run); err != nil {
			return err
		}
		now := s.now()
		if run.WriterID != "" && run.WriterID != writerID && now.Before(run.LeaseExpires) {
			return fmt.Errorf("%s: another writer holds the run lease", RunReasonWriterBusy)
		}
		reEnter := run.WriterID == writerID && now.Before(run.LeaseExpires)
		if !reEnter {
			run.Epoch++ // fencing token advances on any writer change or lease expiry
		}
		run.WriterID = writerID
		run.LeaseExpires = now.Add(ttl)
		if err := s.write(run); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// assertWriter verifies the caller still holds the current, unexpired fencing
// epoch. A writer whose lease was taken over or has expired cannot mutate state.
func (s RunStore) assertWriter(run *InstallRun, writerID string, epoch int64) error {
	if run.WriterID != writerID || run.Epoch != epoch {
		return fmt.Errorf("%s: writer fenced by a newer lease", RunReasonFenced)
	}
	// The lease is exclusive of its expiry instant, so the boundary fails closed.
	if !s.now().Before(run.LeaseExpires) {
		return fmt.Errorf("%s: writer lease expired; re-acquire the writer and re-verify", RunReasonFenced)
	}
	return nil
}

// assertMutable refuses any mutation of a completed run.
func (s RunStore) assertMutable(run *InstallRun) error {
	if run.Status == PhaseSucceededState {
		return fmt.Errorf("%s: run already completed; no mutation", RunReasonTerminalState)
	}
	return nil
}

// BindManagementCluster pins the observed cluster as soon as init returns. A
// resumed writer cannot substitute a different cluster behind the same node.
func (s RunStore) BindManagementCluster(writerID string, epoch int64, uid string) error {
	if !validName(uid) {
		return fmt.Errorf("%s: cluster UID is required", RunReasonIdentityChanged)
	}
	return s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		if err := s.assertWriter(run, writerID, epoch); err != nil {
			return err
		}
		if err := s.assertMutable(run); err != nil {
			return err
		}
		if phase := run.Phases[string(PhaseInit)]; phase == nil || phase.State != PhaseRunningState {
			return fmt.Errorf("%s: cluster identity can only be bound during init", RunReasonPhaseState)
		}
		if run.ManagementClusterUID != "" && run.ManagementClusterUID != uid {
			return fmt.Errorf("%s: management cluster UID changed", RunReasonIdentityChanged)
		}
		run.ManagementClusterUID = uid
		return s.write(run)
	})
}

func (s RunStore) phase(run *InstallRun, name PhaseName) *RunPhase {
	if run.Phases == nil {
		run.Phases = map[string]*RunPhase{}
	}
	p, ok := run.Phases[string(name)]
	if !ok {
		p = &RunPhase{Name: name, State: PhasePendingState}
		run.Phases[string(name)] = p
	}
	return p
}

// BeginPhase verifies the precondition and transitions pending/failed->running.
// A never-rerun phase that has started or completed can never re-enter.
func (s RunStore) BeginPhase(writerID string, epoch int64, name PhaseName, pre Condition) (*InstallRun, error) {
	var result *InstallRun
	err := s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		if err := s.assertWriter(run, writerID, epoch); err != nil {
			return err
		}
		if err := s.assertMutable(run); err != nil {
			return err
		}
		p := s.phase(run, name)
		if phasesNeverRerun[name] && p.State != PhasePendingState {
			return fmt.Errorf("%s: %s already started or completed and is never re-executed", RunReasonNeverRerun, name)
		}
		if pre != nil {
			ok, reason, perr := pre(context.Background())
			if perr != nil || !ok {
				p.State, p.ReasonCode = PhaseBlockedState, RunReasonPrecond
				if reason != "" {
					p.ReasonCode = reason
				}
				_ = s.write(run)
				if perr != nil {
					return fmt.Errorf("%s: precondition error: %v", RunReasonPrecond, perr)
				}
				return fmt.Errorf("%s: %s precondition failed (%s)", RunReasonPrecond, name, reason)
			}
		}
		p.State, p.StartedAt, p.Attempts, p.Epoch = PhaseRunningState, s.now(), p.Attempts+1, epoch
		if err := s.write(run); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CompletePhase verifies the postcondition before recording success. A phase is
// only "succeeded" when its postcondition is independently verified.
func (s RunStore) CompletePhase(writerID string, epoch int64, name PhaseName, post Condition, evidence []string) (*InstallRun, error) {
	var result *InstallRun
	err := s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		if err := s.assertWriter(run, writerID, epoch); err != nil {
			return err
		}
		if err := s.assertMutable(run); err != nil {
			return err
		}
		p := s.phase(run, name)
		if p.State != PhaseRunningState {
			return fmt.Errorf("%s: %s is not running", RunReasonPhaseState, name)
		}
		if post != nil {
			ok, reason, perr := post(context.Background())
			if perr != nil || !ok {
				p.State, p.ReasonCode = PhaseFailedState, RunReasonPostcond
				if reason != "" {
					p.ReasonCode = reason
				}
				_ = s.write(run)
				if perr != nil {
					return fmt.Errorf("%s: postcondition error: %v", RunReasonPostcond, perr)
				}
				return fmt.Errorf("%s: %s postcondition failed (%s)", RunReasonPostcond, name, reason)
			}
		}
		p.State, p.FinishedAt, p.Evidence = PhaseSucceededState, s.now(), evidence
		if err := s.write(run); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FailPhase records a failure without inventing success. It never resets a
// cluster: there is intentionally no reset verb in this store.
func (s RunStore) FailPhase(writerID string, epoch int64, name PhaseName, reason string) (*InstallRun, error) {
	var result *InstallRun
	err := s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		if err := s.assertWriter(run, writerID, epoch); err != nil {
			return err
		}
		if err := s.assertMutable(run); err != nil {
			return err
		}
		p := s.phase(run, name)
		p.State, p.FinishedAt, p.ReasonCode = PhaseFailedState, s.now(), reason
		if err := s.write(run); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ResumeAction tells a resumed run how to treat an already-recorded phase.
type ResumeAction struct {
	Phase  PhaseName `json:"phase"`
	Action string    `json:"action"` // skip_after_recheck | recheck_or_refuse | execute | none
	Reason string    `json:"reason,omitempty"`
}

// PlanResume classifies each phase. Never-rerun phases are never executed again,
// whether they failed, were blocked, are in flight or already succeeded; their
// postcondition must be rechecked or the resume refuses. Succeeded idempotent
// phases become skip_after_recheck. There is no reset action by construction.
func (s RunStore) PlanResume(run *InstallRun, order []PhaseName) []ResumeAction {
	var actions []ResumeAction
	for _, name := range order {
		p := run.Phases[string(name)]
		neverRerun := phasesNeverRerun[name]
		switch {
		case p == nil || p.State == PhasePendingState:
			actions = append(actions, ResumeAction{Phase: name, Action: "execute"})
		case (p.State == PhaseFailedState || p.State == PhaseBlockedState || p.State == PhaseRunningState) && neverRerun:
			actions = append(actions, ResumeAction{Phase: name, Action: "recheck_or_refuse", Reason: "never re-executed; precondition must be re-verified before trusting"})
		case (p.State == PhaseFailedState || p.State == PhaseBlockedState || p.State == PhaseRunningState):
			actions = append(actions, ResumeAction{Phase: name, Action: "execute", Reason: p.ReasonCode})
		case p.State == PhaseSucceededState && neverRerun:
			actions = append(actions, ResumeAction{Phase: name, Action: "recheck_or_refuse", Reason: "never re-executed; precondition must be re-verified before trusting"})
		case p.State == PhaseSucceededState:
			actions = append(actions, ResumeAction{Phase: name, Action: "skip_after_recheck"})
		default:
			// An unrecognized state (empty, or written by a newer version) must not
			// silently execute: the recorded precondition cannot be trusted.
			actions = append(actions, ResumeAction{
				Phase:  name,
				Action: "recheck_or_refuse",
				Reason: fmt.Sprintf("unrecognized phase state %q; refusing to execute without re-verification", p.State),
			})
		}
	}
	return actions
}

// Finalize records the terminal state. It refuses to mark the run succeeded
// while any required phase is not succeeded.
func (s RunStore) Finalize(writerID string, epoch int64, status string, required []PhaseName) (*InstallRun, error) {
	var result *InstallRun
	err := s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		if err := s.assertWriter(run, writerID, epoch); err != nil {
			return err
		}
		if err := s.assertMutable(run); err != nil {
			return err
		}
		if status == PhaseSucceededState {
			for _, name := range required {
				if p := run.Phases[string(name)]; p == nil || p.State != PhaseSucceededState {
					return fmt.Errorf("%s: %s not succeeded; run cannot be finalized as succeeded", RunReasonPhaseState, name)
				}
			}
		}
		run.Status, run.FinishedAt = status, s.now()
		if err := s.write(run); err != nil {
			return err
		}
		result = run
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// NewRunID returns a random opaque run identifier (no customer data).
func NewRunID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// LoadRunState reads a durable run after verifying the directory is private.
// It never creates the directory or any file.
func LoadRunState(dir string) (*InstallRun, error) {
	store := RunStore{Dir: dir}
	if err := store.validateDir(); err != nil {
		return nil, err
	}
	run, err := store.read()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no install run state in %s: %w", dir, err)
		}
		return nil, err
	}
	if run.SchemaVersion != InstallRunSchema {
		return nil, fmt.Errorf("unsupported run state schema")
	}
	return run, nil
}

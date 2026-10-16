package toolkit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func upgradeRunFixture(t *testing.T) (RunStore, string, int64) {
	t.Helper()
	store := RunStore{Dir: filepath.Join(t.TempDir(), "run")}
	if _, err := store.CreateRun("upgrade", "upgrade-1", "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), "external-cluster cluster.reader/shadow", []string{"namespace/hnb-system"}); err != nil {
		t.Fatal(err)
	}
	active, err := store.AcquireWriter("w", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return store, "w", active.Epoch
}

func authorizedUpgradePlan(reversibleMigration bool) UpgradePlan {
	return UpgradePlan{
		SchemaVersion: ProfileUpgradePlanSchema, From: "Minimal", To: "LiteHA",
		FromDigest: "sha256:from", ToDigest: "sha256:to", Authorized: true,
		Steps: []UpgradeStep{
			{Kind: "components", Target: "apiserver", From: "1", To: "3", Reversible: true},
			{Kind: "migration", Target: "platform/001", Reversible: reversibleMigration},
			{Kind: "release", Target: "LiteHA", Reversible: true},
			{Kind: "verify", Target: "LiteHA", Reversible: true},
		},
	}
}

func TestRunUpgradeRefusesBlockedPlan(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	plan := authorizedUpgradePlan(true)
	plan.Authorized, plan.Blocked = false, []string{"TARGET_PROFILE_NOT_CERTIFIED: LiteHA"}
	if _, err := RunUpgrade(context.Background(), UpgradeExecutionInput{Store: store, Writer: writer, Epoch: epoch, Plan: plan}); err == nil || !strings.Contains(err.Error(), "UPGRADE_PLAN_BLOCKED") {
		t.Fatalf("blocked plan executed: %v", err)
	}
}

func TestRunUpgradeRefusesIrreversibleWithoutAck(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	_, err := RunUpgrade(context.Background(), UpgradeExecutionInput{Store: store, Writer: writer, Epoch: epoch, Plan: authorizedUpgradePlan(false)})
	if err == nil || !strings.Contains(err.Error(), "UPGRADE_IRREVERSIBLE_UNACKNOWLEDGED") {
		t.Fatalf("irreversible migration not gated: %v", err)
	}
	state, loadErr := LoadRunState(store.Dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(state.Phases) != 0 {
		t.Fatalf("a refused plan must not record phases: %+v", state.Phases)
	}
}

func TestRunUpgradeBackupGateFailsClosed(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	_, err := RunUpgrade(context.Background(), UpgradeExecutionInput{
		Store: store, Writer: writer, Epoch: epoch, Plan: authorizedUpgradePlan(false),
		AllowIrreversible: true, RequiresBackup: true, BackupVerified: false,
		Steps: UpgradeSteps{Migrate: func(context.Context) error { return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "BACKUP_REQUIRED") {
		t.Fatalf("missing backup not enforced: %v", err)
	}
	state, _ := LoadRunState(store.Dir)
	if state.Status != PhaseBlockedState {
		t.Fatalf("run should be blocked: %s", state.Status)
	}
	if p := state.Phases[string(PhaseDatabaseMigrate)]; p != nil {
		t.Fatalf("migration must not run without a backup: %+v", p)
	}
}

func TestRunUpgradeSuccessRecordsOrderedPhases(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	var order []string
	record := func(name string) func(context.Context) error {
		return func(context.Context) error { order = append(order, name); return nil }
	}
	res, err := RunUpgrade(context.Background(), UpgradeExecutionInput{
		Store: store, Writer: writer, Epoch: epoch, Plan: authorizedUpgradePlan(true),
		Steps: UpgradeSteps{
			Preflight: record("preflight"), Backup: record("backup"), Infra: record("infra"),
			Migrate: record("migrate"), Release: record("release"), Verify: record("verify"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PhaseSucceededState {
		t.Fatalf("upgrade not succeeded: %s", res.Status)
	}
	state, _ := LoadRunState(store.Dir)
	if state.Status != PhaseSucceededState || state.Kind != "upgrade" {
		t.Fatalf("run not finalized as upgrade: %+v", state)
	}
	for _, name := range []PhaseName{PhasePreflight, PhaseBackup, PhaseInfraDelta, PhaseDatabaseMigrate, PhasePlatformRelease, PhasePlatformVerified} {
		if p := state.Phases[string(name)]; p == nil || p.State != PhaseSucceededState {
			t.Fatalf("phase %s not succeeded: %+v", name, p)
		}
	}
}

func TestRunUpgradeResumeSkipsSucceededPhase(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	// Simulate a prior run that already completed preflight and infra-delta.
	if _, err := store.BeginPhase(writer, epoch, PhasePreflight, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase(writer, epoch, PhasePreflight, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginPhase(writer, epoch, PhaseInfraDelta, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase(writer, epoch, PhaseInfraDelta, nil, nil); err != nil {
		t.Fatal(err)
	}
	infraCalls, migrateCalls := 0, 0
	_, err := RunUpgrade(context.Background(), UpgradeExecutionInput{
		Store: store, Writer: writer, Epoch: epoch, Plan: authorizedUpgradePlan(true),
		Steps: UpgradeSteps{
			Infra:   func(context.Context) error { infraCalls++; return nil },
			Migrate: func(context.Context) error { migrateCalls++; return nil },
			Release: func(context.Context) error { return nil },
			Verify:  func(context.Context) error { return nil },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if infraCalls != 0 {
		t.Fatalf("succeeded infra-delta must be skipped, ran %d times", infraCalls)
	}
	if migrateCalls != 1 {
		t.Fatalf("migration must run once on resume, ran %d times", migrateCalls)
	}
}

func TestRunUpgradeOneShotFailedPhaseRefusesRerun(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	if _, err := store.FailPhase(writer, epoch, PhaseDatabaseMigrate, "MIGRATION_APPLY_FAILED"); err != nil {
		t.Fatal(err)
	}
	_, err := RunUpgrade(context.Background(), UpgradeExecutionInput{
		Store: store, Writer: writer, Epoch: epoch, Plan: authorizedUpgradePlan(true),
		Steps: UpgradeSteps{Migrate: func(context.Context) error { return nil }, Release: func(context.Context) error { return nil }, Verify: func(context.Context) error { return nil }},
	})
	if err == nil || !strings.Contains(err.Error(), "UPGRADE_PHASE_NOT_RERUNNABLE") {
		t.Fatalf("failed one-shot phase re-run: %v", err)
	}
}

func TestPlanRollbackBlocksIrreversibleAndReversesSteps(t *testing.T) {
	if _, blocks := PlanRollback(authorizedUpgradePlan(false)); len(blocks) == 0 {
		t.Fatal("irreversible migration rollback must be blocked")
	}
	rollback, blocks := PlanRollback(authorizedUpgradePlan(true))
	if len(blocks) != 0 || !rollback.Authorized {
		t.Fatalf("reversible rollback should be authorized: %v", blocks)
	}
	if rollback.From != "LiteHA" || rollback.To != "Minimal" {
		t.Fatalf("rollback direction wrong: %s -> %s", rollback.From, rollback.To)
	}
	first := rollback.Steps[0]
	last := rollback.Steps[len(rollback.Steps)-1]
	if first.Kind != "verify" || last.Kind != "components" {
		t.Fatalf("rollback steps not reversed: %+v", rollback.Steps)
	}
}

func TestRunUpgradeRollbackRefusesIrreversible(t *testing.T) {
	store, writer, epoch := upgradeRunFixture(t)
	_, err := RunUpgradeRollback(context.Background(), UpgradeExecutionInput{Store: store, Writer: writer, Epoch: epoch, Plan: authorizedUpgradePlan(false)})
	if err == nil || !strings.Contains(err.Error(), "ROLLBACK_REFUSED") {
		t.Fatalf("irreversible rollback not refused: %v", err)
	}
}

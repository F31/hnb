package toolkit

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunDatabaseMigrationsOrderAndFailClosed(t *testing.T) {
	planes := []MigrationPlane{
		{Owner: OwnerPlatform, MigrationsDir: "/p", DSN: "postgres://secret-platform"},
		{Owner: OwnerMarket, MigrationsDir: "/m", DSN: "postgres://secret-market"},
		{Owner: OwnerArtifact, MigrationsDir: "/a", DSN: "postgres://secret-artifact"},
	}
	var seen []MigrationOwner
	applier := func(_ context.Context, plane MigrationPlane) (MigrationOutcomeReport, error) {
		seen = append(seen, plane.Owner)
		if plane.Owner == OwnerMarket {
			return MigrationOutcomeReport{}, errors.New("boom")
		}
		return MigrationOutcomeReport{Owner: string(plane.Owner), Applied: 2, Skipped: 1}, nil
	}
	reports, err := RunDatabaseMigrations(context.Background(), planes, applier)
	if err == nil {
		t.Fatal("partial failure not propagated")
	}
	if len(reports) != 1 || reports[0].Owner != string(OwnerPlatform) || reports[0].Applied != 2 {
		t.Fatalf("partial reports wrong: %+v", reports)
	}
	if strings.Join([]string{string(seen[0]), string(seen[1])}, ",") != "platform,market" {
		t.Fatalf("planes not applied in order: %v", seen)
	}
	// 失败错误不得泄露任何 DSN.
	for _, dsn := range []string{"secret-platform", "secret-market", "secret-artifact"} {
		if strings.Contains(err.Error(), dsn) {
			t.Fatalf("migration error leaked DSN: %v", err)
		}
	}
}

// 输入校验在打开数据库之前发生.
func TestRunDatabaseMigrationsValidatesInputBeforeConnecting(t *testing.T) {
	_, err := RunDatabaseMigrations(context.Background(), []MigrationPlane{{Owner: OwnerPlatform, DSN: "postgres://x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "MIGRATION_INPUT_INVALID") {
		t.Fatalf("empty migrations dir not refused: %v", err)
	}
}

func TestRunInstallMigrationsGatedAndRecorded(t *testing.T) {
	cfg := existingApplierConfig()
	planes := []MigrationPlane{{Owner: OwnerPlatform, MigrationsDir: "/p", DSN: "postgres://secret"}}

	// 门禁未认证：拒绝且不调用 applier.
	store, writer, epoch := newApplierRun(t)
	called := false
	applier := func(_ context.Context, plane MigrationPlane) (MigrationOutcomeReport, error) {
		called = true
		return MigrationOutcomeReport{Owner: string(plane.Owner)}, nil
	}
	if _, err := RunInstallMigrations(context.Background(), store, writer, epoch, cfg, planes, applier, blockedMatrix()); err == nil || !strings.Contains(err.Error(), "PLATFORM_NOT_CERTIFIED") {
		t.Fatalf("uncertified migrations not refused: %v", err)
	}
	if called {
		t.Fatal("applier ran while gated")
	}

	// 已认证：应用并记录 database-migration 阶段.
	store, writer, epoch = newApplierRun(t)
	reports, err := RunInstallMigrations(context.Background(), store, writer, epoch, cfg, planes, applier, certifiedMatrix())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Owner != string(OwnerPlatform) {
		t.Fatalf("reports wrong: %+v", reports)
	}
	run, err := LoadRunState(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if state := run.Phases[string(PhaseDatabaseMigrate)]; state == nil || state.State != PhaseSucceededState {
		t.Fatalf("database-migration phase not recorded: %+v", state)
	}
}

func TestMigrationOwnerForPlane(t *testing.T) {
	cases := map[string]MigrationOwner{"platform": OwnerPlatform, "market": OwnerMarket, "artifact": OwnerArtifact}
	for name, want := range cases {
		got, ok := MigrationOwnerForPlane(name)
		if !ok || got != want {
			t.Fatalf("plane %s -> %s/%v", name, got, ok)
		}
	}
	if _, ok := MigrationOwnerForPlane("unknown"); ok {
		t.Fatal("unknown plane accepted")
	}
}

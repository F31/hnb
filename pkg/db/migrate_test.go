package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// These tests run against a real PostgreSQL instance. Templates, checksums and
// locks are only meaningful against a real server, so a fake would prove nothing.
func migrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("HNB_MIGRATION_E2E_DSN")
	if dsn == "" {
		t.Skip("set HNB_MIGRATION_E2E_DSN to run migration tests against a real PostgreSQL")
	}
	return dsn
}

// freshDatabase gives each test its own database, so ledger and schema state
// cannot leak between cases and the advisory lock is per-database.
func freshDatabase(t *testing.T) *sql.DB {
	t.Helper()
	admin, err := sql.Open("postgres", migrationDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("mig_test_%x", suffix)
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	db := openTestDatabase(t, name)
	t.Cleanup(func() {
		db.Close()
		cleanup, err := sql.Open("postgres", migrationDSN(t))
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
	})
	return db
}

// openTestDatabase returns a pool bound to the per-test database.
func openTestDatabase(t *testing.T, name string) *sql.DB {
	t.Helper()
	base := migrationDSN(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	return db
}

// writeMigration creates a small forward migration that is real DDL.
func writeMigration(t *testing.T, dir string, version int, name, ddl string) {
	t.Helper()
	body := fmtSQL(ddl)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = version
}

func TestMigrationAppliesOnceAndIsIdempotent(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_create_widgets.sql",
		"BEGIN;\nCREATE TABLE widgets (id bigserial PRIMARY KEY, name text NOT NULL);\nCOMMIT;")
	writeMigration(t, dir, 2, "002_create_gadgets.sql",
		"BEGIN;\nCREATE TABLE gadgets (id bigserial PRIMARY KEY);\nCOMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Migrator{DB: db, Owner: OwnerPlatform}

	outcome, err := m.Apply(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Applied) != 2 || len(outcome.Skipped) != 0 {
		t.Fatalf("first run should apply both: %+v", outcome)
	}
	if outcome.SchemaBefore == outcome.SchemaAfter {
		t.Fatal("schema fingerprint did not change after applying migrations")
	}

	// A repeated install must be a no-op, not a second application.
	again, err := m.Apply(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Applied) != 0 || len(again.Skipped) != 2 {
		t.Fatalf("repeat run re-applied migrations: %+v", again)
	}
	if again.SchemaBefore != again.SchemaAfter {
		t.Fatal("no-op run changed the schema")
	}
}

func TestMigrationChecksumDriftBlocks(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_create_widgets.sql",
		"BEGIN;\nCREATE TABLE widgets (id bigserial PRIMARY KEY);\nCOMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Migrator{DB: db, Owner: OwnerPlatform}
	if _, err := m.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	// Same version, different bytes: the schema and the code now disagree.
	writeMigration(t, dir, 1, "001_create_widgets.sql",
		"BEGIN;\nCREATE TABLE widgets (id bigserial PRIMARY KEY, extra text);\nCOMMIT;")
	drifted, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), drifted); err == nil {
		t.Fatal("checksum drift was accepted")
	}
	problems, matched, err := m.Verify(context.Background(), drifted)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 0 || len(problems) != 1 {
		t.Fatalf("unexpected verification result: %v %+v", problems, matched)
	}
}

// Two installers must not interleave schema changes for one owner.
func TestConcurrentMigrationIsMutuallyExclusive(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		writeMigration(t, dir, i, fmt.Sprintf("%03d_step.sql", i),
			fmt.Sprintf("BEGIN;\nCREATE TABLE step_%d (id bigserial PRIMARY KEY);\nCOMMIT;", i))
	}
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Two independent connections, i.e. two installers.
	var database string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	other := openTestDatabase(t, database)
	defer other.Close()

	first := Migrator{DB: db, Owner: OwnerPlatform}
	second := Migrator{DB: other, Owner: OwnerPlatform}

	var wg sync.WaitGroup
	results := make([]*MigrationOutcome, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results[0], errs[0] = first.Apply(context.Background(), files) }()
	go func() { defer wg.Done(); results[1], errs[1] = second.Apply(context.Background(), files) }()
	wg.Wait()

	applied := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("installer %d failed instead of observing the same database ledger: %v", i, err)
		}
		applied += len(results[i].Applied)
	}
	if applied != 5 {
		t.Fatalf("migrations were applied %d times, want exactly 5 across both installers", applied)
	}
	// Every migration is recorded exactly once and every table exists once.
	var recorded int
	if err := db.QueryRow(`SELECT count(*) FROM platform_schema_migrations`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 5 {
		t.Fatalf("ledger recorded %d migrations, want 5", recorded)
	}
	for i := 1; i <= 5; i++ {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`,
			"step_"+itoaTest(i)).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table step_%d exists %d times", i, count)
		}
	}
}

// A failing migration must leave the schema at the last fully applied version.
func TestInterruptedMigrationLeavesSchemaConsistent(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_create_widgets.sql",
		"BEGIN;\nCREATE TABLE widgets (id bigserial PRIMARY KEY);\nCOMMIT;")
	writeMigration(t, dir, 2, "002_invalid.sql",
		"BEGIN;\nCREATE TABLE ok_before_failure (id bigserial PRIMARY KEY);\nTHIS IS NOT SQL;\nCOMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Migrator{DB: db, Owner: OwnerPlatform}
	if _, err := m.Apply(context.Background(), files); err == nil {
		t.Fatal("invalid migration was accepted")
	}
	// The first migration stands; the failed one left nothing behind.
	var widgets int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='widgets'`).Scan(&widgets); err != nil {
		t.Fatal(err)
	}
	if widgets != 1 {
		t.Fatal("successful migration was rolled back with the failing one")
	}
	var partial int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='ok_before_failure'`).Scan(&partial); err != nil {
		t.Fatal(err)
	}
	if partial != 0 {
		t.Fatal("failed migration left partial schema behind")
	}
	var recorded int
	if err := db.QueryRow(`SELECT count(*) FROM platform_schema_migrations`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("ledger recorded %d migrations, want only the successful one", recorded)
	}
	// The lock must be released so a corrected retry can proceed.
	writeMigration(t, dir, 2, "002_invalid.sql",
		"BEGIN;\nCREATE TABLE ok_before_failure (id bigserial PRIMARY KEY);\nCOMMIT;")
	fixed, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := m.Apply(context.Background(), fixed)
	if err != nil {
		t.Fatalf("retry after a fixed migration failed: %v", err)
	}
	if len(retry.Applied) != 1 || retry.Applied[0].Version != 2 {
		t.Fatalf("retry did not apply the corrected migration: %+v", retry.Applied)
	}
}

// Plane boundaries: one owner's migrator must not touch another's ledger.
func TestPlaneOwnersAreIsolated(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_market_tables.sql",
		"BEGIN;\nCREATE TABLE market_products (id bigserial PRIMARY KEY);\nCOMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	market := Migrator{DB: db, Owner: OwnerMarket}
	if _, err := market.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	// The platform owner must see no ledger for that version.
	platform := Migrator{DB: db, Owner: OwnerPlatform}
	_, matched, err := platform.Verify(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 0 {
		t.Fatalf("platform owner observed the market ledger: %+v", matched)
	}
	if ok, _, err := platform.VerifySchemaForService(context.Background(), files); err != nil || ok {
		t.Fatalf("platform reported itself ready against an empty schema: %v %v", ok, err)
	}
}

// An incompatible schema must keep the service out of readiness.
func TestIncompleteSchemaBlocksServiceReadiness(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_a.sql", "BEGIN;\nCREATE TABLE a (id int);\nCOMMIT;")
	writeMigration(t, dir, 2, "002_b.sql", "BEGIN;\nCREATE TABLE b (id int);\nCOMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Migrator{DB: db, Owner: OwnerPlatform}
	if _, err := m.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	// Simulate a newer migration the installer has not applied yet.
	writeMigration(t, dir, 3, "003_c.sql", "BEGIN;\nCREATE TABLE c (id int);\nCOMMIT;")
	newer, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	ok, problems, err := m.VerifySchemaForService(context.Background(), newer)
	if err != nil {
		t.Fatal(err)
	}
	if ok || len(problems) != 1 {
		t.Fatalf("pending migration did not block readiness: %v %v", ok, problems)
	}
}

func TestLoadMigrationsRejectsUnsafeLayout(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"widgets.sql":       "SELECT 1;",
		"abc_create.sql":    "SELECT 1;",
		"0_zero.sql":        "SELECT 1;",
		"001_dup.sql":       "SELECT 1;",
		"001_dup_again.sql": "SELECT 1;",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadMigrations(dir); err == nil {
		t.Fatal("unversioned or duplicate migrations were accepted")
	}
	// A rollback twin must never be applied as a forward step.
	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, "001_x.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "001_x.rollback.sql"), []byte("DROP TABLE x;"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := LoadMigrations(good)
	if err != nil || len(files) != 1 || files[0].Name != "001_x.sql" {
		t.Fatalf("rollback file treated as forward migration: %v %+v", err, files)
	}
}

func TestMigrationLockIsReleasedOnContextCancellation(t *testing.T) {
	db := freshDatabase(t)
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_a.sql", "BEGIN;\nCREATE TABLE a (id int);\nCOMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Migrator{DB: db, Owner: OwnerPlatform}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Apply(ctx, files); err == nil {
		t.Fatal("cancelled migration run succeeded")
	}
	// A cancelled run must not leave the owner's schema locked for the next one.
	done := make(chan error, 1)
	go func() {
		_, err := m.Apply(context.Background(), files)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("lock was not released after cancellation: %v", err)
		}
	case <-time.After(MigrationLockTimeout + 5*time.Second):
		t.Fatal("migration lock leaked after a cancelled run")
	}
}

func fmtSQL(ddl string) string { return ddl }

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealMigrationDirectoryHasUniqueVersions(t *testing.T) {
	files, err := LoadMigrations("../../database/postgresql/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 102 || files[len(files)-1].Version != 103 {
		t.Fatalf("unexpected real migration set: %d files", len(files))
	}
	for _, file := range files {
		if _, err := transactionalBody(file.SQL); err != nil {
			t.Fatalf("unsupported transaction layout in %s: %v", file.Name, err)
		}
	}
}

func TestTransactionWrapperNormalization(t *testing.T) {
	for _, body := range []string{
		"-- source header\nBEGIN; CREATE TABLE a(id int); COMMIT;",
		"/* outer /* nested */ comment */ BEGIN; DO $body$ BEGIN PERFORM 'COMMIT;'; END $body$; COMMIT;",
		"CREATE TABLE a(id int); INSERT INTO a VALUES (1);",
		`BEGIN; SELECT E'escaped\'semicolon;'; COMMIT;`,
	} {
		if _, err := transactionalBody(body); err != nil {
			t.Fatalf("valid wrapper rejected: %v", err)
		}
	}
	for _, body := range []string{"BEGIN; CREATE TABLE a(id int);", "CREATE TABLE a(id int); COMMIT;", "BEGIN; SELECT 1; COMMIT; BEGIN; SELECT 2; COMMIT;", "SELECT 'unclosed;"} {
		if _, err := transactionalBody(body); err == nil {
			t.Fatal("escaping transaction control accepted")
		}
	}
}

func TestBaselineAdoptsReviewedRowsWithoutExecutingSQL(t *testing.T) {
	db := freshDatabase(t)
	ctx := context.Background()
	m := Migrator{DB: db, Owner: OwnerPlatform}
	if _, err := db.Exec(`CREATE TABLE adopted(id int PRIMARY KEY); INSERT INTO adopted VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeMigration(t, dir, 102, "102_adopted.sql", "BEGIN; CREATE TABLE adopted(id int PRIMARY KEY); INSERT INTO adopted VALUES (1); COMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, fingerprint, err := m.InspectSchema(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(ctx, files); err == nil {
		t.Fatal("legacy schema was replayed without an explicit baseline")
	}
	manifest := &MigrationBaseline{SchemaVersion: MigrationBaselineSchema, Owner: OwnerPlatform, Database: name, SchemaFingerprint: fingerprint, ReviewedBy: "fixture-reviewer", Entries: []BaselineEntry{{Version: 102, Name: files[0].Name, Checksum: files[0].Checksum, LegacyName: "066_adopted.sql"}}}
	if err := m.Baseline(ctx, files, manifest); err != nil {
		t.Fatal(err)
	}
	if err := m.Baseline(ctx, files, manifest); err != nil {
		t.Fatalf("baseline replay: %v", err)
	}
	out, err := m.Apply(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Applied) != 0 || len(out.Skipped) != 1 {
		t.Fatal("adopted SQL ran again")
	}
	if out.Skipped[0].Origin != "baseline" || out.Skipped[0].ReviewedBy != "fixture-reviewer" {
		t.Fatal("baseline adoption disguised as SQL execution")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM adopted`).Scan(&count); err != nil || count != 1 {
		t.Fatal("baseline changed user data")
	}
	_, after, err := m.InspectSchema(ctx)
	if err != nil || after != fingerprint {
		t.Fatal("baseline changed application schema")
	}
	manifest.Database = "not-this-database"
	if err := m.Baseline(ctx, files, manifest); err == nil {
		t.Fatal("wrong database accepted")
	}
	manifest.Database = name
	if _, err := db.Exec(`ALTER TABLE adopted ADD COLUMN changed text`); err != nil {
		t.Fatal(err)
	}
	if err := m.Baseline(ctx, files, manifest); err == nil {
		t.Fatal("stale schema snapshot accepted")
	}
}

func TestDDLAndLedgerAreAtomicForWrappedScripts(t *testing.T) {
	db := freshDatabase(t)
	m := Migrator{DB: db, Owner: OwnerPlatform}
	ctx := context.Background()
	if err := m.ensureLedger(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION deny_record() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'fixture ledger unavailable'; END$$;
	 CREATE TRIGGER deny_record BEFORE INSERT ON platform_schema_migrations FOR EACH ROW EXECUTE FUNCTION deny_record()`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_atomic.sql", "BEGIN; CREATE TABLE must_rollback(id int); COMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(ctx, files); err == nil {
		t.Fatal("failed ledger write reported success")
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('must_rollback') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("DDL committed without ledger")
	}
}

func TestCancellationRollsBackInFlightDDLAndReleasesSession(t *testing.T) {
	db := freshDatabase(t)
	db.SetMaxOpenConns(1)
	m := Migrator{DB: db, Owner: OwnerPlatform}
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_cancel.sql", "BEGIN; CREATE TABLE interrupted(id int); SELECT pg_sleep(5); COMMIT;")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := m.Apply(ctx, files); err == nil {
		t.Fatal("in-flight cancellation succeeded")
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('interrupted') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("cancelled DDL survived")
	}
	writeMigration(t, dir, 1, "001_cancel.sql", "CREATE TABLE interrupted(id int);")
	files, err = LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	retry, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if _, err := m.Apply(retry, files); err != nil {
		t.Fatalf("connection/lock leaked: %v", err)
	}
}

func TestMigrationVerifyDoesNotWriteLedger(t *testing.T) {
	db := freshDatabase(t)
	m := Migrator{DB: db, Owner: OwnerPlatform}
	dir := t.TempDir()
	writeMigration(t, dir, 1, "001_pending.sql", "CREATE TABLE pending(id int);")
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ready, _, err := m.VerifySchemaForService(context.Background(), files); err != nil || ready {
		t.Fatal("missing ledger passed verification")
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('platform_schema_migrations') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("verify wrote a ledger")
	}
}

func TestBaselineManifestStrictDecodeAndContentBinding(t *testing.T) {
	if _, err := DecodeMigrationBaseline([]byte("schemaVersion: migration-baseline/v1\npassword: secret\n")); err == nil {
		t.Fatal("unknown baseline field accepted")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "001_real.sql")
	if err := os.WriteFile(file, []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	files[0].SQL = "DROP TABLE customer_data;"
	if err := validateMigrationFiles(files); err == nil {
		t.Fatal("in-memory SQL changed without identity verification")
	}
	if err := (Migrator{DB: &sql.DB{}, Owner: MigrationOwner(strings.Repeat("x", 100))}).validate(); err == nil {
		t.Fatal("unknown owner accepted")
	}
}

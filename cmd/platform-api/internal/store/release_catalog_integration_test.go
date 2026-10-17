package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// releaseCatalogTestDB mirrors the shipped migration 104 table shape in a
// dedicated schema, so the registration transaction is tested without coupling
// to the platform's migration acceptance.
func releaseCatalogTestDB(t *testing.T) *sql.DB {
	t.Helper()
	base := openPGIntDB(t)
	ctx := context.Background()
	schema := "release_catalog_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := base.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`) })
	u, err := url.Parse(os.Getenv("HNB_TEST_POSTGRES_DSN"))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("release catalog tests require a PostgreSQL URL DSN")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.ExecContext(ctx, `CREATE TABLE release_catalog_entries (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		release_id TEXT NOT NULL,
		release_version TEXT NOT NULL,
		role TEXT NOT NULL CHECK (role IN ('console', 'control-plane', 'worker')),
		bundle_id TEXT NOT NULL,
		lock_digest TEXT NOT NULL CHECK (lock_digest ~ '^sha256:[0-9a-f]{64}$'),
		content_digest TEXT NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
		imported_by TEXT NOT NULL,
		registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (release_id, release_version, role))`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func releaseFixture(lockHex rune, contentHex rune, bundleHex rune) ReleaseCatalogEntry {
	return ReleaseCatalogEntry{
		ReleaseID: "r1", ReleaseVersion: "1.0", Role: "worker",
		BundleID:      "bundle-" + strings.Repeat(string(bundleHex), 16),
		LockDigest:    "sha256:" + strings.Repeat(string(lockHex), 64),
		ContentDigest: "sha256:" + strings.Repeat(string(contentHex), 64),
	}
}

func TestReleaseCatalogPGIdempotentAndFixedReleaseRejected(t *testing.T) {
	db := releaseCatalogTestDB(t)
	st := NewPGStore(db)
	ctx := context.Background()

	first := releaseFixture('1', '2', 'b')
	created, ok, err := st.RegisterReleaseMaterial(ctx, first)
	if err != nil || !ok {
		t.Fatalf("first import failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("first import returned no id")
	}
	replays, ok, err := st.RegisterReleaseMaterial(ctx, first)
	if err != nil || ok || replays.ID != created.ID {
		t.Fatalf("duplicate import was not idempotent: %+v %v", replays, err)
	}
	list, err := st.ListReleaseMaterials(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list after duplicates = %d (%v)", len(list), err)
	}

	conflict := releaseFixture('1', '9', 'b')
	if _, _, err := st.RegisterReleaseMaterial(ctx, conflict); !errors.Is(err, ErrFixedReleaseConflict) {
		t.Fatalf("fixed release silently replaced: %v", err)
	}
	list, err = st.ListReleaseMaterials(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("rejected replacement altered the catalog: %d (%v)", len(list), err)
	}

	another := releaseFixture('1', '3', 'c')
	another.ReleaseVersion = "2.0"
	if _, ok, err := st.RegisterReleaseMaterial(ctx, another); err != nil || !ok {
		t.Fatalf("new release import failed: %v", err)
	}
	list, err = st.ListReleaseMaterials(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list after new release = %d (%v)", len(list), err)
	}
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/google/uuid"
)

// Each test uses its own schema. This suite tests the registration transaction,
// not provider action qualification or the platform's migration acceptance.
func conformanceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	base := openPGIntDB(t)
	ctx := context.Background()
	schema := "conformance_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := base.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`) })
	u, err := url.Parse(os.Getenv("HNB_TEST_POSTGRES_DSN"))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("conformance tests require a PostgreSQL URL DSN")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// Mirrors the shipped migration 030 shape: a surrogate id primary key and a
	// plain (non-unique) provider_id index, so upserts cannot rely on ON CONFLICT.
	_, err = db.ExecContext(ctx, `CREATE TABLE provider_manifests (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(), provider_id TEXT NOT NULL,
		name TEXT NOT NULL, version TEXT NOT NULL, protocol_version TEXT NOT NULL,
		capabilities TEXT[] NOT NULL DEFAULT '{}', actions TEXT[] NOT NULL DEFAULT '{}',
		permissions JSONB DEFAULT '{}', resource_requirements JSONB DEFAULT '{}',
		dependencies JSONB DEFAULT '[]', compatibility JSONB DEFAULT '{}',
		conformance_level TEXT NOT NULL DEFAULT 'none', conformance_evidence JSONB DEFAULT '[]',
		conformance_expires_at TIMESTAMPTZ, storage_driver_package JSONB NOT NULL DEFAULT 'null',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
		CREATE INDEX provider_manifests_provider ON provider_manifests(provider_id)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestConformancePGAtomicRegistration(t *testing.T) {
	db := conformanceTestDB(t)
	st := NewPGStore(db)
	ctx := context.Background()
	digest := "sha256:" + strings.Repeat("d", 64)
	manifest := &ProviderManifest{ProviderID: "fixture-provider", Name: "Fixture", Version: "1", ProtocolVersion: "2.0.0",
		Actions: []string{"import"}, ConformanceLevel: "none", Compatibility: map[string]any{"artifactDigest": digest}}
	if err := st.SaveManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetManifest(ctx, manifest.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	report := core.SignedConformanceReport{SignerID: "fixture", Signature: "fixture-not-an-attestation", Report: core.ConformanceReport{
		ProviderID: manifest.ProviderID, ProviderVersion: manifest.Version, ArtifactDigest: digest,
		ManifestDigest: ManifestIdentityDigest(current), MatrixVersion: "fixture-matrix", TargetKind: "KubernetesTarget", Action: "import", ExpiresAt: time.Now().Add(time.Hour)}}
	if err := st.RegisterConformance(ctx, report); err != nil {
		t.Fatal(err)
	}
	qualified, err := st.GetManifest(ctx, manifest.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if qualified.ConformanceLevel != "production_ready" || len(qualified.ConformanceEvidence) != 1 || qualified.ConformanceExpiresAt == nil {
		t.Fatal("qualification not stored atomically")
	}
	if ManifestIdentityDigest(qualified) != report.Report.ManifestDigest {
		t.Fatal("report storage changed base identity")
	}
	retained, _ := json.Marshal(qualified.ResourceRequirements["conformanceReport"])
	var signed core.SignedConformanceReport
	if err := json.Unmarshal(retained, &signed); err != nil || signed.SignerID != report.SignerID {
		t.Fatal("signed report not retained")
	}
	// A manifest replacement invalidates the old qualification.
	manifest.Version = "2"
	if err := st.SaveManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterConformance(ctx, report); !errors.Is(err, ErrConformanceIdentityChanged) {
		t.Fatalf("stale report accepted: %v", err)
	}
	latest, err := st.GetManifest(ctx, manifest.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ConformanceLevel != "none" {
		t.Fatal("stale report promoted replaced manifest")
	}
}

func TestConformancePGManifestUpsert(t *testing.T) {
	db := conformanceTestDB(t)
	st := NewPGStore(db)
	ctx := context.Background()
	manifest := &ProviderManifest{ProviderID: "fixture-provider", Name: "Fixture", Version: "1", ProtocolVersion: "2.0.0", Actions: []string{"import"}}
	if err := st.SaveManifest(ctx, manifest); err != nil {
		t.Fatalf("insert: %v", err)
	}
	manifest.Version = "2"
	if err := st.SaveManifest(ctx, manifest); err != nil {
		t.Fatalf("update: %v", err)
	}
	var rows, version int
	if err := db.QueryRow(`SELECT count(*), max(CASE WHEN version='2' THEN 1 ELSE 0 END) FROM provider_manifests WHERE provider_id=$1`, manifest.ProviderID).Scan(&rows, &version); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || version != 1 {
		t.Fatalf("upsert produced rows=%d version2=%d", rows, version)
	}
	// Duplicated rows make the lookup ambiguous and must not be merged silently.
	if _, err := db.Exec(`INSERT INTO provider_manifests (provider_id,name,version,protocol_version) VALUES ('fixture-provider','Duplicate','3','2.0.0')`); err != nil {
		t.Fatal(err)
	}
	// Duplicated rows make the lookup ambiguous; reads must fail closed.
	if _, err := st.GetManifest(ctx, manifest.ProviderID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ambiguous lookup error = %v", err)
	}
	if err := st.SaveManifest(ctx, manifest); err == nil {
		t.Fatal("ambiguous upsert was accepted")
	}
	if err := st.RegisterConformance(ctx, core.SignedConformanceReport{}); err == nil {
		t.Fatal("conformance registered against an ambiguous manifest")
	}
}

func TestConformancePGConcurrentReplacement(t *testing.T) {
	db := conformanceTestDB(t)
	st := NewPGStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	digest := "sha256:" + strings.Repeat("e", 64)
	manifest := &ProviderManifest{ProviderID: "fixture-provider", Name: "Fixture", Version: "1", ProtocolVersion: "2.0.0", Actions: []string{"import"}, ConformanceLevel: "none", Compatibility: map[string]any{"artifactDigest": digest}}
	if err := st.SaveManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	current, err := st.GetManifest(ctx, manifest.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	report := core.SignedConformanceReport{Report: core.ConformanceReport{ProviderID: manifest.ProviderID, ProviderVersion: "1", ArtifactDigest: digest, ManifestDigest: ManifestIdentityDigest(current), Action: "import", ExpiresAt: time.Now().Add(time.Hour)}}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE provider_manifests SET version='2' WHERE provider_id=$1`, manifest.ProviderID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- st.RegisterConformance(ctx, report) }()
	select {
	case err := <-done:
		t.Fatalf("registration bypassed locked replacement: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrConformanceIdentityChanged) {
		t.Fatalf("concurrent replacement accepted: %v", err)
	}
}

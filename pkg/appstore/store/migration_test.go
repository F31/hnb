package store

import (
	"os"
	"strings"
	"testing"
)

func TestArtifactDescriptorMigrationGuardsIdentity(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/033_artifact_descriptors.sql")
	if err != nil {
		t.Fatalf("read descriptor migration: %v", err)
	}
	sql := string(contents)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS tenant_id TEXT",
		"ALTER COLUMN package_id DROP NOT NULL",
		"^sha256:[0-9a-f]{64}$",
		"idx_artifacts_tenant_digest",
		"ADD COLUMN IF NOT EXISTS repository TEXT",
		"ALTER COLUMN repository SET NOT NULL",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration does not contain %q", required)
		}
	}
}

func TestArtifactDescriptorRollbackWarnsAgainstProductionDataLoss(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/033_artifact_descriptors.rollback.sql")
	if err != nil {
		t.Fatalf("read descriptor rollback: %v", err)
	}
	if !strings.Contains(string(contents), "empty development databases only") {
		t.Fatal("rollback must document its destructive environment restriction")
	}
}

func TestReleaseArtifactsMigrationNormalizesVerifiedDigests(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/034_release_artifacts.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS release_artifacts",
		"REFERENCES artifacts(id) ON DELETE RESTRICT",
		"digest ~ '^sha256:[0-9a-f]{64}$'",
		"jsonb_array_elements",
		"verification_status = 'verified'",
		"lifecycle_state = 'active'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("034 migration missing %q", fragment)
		}
	}
}

func TestArtifactStorageProfilesMigrationDefinesControlPlaneOnlyMetadata(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/035_artifact_storage_profiles.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS artifact_storage_profiles",
		"backend IN ('local', 'pvc', 's3', 'oci')",
		"secret_reference TEXT",
		"ADD COLUMN IF NOT EXISTS storage_profile_id UUID REFERENCES artifact_storage_profiles(id) ON DELETE SET NULL",
		"CREATE TABLE IF NOT EXISTS artifact_profile_migrations",
		"operation_id UUID REFERENCES operations(id) ON DELETE SET NULL",
		"checkpoint JSONB NOT NULL DEFAULT '{}'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("035 migration missing %q", fragment)
		}
	}
}

func TestArtifactStorageAndWorkloadStorageTablesRemainSeparate(t *testing.T) {
	artifactMigration, err := os.ReadFile("../../../database/postgresql/migrations/035_artifact_storage_profiles.sql")
	if err != nil {
		t.Fatal(err)
	}
	volumeBoundary, err := os.ReadFile("../../../database/postgresql/migrations/074_storage_volume_semantics_boundary.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(artifactMigration), "workload_storage_offerings") || strings.Contains(string(artifactMigration), "storage_class_bindings") {
		t.Fatal("App Market artifact migration owns workload storage tables")
	}
	if strings.Contains(string(volumeBoundary), "artifact_storage_profiles") || strings.Contains(string(volumeBoundary), "artifact_profile_migrations") {
		t.Fatal("workload storage migration references App Market tables")
	}
	for _, discriminator := range []string{"consumption_model = 'KubernetesPersistentVolume'", "binding_target = 'KubernetesStorageClass'"} {
		if !strings.Contains(string(volumeBoundary), discriminator) {
			t.Fatalf("workload storage boundary migration missing %q", discriminator)
		}
	}
}

func TestArtifactDistributionTargetsMigrationDefinesRebuildAndWatermarks(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/036_artifact_distribution_targets.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS artifact_distribution_targets",
		"target_role IN ('regional_mirror', 'edge_cache')",
		"state IN ('pending', 'syncing', 'ready', 'stale', 'failed')",
		"rebuild_operation_id UUID REFERENCES operations(id) ON DELETE SET NULL",
		"high_watermark_bytes >= low_watermark_bytes",
		"authority_profile_id <> target_profile_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("036 migration missing %q", fragment)
		}
	}
}

func TestArtifactGCMigrationDefinesProtectedReferenceFlow(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/037_artifact_gc.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS artifact_references",
		"owner_type IN ('release', 'runtime', 'rollback', 'composition', 'dr_snapshot', 'offline_bundle')",
		"REFERENCES artifacts(id) ON DELETE RESTRICT",
		"CREATE TABLE IF NOT EXISTS artifact_tombstones",
		"operation_id UUID REFERENCES operations(id) ON DELETE SET NULL",
		"CREATE TABLE IF NOT EXISTS artifact_locks",
		"lease_until TIMESTAMPTZ NOT NULL",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("037 migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "WHERE expires_at IS NULL OR expires_at > now()") {
		t.Fatal("partial index must not use non-immutable now()")
	}
}

func TestUploadSessionReleaseMigrationAddsNullableSetNullRelation(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/040_upload_session_release.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS release_id UUID REFERENCES releases(id) ON DELETE SET NULL",
		"idx_upload_sessions_release_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("040 migration missing %q", fragment)
		}
	}
}

func TestReleaseManifestDigestIndexAllowsDuplicates(t *testing.T) {
	contents, err := os.ReadFile("../../../database/postgresql/migrations/041_release_manifest_digest_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	if !strings.Contains(sql, "DROP INDEX IF EXISTS idx_releases_manifest_digest") {
		t.Fatal("041 migration does not drop the unique manifest digest index")
	}
	if !strings.Contains(sql, "CREATE INDEX IF NOT EXISTS idx_releases_manifest_digest") {
		t.Fatal("041 migration does not recreate a non-unique manifest digest index")
	}
	if strings.Contains(sql, "CREATE UNIQUE INDEX") {
		t.Fatal("041 migration retains uniqueness")
	}
}

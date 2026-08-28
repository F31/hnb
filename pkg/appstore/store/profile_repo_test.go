package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/appstore"
)

func TestValidateStorageProfileTierRules(t *testing.T) {
	base := appstore.ArtifactStorageProfile{
		ID: "profile-a", TenantID: "tenant-a", Name: "authority", CreatedBy: "subject-a",
		Backend: appstore.StoragePVC, ServiceTier: appstore.StorageTierLiteHA, AuthorityRole: appstore.StorageAuthoritative,
	}
	if err := ValidateStorageProfile(&base); err != nil {
		t.Fatalf("valid lite_ha profile rejected: %v", err)
	}

	localHA := base
	localHA.Backend = appstore.StorageLocal
	if err := ValidateStorageProfile(&localHA); !errors.Is(err, ErrInvalidStorageProfile) {
		t.Fatalf("expected invalid local lite_ha profile, got %v", err)
	}

	mirrorHA := base
	mirrorHA.AuthorityRole = appstore.StorageMirror
	if err := ValidateStorageProfile(&mirrorHA); !errors.Is(err, ErrInvalidStorageProfile) {
		t.Fatalf("expected invalid mirror authority for lite_ha, got %v", err)
	}
}

func TestValidateStorageProfileRejectsInlineSecrets(t *testing.T) {
	profile := &appstore.ArtifactStorageProfile{
		ID: "profile-a", TenantID: "tenant-a", Name: "s3", CreatedBy: "subject-a",
		Backend: appstore.StorageS3, ServiceTier: appstore.StorageTierMinimal, AuthorityRole: appstore.StorageAuthoritative,
		Metadata: map[string]any{"access_token": "plain-token"},
	}
	if err := ValidateStorageProfile(profile); !errors.Is(err, ErrInvalidStorageProfile) {
		t.Fatalf("expected inline secret rejection, got %v", err)
	}
}

func TestValidateStorageProfileRejectsWorkloadVolumeSemantics(t *testing.T) {
	for _, key := range []string{"consumptionModel", "bindingTarget", "storageClassBindingId", "workloadStorageOfferingId"} {
		profile := &appstore.ArtifactStorageProfile{
			ID: "profile-a", TenantID: "tenant-a", Name: "s3", CreatedBy: "subject-a",
			Backend: appstore.StorageS3, ServiceTier: appstore.StorageTierMinimal, AuthorityRole: appstore.StorageAuthoritative,
			Metadata: map[string]any{key: "not-app-market-owned"},
		}
		if err := ValidateStorageProfile(profile); !errors.Is(err, ErrInvalidStorageProfile) {
			t.Fatalf("expected %s rejection, got %v", key, err)
		}
	}
}

func TestStorageProfileRepoIsTenantScoped(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()
	capture.affected = 1
	repo := NewStorageProfileRepo(db)
	profile := &appstore.ArtifactStorageProfile{
		ID: "profile-a", TenantID: "tenant-a", Name: "local-authority", CreatedBy: "subject-a",
		Backend: appstore.StorageLocal, ServiceTier: appstore.StorageTierMinimal, AuthorityRole: appstore.StorageAuthoritative,
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("create error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id")

	if _, err := repo.Get("profile-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id=$2")

	profiles, err := repo.List("tenant-a", 100, 0)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected no rows from capture DB, got %d", len(profiles))
	}
	assertTenantSQL(t, capture, "tenant_id=$1")
}

func TestStorageProfileMigrationRequestPreservesDigestAndReferencesProfiles(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()
	migration := &appstore.ArtifactProfileMigration{
		ID: "migration-a", TenantID: "tenant-a", ArtifactID: "artifact-a",
		SourceProfileID: "profile-source", TargetProfileID: "profile-target",
		ArtifactDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Status:         "requested", IdempotencyKey: "idem-a", RequestedBy: "subject-a",
		Checkpoint: map[string]any{"offset": 0},
	}
	if err := NewStorageProfileRepo(db).RequestMigration(migration); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("migration request error = %v", err)
	}
	for _, fragment := range []string{"artifact_profile_migrations", "JOIN artifact_storage_profiles src", "JOIN artifact_storage_profiles dst", "a.digest=$6", "a.lifecycle_state='active'"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("migration query missing %q: %s", fragment, capture.query)
		}
	}
}

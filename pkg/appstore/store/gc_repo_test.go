package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/appstore"
)

func TestGCRepoRegisterAndListReferencesAreTenantScoped(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	ref := &appstore.ArtifactReference{ID: "ref-a", TenantID: "tenant-a", ArtifactID: "artifact-a", OwnerType: "release", OwnerID: "release-a", CreatedBy: "subject-a"}
	if err := NewGCRepo(db).RegisterReference(ref); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("register reference error = %v", err)
	}
	for _, fragment := range []string{"artifact_references", "a.tenant_id=$2", "a.lifecycle_state='active'", "ON CONFLICT"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("register query missing %q: %s", fragment, capture.query)
		}
	}

	refs, err := NewGCRepo(db).ListReferences("artifact-a", "tenant-a")
	if err != nil {
		t.Fatalf("list references error = %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs from capture DB, got %d", len(refs))
	}
	for _, fragment := range []string{"artifact_id=$1", "tenant_id=$2", "expires_at IS NULL OR expires_at > NOW()"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("list query missing %q: %s", fragment, capture.query)
		}
	}
}

func TestGCOperationCommandContainsNoDeleteCredentialOrBody(t *testing.T) {
	tombstone := &appstore.ArtifactTombstone{ID: "tombstone-a", ArtifactID: "artifact-a", ArtifactDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", OperationID: "operation-a"}
	cmd := GCOperationCommand(tombstone)
	for _, forbidden := range []string{"registry_url", "token", "secret", "body"} {
		if _, ok := cmd[forbidden]; ok {
			t.Fatalf("GC command leaked %s: %+v", forbidden, cmd)
		}
	}
}

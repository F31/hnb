package lifecycle

import (
	"os"
	"strings"
	"testing"
)

func TestProviderLifecycleMigrationDefinesMetadataSnapshots(t *testing.T) {
	contents, err := os.ReadFile("../../../../database/postgresql/migrations/038_provider_lifecycle_metadata.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, fragment := range []string{"provider_lifecycle_states", "bundle_digest ~ '^sha256:[0-9a-f]{64}$'", "provider_capability_registrations", "provider_navigation_metadata", "REFERENCES operations(id) ON DELETE SET NULL"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

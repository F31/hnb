package toolkit

import (
	"os"
	"strings"
	"testing"
)

func TestRoleClosureMatchesBuiltBundleForDeclaredRoles(t *testing.T) {
	lock, _, bundle := builtOfflineFixture(t)
	for _, role := range []string{RoleConsole, installRoleString(RoleControlPlane), installRoleString(RoleWorker)} {
		if reasons := ValidateRoleClosure(bundle, lock, role); len(reasons) != 0 {
			t.Fatalf("%s closure rejected: %v", role, reasons)
		}
	}
	if reasons := ValidateRoleClosure(bundle, lock, "not-a-role"); len(reasons) == 0 {
		t.Fatal("unknown install target role accepted")
	}
}

func TestRoleClosureRejectsMissingRoleDependency(t *testing.T) {
	lock, _, bundle := builtOfflineFixture(t)
	for _, role := range []string{RoleConsole, installRoleString(RoleControlPlane), installRoleString(RoleWorker)} {
		for _, kind := range RoleRequiredKinds(role) {
			t.Run(role+"/"+kind, func(t *testing.T) {
				stripped := bundle
				var entries []BundleEntry
				for _, e := range bundle.Entries {
					if e.Role != kind {
						entries = append(entries, e)
					}
				}
				stripped.Entries = entries
				reasons := ValidateRoleClosure(stripped, lock, role)
				if len(reasons) == 0 {
					t.Fatalf("%s accepted without %s material", role, kind)
				}
				found := false
				for _, r := range reasons {
					if strings.Contains(r, "ROLE_MATERIAL_MISSING") && strings.Contains(r, kind) {
						found = true
					}
				}
				if !found {
					t.Fatalf("rejection did not name the missing %s dependency: %v", kind, reasons)
				}
			})
		}
	}
}

func TestRoleClosureRejectsMissingOSPackageIndex(t *testing.T) {
	lock, _, bundle := builtOfflineFixture(t)
	expanded := *lock
	expanded.Compatibility.OS = append(append([]string{}, lock.Compatibility.OS...), "rocky-10.1")
	reasons := ValidateRoleClosure(bundle, &expanded, installRoleString(RoleWorker))
	if len(reasons) == 0 {
		t.Fatal("worker role accepted without a package index for every compatibility OS")
	}
	found := false
	for _, r := range reasons {
		if strings.Contains(r, "rocky-10.1") && strings.Contains(r, "package dependency index") {
			found = true
		}
	}
	if !found {
		t.Fatalf("rejection did not name the missing OS index: %v", reasons)
	}
	if reasons := ValidateRoleClosure(bundle, &expanded, RoleConsole); len(reasons) != 0 {
		t.Fatalf("console role must not depend on node package indexes: %v", reasons)
	}
}

// variantBundle replaces chart content while keeping the release identity, so a
// fixed-release import records the same release with a different content digest.
func variantBundle(t *testing.T, lock *ReleaseLock, dir string) (OfflineBundle, error) {
	t.Helper()
	alt := writeOfflineCache(t, dir, "chart-tgz-alternative")
	for i := range lock.Charts {
		lock.Charts[i].Digest = alt
	}
	return BuildOfflineBundle(lock, dir, nil, nil, offlineNow())
}

func TestReleaseCatalogImportIdempotentAndRejectsReplacement(t *testing.T) {
	lock, dir, bundle := builtOfflineFixture(t)
	catalog := &ReleaseImportCatalog{SchemaVersion: ReleaseCatalogSchema}
	first, added, err := catalog.Import(bundle, lock, installRoleString(RoleWorker), offlineNow())
	if err != nil || !added {
		t.Fatalf("first worker import failed: %v", err)
	}
	again, added, err := catalog.Import(bundle, lock, installRoleString(RoleWorker), offlineNow())
	if err != nil || added || again.BundleID != first.BundleID {
		t.Fatalf("duplicate import was not idempotent: %+v %v", again, err)
	}
	if len(catalog.Entries) != 1 {
		t.Fatalf("duplicate import added an entry: %d", len(catalog.Entries))
	}
	if _, added, err := catalog.Import(bundle, lock, RoleConsole, offlineNow()); err != nil || !added {
		t.Fatalf("console import failed: %v", err)
	}
	if len(catalog.Entries) != 2 {
		t.Fatalf("expected two role entries, got %d", len(catalog.Entries))
	}

	variant, err := variantBundle(t, lock, dir)
	if err != nil {
		t.Fatal(err)
	}
	if variant.ContentDigest == bundle.ContentDigest {
		t.Fatal("variant bundle did not change the content digest")
	}
	if _, _, err := catalog.Import(variant, lock, installRoleString(RoleWorker), offlineNow()); err == nil || !strings.Contains(err.Error(), "different content digest") {
		t.Fatalf("fixed release silently replaced: %v", err)
	}
	if len(catalog.Entries) != 2 {
		t.Fatalf("rejected replacement still altered the catalog: %d", len(catalog.Entries))
	}

	if _, _, err := catalog.Import(bundle, lock, "not-a-role", offlineNow()); err == nil || !strings.Contains(err.Error(), "ROLE_MATERIAL_MISSING") {
		t.Fatalf("unknown role import accepted: %v", err)
	}
	if _, _, err := catalog.Import(bundle, nil, installRoleString(RoleWorker), offlineNow()); err == nil || !strings.Contains(err.Error(), "invalid release lock") {
		t.Fatalf("catalog import without a lock accepted: %v", err)
	}
	tampered := bundle
	tampered.ContentDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	if _, _, err := catalog.Import(tampered, lock, installRoleString(RoleWorker), offlineNow()); err == nil || !strings.Contains(err.Error(), "invalid offline bundle identity") {
		t.Fatalf("tampered bundle identity accepted: %v", err)
	}
}

func TestReleaseCatalogPersistsAndStaysStrict(t *testing.T) {
	lock, _, bundle := builtOfflineFixture(t)
	catalog := &ReleaseImportCatalog{SchemaVersion: ReleaseCatalogSchema}
	if _, _, err := catalog.Import(bundle, lock, installRoleString(RoleWorker), offlineNow()); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/release-catalog.json"
	if err := SaveReleaseImportCatalog(path, *catalog); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReleaseImportCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].Role != installRoleString(RoleWorker) || loaded.Entries[0].LockDigest != bundle.LockDigest {
		t.Fatalf("round trip lost identity: %+v", loaded.Entries)
	}
	// Reimporting the persisted catalog is idempotent.
	if _, added, err := loaded.Import(bundle, lock, installRoleString(RoleWorker), offlineNow()); err != nil || added {
		t.Fatalf("persisted catalog duplicate import failed: %v", err)
	}
}

func TestReleaseCatalogLoadRejectsInvalidDocuments(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"unknown field":  `{"schemaVersion":"release-catalog/v1","entries":[],"surprise":true}`,
		"extra document": `{"schemaVersion":"release-catalog/v1","entries":[]}{"schemaVersion":"release-catalog/v1"}`,
		"bad schema":     `{"schemaVersion":"release-catalog/v0","entries":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := dir + "/" + name + ".json"
			writeFixtureFile(t, path, content)
			if _, err := LoadReleaseImportCatalog(path); err == nil {
				t.Fatal("invalid release catalog accepted")
			}
		})
	}
	if _, err := LoadReleaseImportCatalog(dir + "/absent.json"); err == nil {
		t.Fatal("absent release catalog accepted")
	}
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

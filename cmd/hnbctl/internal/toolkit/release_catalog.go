package toolkit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// Install target roles share one release lock and one content-addressed material
// closure. Each role declares the offline material it depends on; the console/
// management-platform role needs the platform image/chart/migration closure,
// while the node roles need the bootstrap closure. Node roles reuse the existing
// NodeRole constants (control-plane, worker) by value.
const RoleConsole = "console"

func installRoleString(role NodeRole) string { return string(role) }

// RoleRequiredKinds returns the material entry kinds an install target role
// depends on under the shared release lock. The console role does not require a
// node package closure and the node roles do not require platform charts.
func RoleRequiredKinds(role string) []string {
	switch role {
	case RoleConsole:
		return []string{"chart", "migration", "image-manifest", "image-config", "image-layer", "cli"}
	case installRoleString(RoleControlPlane), installRoleString(RoleWorker):
		return []string{"sandbox-image", "os-package-index", "os-package", "cli"}
	default:
		return nil
	}
}

// ValidateRoleClosure verifies that the bundle provides every material kind the
// requested install target role depends on. Node roles additionally require a
// pinned OS package index for every declared OS/architecture. A missing role
// dependency is a rejection reason, never a silent gap.
func ValidateRoleClosure(b OfflineBundle, lock *ReleaseLock, role string) []string {
	if lock == nil || len(lock.Validate()) > 0 {
		return []string{"invalid release lock"}
	}
	kinds := RoleRequiredKinds(role)
	if kinds == nil {
		return []string{"unsupported install target role " + role}
	}
	have := map[string]bool{}
	for _, e := range b.Entries {
		have[e.Role] = true
	}
	var reasons []string
	for _, kind := range kinds {
		if !have[kind] {
			reasons = append(reasons, fmt.Sprintf("ROLE_MATERIAL_MISSING: %s requires %s material", role, kind))
		}
	}
	if role == installRoleString(RoleControlPlane) || role == installRoleString(RoleWorker) {
		indexed := map[string]bool{}
		for _, a := range lock.NodeArtifacts {
			if a.Kind == "package-index" {
				indexed[a.OS+"/"+a.Arch] = true
			}
		}
		for _, osName := range lock.Compatibility.OS {
			for _, arch := range lock.Compatibility.Arch {
				if !indexed[osName+"/"+arch] {
					reasons = append(reasons, fmt.Sprintf("ROLE_MATERIAL_MISSING: %s lacks an OS package dependency index for %s/%s", role, osName, arch))
				}
			}
		}
	}
	return reasons
}

const ReleaseCatalogSchema = "release-catalog/v1"

// ReleaseCatalogEntry records one validated release import under a target role,
// tied to the same release identity, lock digest and content digest as the
// offline bundle. Roles are per-entry so the same release can be imported for
// multiple target roles from one catalog.
type ReleaseCatalogEntry struct {
	Role           string    `json:"role"`
	BundleID       string    `json:"bundleId"`
	ReleaseID      string    `json:"releaseId"`
	ReleaseVersion string    `json:"releaseVersion"`
	LockDigest     string    `json:"lockDigest"`
	ContentDigest  string    `json:"contentDigest"`
	ImportedAt     time.Time `json:"importedAt"`
}

// ReleaseImportCatalog is the local, installer-owned view of a release material
// directory. The owning-plane artifacts catalog is where platform-installed
// registrations live; this local catalog stays usable before the platform runs.
type ReleaseImportCatalog struct {
	SchemaVersion string                `json:"schemaVersion"`
	Entries       []ReleaseCatalogEntry `json:"entries"`
}

// Import registers a validated offline bundle for a target role. Validating the
// role dependency closures requires the same pinned release lock the bundle was
// built from. Re-importing the identical bundle and role is a no-op.
func (c *ReleaseImportCatalog) Import(b OfflineBundle, lock *ReleaseLock, role string, now time.Time) (ReleaseCatalogEntry, bool, error) {
	if b.SchemaVersion != OfflineBundleManifest || b.ContentDigest != bundleContentDigest(b) {
		return ReleaseCatalogEntry{}, false, fmt.Errorf("invalid offline bundle identity")
	}
	if role == "" {
		return ReleaseCatalogEntry{}, false, fmt.Errorf("install target role is required")
	}
	if reasons := ValidateRoleClosure(b, lock, role); len(reasons) > 0 {
		return ReleaseCatalogEntry{}, false, fmt.Errorf("ROLE_MATERIAL_MISSING: %s", reasons[0])
	}
	for _, e := range c.Entries {
		if e.ReleaseID == b.ReleaseID && e.ReleaseVersion == b.ReleaseVersion && e.Role == role {
			if e.ContentDigest != b.ContentDigest {
				return ReleaseCatalogEntry{}, false, fmt.Errorf("fixed release %s/%s already imported for %s with a different content digest", b.ReleaseID, b.ReleaseVersion, role)
			}
		}
	}
	for _, e := range c.Entries {
		if e.Role == role && e.BundleID == b.BundleID && e.LockDigest == b.LockDigest && e.ContentDigest == b.ContentDigest {
			return e, false, nil
		}
	}
	stamp := now.UTC()
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	}
	entry := ReleaseCatalogEntry{
		Role: role, BundleID: b.BundleID, ReleaseID: b.ReleaseID,
		ReleaseVersion: b.ReleaseVersion, LockDigest: b.LockDigest,
		ContentDigest: b.ContentDigest, ImportedAt: stamp,
	}
	c.Entries = append(c.Entries, entry)
	sort.Slice(c.Entries, func(i, j int) bool {
		a, d := c.Entries[i], c.Entries[j]
		if a.Role != d.Role {
			return a.Role < d.Role
		}
		return a.BundleID < d.BundleID
	})
	return entry, true, nil
}

// LoadReleaseImportCatalog reads a strict single-document catalog file.
func LoadReleaseImportCatalog(path string) (ReleaseImportCatalog, error) {
	var catalog ReleaseImportCatalog
	fi, err := os.Lstat(path)
	if err != nil {
		return catalog, fmt.Errorf("release catalog unavailable")
	}
	if !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return catalog, fmt.Errorf("release catalog must be a regular file under 1 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return catalog, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return catalog, fmt.Errorf("invalid release catalog")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return catalog, fmt.Errorf("release catalog must contain one document")
	}
	if catalog.SchemaVersion != ReleaseCatalogSchema {
		return catalog, fmt.Errorf("unsupported release catalog schema")
	}
	return catalog, nil
}

// SaveReleaseImportCatalog persists the catalog atomically.
func SaveReleaseImportCatalog(path string, catalog ReleaseImportCatalog) error {
	if catalog.SchemaVersion != ReleaseCatalogSchema {
		return fmt.Errorf("unsupported release catalog schema")
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(f).Encode(catalog)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write release catalog")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// CatalogRoleLabel humanizes a target role for manifests and CLI output.
func CatalogRoleLabel(role string) string {
	switch role {
	case RoleConsole:
		return "console/management platform"
	case installRoleString(RoleControlPlane):
		return "control-plane node"
	case installRoleString(RoleWorker):
		return "worker node"
	default:
		return role
	}
}

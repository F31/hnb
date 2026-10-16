package toolkit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func offlineNow() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }

// writeOfflineCache materializes one content-addressed object in a prepared dir.
func writeOfflineCache(t *testing.T, dir, content string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, "blobs", "sha256", digest)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return "sha256:" + digest
}

// offlineFixture builds a complete bundle: image closure, chart, cli, migration,
// OS package index and sandbox image.
func offlineFixture(t *testing.T) (*ReleaseLock, string, []BundleEntry, []BundleEntry) {
	t.Helper()
	dir := t.TempDir()
	config := writeOfflineCache(t, dir, `{"os":"linux","architecture":"amd64"}`)
	layerData := strings.Repeat("layer", 1000)
	layer := writeOfflineCache(t, dir, layerData)
	manifest := writeOfflineCache(t, dir, fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":%q,"size":%d},"layers":[{"digest":%q,"size":%d}]}`, config, len(`{"os":"linux","architecture":"amd64"}`), layer, len(layerData)))
	chart := writeOfflineCache(t, dir, "chart-tgz")
	cli := writeOfflineCache(t, dir, "cli-binary")
	migration := writeOfflineCache(t, dir, "SELECT 1;")
	packageDigest := writeOfflineCache(t, dir, "synthetic kubeadm package")
	dependencyDigest := writeOfflineCache(t, dir, "synthetic libc dependency")
	toolDigest := writeOfflineCache(t, dir, "synthetic helm binary")
	indexData, err := json.Marshal(OSDependencyIndex{SchemaVersion: "os-package-closure/v1", OS: "ubuntu-24.04", Arch: "amd64", Roots: []string{"kubeadm.deb"}, Packages: []OSDependencyPackage{{Ref: "kubeadm.deb", Version: "1.36.5", Digest: packageDigest, Dependencies: []string{"libc.deb"}}, {Ref: "libc.deb", Version: "1", Digest: dependencyDigest}}})
	if err != nil {
		t.Fatal(err)
	}
	osIndex := writeOfflineCache(t, dir, string(indexData))

	lock := &ReleaseLock{SchemaVersion: LockSchemaVersion, ReleaseID: "r1", ReleaseVersion: "1.0",
		Images:        []PinnedImage{{Name: "apiserver", Ref: "registry.invalid/apiserver:v1", Digest: manifest, Platform: "linux/amd64"}},
		Charts:        []PinnedChart{{Name: "hnb", Version: "1.0", Ref: "oci://registry.invalid/hnb", Digest: chart}},
		CLI:           []PinnedCLI{{OS: "linux", Arch: "amd64", Ref: "hnbctl", Digest: cli}},
		Migrations:    []Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.TrimPrefix(migration, "sha256:")}},
		NodeArtifacts: []NodeArtifact{{OS: "ubuntu-24.04", Arch: "amd64", Kind: "package-index", Ref: "ubuntu-24.04-index", Digest: osIndex}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "package", Ref: "kubeadm.deb", Digest: packageDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "binary", Ref: "helm", Digest: toolDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "container", Ref: "registry.invalid/pause:3.10", Digest: manifest}},
		Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}

	osPackages := []BundleEntry{{Role: "os-package-index", Name: "ubuntu-24.04-index", Digest: osIndex}}
	sandboxImages := []BundleEntry{{Role: "sandbox-image", Name: "registry.invalid/pause:3.10", Digest: manifest}}
	return lock, dir, osPackages, sandboxImages
}

// 完整闭包可构建并通过校验.
func TestOfflineBundleBuildsAndVerifiesCompleteClosure(t *testing.T) {
	lock, dir, osPackages, sandboxImages := offlineFixture(t)
	bundle, err := BuildOfflineBundle(lock, dir, osPackages, sandboxImages, offlineNow())
	if err != nil {
		t.Fatal(err)
	}
	if bundle.SchemaVersion != OfflineBundleManifest || bundle.BundleID == "" || bundle.ContentDigest == "" {
		t.Fatalf("incomplete bundle identity: %+v", bundle)
	}
	v := VerifyOfflineBundle(bundle, lock, dir)
	if !v.Complete {
		t.Fatalf("complete bundle failed verification: %+v", v)
	}
	// Identity is content-derived and stable.
	again, err := BuildOfflineBundle(lock, dir, osPackages, sandboxImages, offlineNow())
	if err != nil || again.ContentDigest != bundle.ContentDigest || again.BundleID != bundle.BundleID {
		t.Fatalf("bundle identity is not deterministic: %+v %v", again, err)
	}
}

// 删去任一必须物料即校验失败（DBND-003 核心验收）.
func TestRemovingAnyRequiredMaterialFailsVerification(t *testing.T) {
	lock, dir, osPackages, sandboxImages := offlineFixture(t)
	bundle, err := BuildOfflineBundle(lock, dir, osPackages, sandboxImages, offlineNow())
	if err != nil {
		t.Fatal(err)
	}

	for _, drop := range []struct {
		role string
		name string
	}{
		{"os-package-index", "ubuntu-24.04-index"},
		{"sandbox-image", "registry.invalid/pause:3.10"},
		{"image-layer", "apiserver"},
		{"image-config", "apiserver"},
		{"image-manifest", "apiserver"},
		{"chart", "hnb"},
		{"cli", "cli-linux-amd64-0"},
		{"migration", "platform/001_init.sql"},
		{"binary", "helm"},
		{"package", "kubeadm.deb"},
		{"os-package", "ubuntu-24.04/amd64/libc.deb"},
	} {
		t.Run(drop.role, func(t *testing.T) {
			trimmed := bundle
			trimmed.Entries = nil
			removedSomething := false
			for _, entry := range bundle.Entries {
				if entry.Role == drop.role && entry.Name == drop.name {
					removedSomething = true
					continue
				}
				trimmed.Entries = append(trimmed.Entries, entry)
			}
			if !removedSomething {
				t.Fatalf("fixture does not contain %s/%s", drop.role, drop.name)
			}
			v := VerifyOfflineBundle(trimmed, lock, dir)
			if v.Complete {
				t.Fatalf("removing %s/%s still verified: %+v", drop.role, drop.name, v)
			}
			if len(v.Reasons) == 0 || !strings.Contains(strings.Join(v.Reasons, " "), "node modification must not proceed") {
				t.Fatalf("missing blocking reason for %s/%s: %+v", drop.role, drop.name, v.Reasons)
			}
		})
	}
}

// 构建阶段缺失物料即失败，绝不产出"部分闭包".
func TestBuildFailsWhenRequiredMaterialAbsent(t *testing.T) {
	lock, dir, osPackages, _ := offlineFixture(t)
	// Ask for a sandbox image that was never prepared.
	missing := append(append([]BundleEntry{}, osPackages...), BundleEntry{Role: "sandbox-image", Name: "absent", Digest: "sha256:" + strings.Repeat("0", 64)})
	_, err := BuildOfflineBundle(lock, dir, missing, nil, offlineNow())
	if err == nil || !strings.Contains(err.Error(), "BUNDLE_CLOSURE_INCOMPLETE") {
		t.Fatalf("build accepted an incomplete closure: %v", err)
	}
}

// 内容被篡改即校验失败（摘要不符）.
func TestCorruptBundleContentFailsVerification(t *testing.T) {
	lock, dir, osPackages, sandboxImages := offlineFixture(t)
	bundle, err := BuildOfflineBundle(lock, dir, osPackages, sandboxImages, offlineNow())
	if err != nil {
		t.Fatal(err)
	}
	victim := bundle.Entries[0]
	if err := os.WriteFile(filepath.Join(dir, victim.Path), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := VerifyOfflineBundle(bundle, lock, dir)
	if v.Complete {
		t.Fatalf("tampered bundle verified: %+v", v)
	}
	if len(v.Corrupt) == 0 || !strings.Contains(strings.Join(v.Reasons, " "), "BUNDLE_CONTENT_CORRUPT") {
		t.Fatalf("corruption not reported: %+v", v)
	}
}

// bundle 与其它 release lock 不匹配时拒绝.
func TestBundleLockMismatchRejected(t *testing.T) {
	lock, dir, osPackages, sandboxImages := offlineFixture(t)
	bundle, err := BuildOfflineBundle(lock, dir, osPackages, sandboxImages, offlineNow())
	if err != nil {
		t.Fatal(err)
	}
	other := &ReleaseLock{SchemaVersion: LockSchemaVersion, ReleaseID: "r2", ReleaseVersion: "2.0",
		Images: []PinnedImage{{Name: "apiserver", Ref: "registry.invalid/apiserver:v2",
			Digest: "sha256:" + strings.Repeat("f", 64), Platform: "linux/amd64"}},
		Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}
	v := VerifyOfflineBundle(bundle, other, dir)
	if v.Complete || !strings.Contains(strings.Join(v.Reasons, " "), "BUNDLE_LOCK_MISMATCH") {
		t.Fatalf("lock mismatch not rejected: %+v", v)
	}
}

// manifest 写入私有且独占，可回读.
func TestBundleManifestRoundTrips(t *testing.T) {
	lock, dir, osPackages, sandboxImages := offlineFixture(t)
	bundle, err := BuildOfflineBundle(lock, dir, osPackages, sandboxImages, offlineNow())
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "bundle.json")
	if err := WriteBundleManifest(manifest, bundle); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(manifest)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("manifest must be private 0600: %v %v", fi, err)
	}
	if err := WriteBundleManifest(manifest, bundle); err == nil {
		t.Fatal("existing manifest was overwritten")
	}
	loaded, err := LoadBundleManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ContentDigest != bundle.ContentDigest || len(loaded.Entries) != len(bundle.Entries) {
		t.Fatalf("manifest round trip lost identity: %+v", loaded)
	}
	if err := os.WriteFile(manifest, []byte(`{"schemaVersion":"bogus/v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBundleManifest(manifest); err == nil {
		t.Fatal("unsupported manifest schema accepted")
	}
}

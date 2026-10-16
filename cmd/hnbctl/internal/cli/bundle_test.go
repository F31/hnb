package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"gopkg.in/yaml.v3"
)

// writeBundleFixture materializes a prepared cache plus its OS/sandbox material.
func writeBundleFixture(t *testing.T) (lockPath, cacheDir, osIndex, sandbox string) {
	t.Helper()
	cacheDir = t.TempDir()
	put := func(content string) string {
		sum := sha256.Sum256([]byte(content))
		digest := hex.EncodeToString(sum[:])
		path := filepath.Join(cacheDir, "blobs", "sha256", digest[:2], digest[2:])
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return "sha256:" + digest
	}
	configData := `{"os":"linux","architecture":"amd64"}`
	config := put(configData)
	layerData := "synthetic layer"
	layer := put(layerData)
	manifestData := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":%q,"size":%d},"layers":[{"digest":%q,"size":%d}]}`, config, len(configData), layer, len(layerData))
	manifest := put(manifestData)
	chart := put("chart-tgz")
	cli := put("cli-binary")
	migration := put("SELECT 1;")
	packageDigest := put("synthetic package")
	dependencyDigest := put("synthetic package dependency")
	toolDigest := put("synthetic helm binary")
	indexData, err := json.Marshal(toolkit.OSDependencyIndex{SchemaVersion: "os-package-closure/v1", OS: "ubuntu-24.04", Arch: "amd64", Roots: []string{"kubeadm.deb"}, Packages: []toolkit.OSDependencyPackage{{Ref: "kubeadm.deb", Version: "1.36.5", Digest: packageDigest, Dependencies: []string{"libc.deb"}}, {Ref: "libc.deb", Version: "1", Digest: dependencyDigest}}})
	if err != nil {
		t.Fatal(err)
	}
	indexSum := sha256.Sum256(indexData)
	indexDigest := fmt.Sprintf("sha256:%x", indexSum)

	lock := toolkit.ReleaseLock{SchemaVersion: toolkit.LockSchemaVersion, ReleaseID: "r1", ReleaseVersion: "1.0",
		Images:        []toolkit.PinnedImage{{Name: "apiserver", Ref: "registry.invalid/apiserver:v1", Digest: manifest, Platform: "linux/amd64"}},
		Charts:        []toolkit.PinnedChart{{Name: "hnb", Version: "1.0", Ref: "oci://registry.invalid/hnb", Digest: chart}},
		CLI:           []toolkit.PinnedCLI{{OS: "linux", Arch: "amd64", Ref: "hnbctl", Digest: cli}},
		Migrations:    []toolkit.Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.TrimPrefix(migration, "sha256:")}},
		NodeArtifacts: []toolkit.NodeArtifact{{OS: "ubuntu-24.04", Arch: "amd64", Kind: "package-index", Ref: "os-index", Digest: indexDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "package", Ref: "kubeadm.deb", Digest: packageDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "binary", Ref: "helm", Digest: toolDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "container", Ref: "registry.invalid/pause:3.10", Digest: manifest}},
		Compatibility: toolkit.LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}, Profiles: []string{"Minimal"}}}
	raw, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	lockYAML := filepath.Join(t.TempDir(), "offline.yaml")
	if err := os.WriteFile(lockYAML, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	osIndex = filepath.Join(cacheDir, "os-index")
	if err := os.WriteFile(osIndex, indexData, 0o600); err != nil {
		t.Fatal(err)
	}
	sandbox = filepath.Join(cacheDir, "sandbox-image")
	if err := os.WriteFile(sandbox, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	return lockYAML, cacheDir, osIndex, sandbox
}

// 完整闭包：prepare → verify → import 全部成功.
func TestBundleLifecycleSucceedsWithCompleteClosure(t *testing.T) {
	lockPath, cacheDir, osIndex, sandbox := writeBundleFixture(t)
	manifest := filepath.Join(t.TempDir(), "bundle.json")

	code, out, errOut := runCLI(t, "bundle", "prepare", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--os-package-index", osIndex, "--sandbox-image", sandbox, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("bundle prepare failed: %d %s %s", code, out, errOut)
	}
	code, out, errOut = runCLI(t, "bundle", "verify", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("bundle verify failed: %d %s %s", code, out, errOut)
	}
	var verified map[string]any
	if err := json.Unmarshal([]byte(out), &verified); err != nil {
		t.Fatal(err)
	}
	if verified["complete"] != true {
		t.Fatalf("complete closure not verified: %s", out)
	}
	target := filepath.Join(t.TempDir(), "imported-cache")
	code, out, errOut = runCLI(t, "bundle", "import", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--target-dir", target, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("bundle import failed: %d %s %s", code, out, errOut)
	}
	var imported map[string]any
	if err := json.Unmarshal([]byte(out), &imported); err != nil {
		t.Fatal(err)
	}
	if imported["imported"] != true {
		t.Fatalf("import did not complete: %s", out)
	}
	if imported["offlineInstallCertified"] != false {
		t.Fatal("local import claimed offline installation certification")
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runCLI(t, "bundle", "verify", "--lock", lockPath, "--cache-dir", target, "--manifest", filepath.Join(target, "bundle.json"), "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("import did not preserve independent material: %d %s %s", code, out, errOut)
	}
}

// 缺少 OS 依赖或 sandbox 镜像时，prepare/import 均在节点修改前阻塞.
func TestBundleIncompleteClosureBlocksBeforeNodeModification(t *testing.T) {
	lockPath, cacheDir, osIndex, _ := writeBundleFixture(t)
	// Remove the locked sandbox root rather than just omitting its optional
	// locator flag (the root is otherwise already discoverable in the cache).
	lock, err := readValidatedStandaloneLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	lock.NodeArtifacts = lock.NodeArtifacts[:len(lock.NodeArtifacts)-1]
	data, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "bundle.json")

	// No sandbox image supplied at all.
	code, out, errOut := runCLI(t, "bundle", "prepare", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--os-package-index", osIndex, "--format", "json")
	if code != ExitSuccess {
		// A refusal at prepare is also acceptable: the closure is incomplete.
		if !strings.Contains(out+errOut, "closure") && !strings.Contains(out+errOut, "sandbox") {
			t.Fatalf("unexpected refusal: %d %s %s", code, out, errOut)
		}
		return
	}
	// If prepare succeeded, verify/import must still block on the missing role.
	code, out, errOut = runCLI(t, "bundle", "verify", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("incomplete closure verified: %s %s", out, errOut)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if v["complete"] != false {
		t.Fatalf("missing sandbox image not detected: %s", out)
	}
	if code, _, _ := runCLI(t, "bundle", "import", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--format", "json"); code == ExitSuccess {
		t.Fatal("incomplete closure imported")
	}
}

// 篡改物料后 verify 失败，import 随之被阻断.
func TestBundleCorruptionBlocksImport(t *testing.T) {
	lockPath, cacheDir, osIndex, sandbox := writeBundleFixture(t)
	manifest := filepath.Join(t.TempDir(), "bundle.json")
	if code, out, errOut := runCLI(t, "bundle", "prepare", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--os-package-index", osIndex, "--sandbox-image", sandbox, "--format", "json"); code != ExitSuccess {
		t.Fatalf("prepare failed: %d %s %s", code, out, errOut)
	}
	// Corrupt one material on disk.
	if err := os.WriteFile(osIndex, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, "bundle", "verify", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("corrupted bundle verified: %s", out)
	}
	if !strings.Contains(out, "complete") && !strings.Contains(errOut, "complete") {
		t.Fatalf("unexpected verification output: %s %s", out, errOut)
	}
	if code, _, _ := runCLI(t, "bundle", "import", "--lock", lockPath, "--cache-dir", cacheDir,
		"--manifest", manifest); code == ExitSuccess {
		t.Fatal("corrupted bundle imported")
	}
}

func TestBundleRequiresFlagsAndRejectsUnknownSubcommand(t *testing.T) {
	code, _, errOut := runCLI(t, "bundle")
	if code == ExitSuccess || !strings.Contains(errOut, "bundle subcommand required") {
		t.Fatalf("bare bundle accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "bundle", "export")
	if code == ExitSuccess || !strings.Contains(errOut, "unknown bundle subcommand") {
		t.Fatalf("unknown bundle subcommand accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "bundle", "verify", "--lock", "a.yaml")
	if code == ExitSuccess || !strings.Contains(errOut, "--cache-dir") {
		t.Fatalf("bundle without --cache-dir accepted: %d %s", code, errOut)
	}
}

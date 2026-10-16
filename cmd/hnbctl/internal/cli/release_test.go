package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func writeBlob(t *testing.T, dir, content string) string {
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

// buildReleaseCLIFixture builds a verified offline closure identical in shape
// to the toolkit fixtures, returning the lock path, bundle path and cache root.
func buildReleaseCLIFixture(t *testing.T) (lockPath, bundlePath, cacheDir string) {
	t.Helper()
	cacheDir = t.TempDir()
	config := writeBlob(t, cacheDir, `{"os":"linux","architecture":"amd64"}`)
	layerData := strings.Repeat("layer", 1000)
	layer := writeBlob(t, cacheDir, layerData)
	manifest := writeBlob(t, cacheDir, `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":"`+config+`","size":`+strconv.Itoa(len(`{"os":"linux","architecture":"amd64"}`))+`},"layers":[{"digest":"`+layer+`","size":`+strconv.Itoa(len(layerData))+`}]}`)
	chart := writeBlob(t, cacheDir, "chart-tgz")
	cli := writeBlob(t, cacheDir, "cli-binary")
	migration := writeBlob(t, cacheDir, "SELECT 1;")
	packageDigest := writeBlob(t, cacheDir, "synthetic kubeadm package")
	dependencyDigest := writeBlob(t, cacheDir, "synthetic libc dependency")
	toolDigest := writeBlob(t, cacheDir, "synthetic helm binary")
	indexData, err := json.Marshal(toolkit.OSDependencyIndex{SchemaVersion: "os-package-closure/v1", OS: "ubuntu-24.04", Arch: "amd64", Roots: []string{"kubeadm.deb"}, Packages: []toolkit.OSDependencyPackage{{Ref: "kubeadm.deb", Version: "1.36.5", Digest: packageDigest, Dependencies: []string{"libc.deb"}}, {Ref: "libc.deb", Version: "1", Digest: dependencyDigest}}})
	if err != nil {
		t.Fatal(err)
	}
	osIndex := writeBlob(t, cacheDir, string(indexData))
	lock := &toolkit.ReleaseLock{SchemaVersion: toolkit.LockSchemaVersion, ReleaseID: "r1", ReleaseVersion: "1.0",
		Images:        []toolkit.PinnedImage{{Name: "apiserver", Ref: "registry.invalid/apiserver:v1", Digest: manifest, Platform: "linux/amd64"}},
		Charts:        []toolkit.PinnedChart{{Name: "hnb", Version: "1.0", Ref: "oci://registry.invalid/hnb", Digest: chart}},
		CLI:           []toolkit.PinnedCLI{{OS: "linux", Arch: "amd64", Ref: "hnbctl", Digest: cli}},
		Migrations:    []toolkit.Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.TrimPrefix(migration, "sha256:")}},
		NodeArtifacts: []toolkit.NodeArtifact{{OS: "ubuntu-24.04", Arch: "amd64", Kind: "package-index", Ref: "ubuntu-24.04-index", Digest: osIndex}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "package", Ref: "kubeadm.deb", Digest: packageDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "binary", Ref: "helm", Digest: toolDigest}, {OS: "ubuntu-24.04", Arch: "amd64", Kind: "container", Ref: "registry.invalid/pause:3.10", Digest: manifest}},
		Compatibility: toolkit.LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}
	lockData, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	lockPath = filepath.Join(t.TempDir(), "release.lock.json")
	if err := os.WriteFile(lockPath, lockData, 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := toolkit.BuildOfflineBundle(lock, cacheDir,
		[]toolkit.BundleEntry{{Role: "os-package-index", Name: "ubuntu-24.04-index", Digest: osIndex}},
		[]toolkit.BundleEntry{{Role: "sandbox-image", Name: "registry.invalid/pause:3.10", Digest: manifest}},
		time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	bundleData, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundlePath = filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(bundlePath, bundleData, 0o600); err != nil {
		t.Fatal(err)
	}
	return lockPath, bundlePath, cacheDir
}

func TestReleaseVerifyIsOffline(t *testing.T) {
	lockPath, bundlePath, cacheDir := buildReleaseCLIFixture(t)
	code, out, errOut := runCLI(t, "release", "verify",
		"--bundle", bundlePath, "--lock", lockPath, "--cache-dir", cacheDir, "--role", "worker")
	if code != ExitSuccess {
		t.Fatalf("offline verify failed: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "closure=ready") {
		t.Fatalf("output missing closure: %s", out)
	}
	// verify refuses platform flags so it can never fall back to a network source
	code, _, errOut = runCLI(t, "release", "verify",
		"--bundle", bundlePath, "--lock", lockPath, "--cache-dir", cacheDir, "--role", "worker",
		"--platform-url", "https://api.invalid")
	if code != ExitUsage || !strings.Contains(errOut, "offline") {
		t.Fatalf("verify accepted platform flags: %d %s", code, errOut)
	}
}

// releasePlatformFixture is a live TLS owning-plane recording the register body.
func releasePlatformFixture(t *testing.T, status int, body string) (*httptest.Server, map[string]any) {
	t.Helper()
	captured := map[string]any{}
	if body == "" {
		body = `{"id":"release-1","releaseId":"r1","releaseVersion":"1.0","role":"worker","bundleId":"bundle-1111111111111111","lockDigest":"` + strings.Repeat("1", 64) + `","contentDigest":"` + strings.Repeat("2", 64) + `"}`
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func TestReleaseRegisterSubmitsVerifiedClosure(t *testing.T) {
	lockPath, bundlePath, cacheDir := buildReleaseCLIFixture(t)
	server, captured := releasePlatformFixture(t, http.StatusCreated, "")
	platformClientOverride = server.Client()
	t.Cleanup(func() { platformClientOverride = nil })
	token := writeTokenFile(t)
	code, out, errOut := runCLI(t, "release", "register",
		"--bundle", bundlePath, "--lock", lockPath, "--cache-dir", cacheDir, "--role", "worker",
		"--platform-url", server.URL, "--token-file", token)
	if code != ExitSuccess {
		t.Fatalf("register failed: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "registered") {
		t.Fatalf("output missing registration: %s", out)
	}
	if captured["role"] != "worker" || captured["releaseId"] != "r1" {
		t.Fatalf("captured body = %v", captured)
	}
	bundleRaw, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var localBundle toolkit.OfflineBundle
	if err := json.Unmarshal(bundleRaw, &localBundle); err != nil {
		t.Fatal(err)
	}
	if captured["contentDigest"] != localBundle.ContentDigest {
		t.Fatalf("content digest not submitted: %v", captured["contentDigest"])
	}
}

func TestReleaseRegisterReplaysSameClosure(t *testing.T) {
	lockPath, bundlePath, cacheDir := buildReleaseCLIFixture(t)
	server, _ := releasePlatformFixture(t, http.StatusOK, "")
	platformClientOverride = server.Client()
	t.Cleanup(func() { platformClientOverride = nil })
	token := writeTokenFile(t)
	code, out, errOut := runCLI(t, "release", "register",
		"--bundle", bundlePath, "--lock", lockPath, "--cache-dir", cacheDir, "--role", "worker",
		"--platform-url", server.URL, "--token-file", token)
	if code != ExitSuccess || !strings.Contains(out, "already present") {
		t.Fatalf("replay not idempotent: %d %s %s", code, out, errOut)
	}
}

func TestReleaseRegisterSurfacesFixedReleaseConflict(t *testing.T) {
	lockPath, bundlePath, cacheDir := buildReleaseCLIFixture(t)
	server, _ := releasePlatformFixture(t, http.StatusConflict, "")
	platformClientOverride = server.Client()
	t.Cleanup(func() { platformClientOverride = nil })
	token := writeTokenFile(t)
	code, _, errOut := runCLI(t, "release", "register",
		"--bundle", bundlePath, "--lock", lockPath, "--cache-dir", cacheDir, "--role", "worker",
		"--platform-url", server.URL, "--token-file", token)
	if code != ExitUsage || !strings.Contains(errOut, "CONFLICT") {
		t.Fatalf("fixed release conflict not surfaced: %d %s", code, errOut)
	}
}

func TestReleaseList(t *testing.T) {
	listBody := `{"releases":[` + `{"id":"release-1","releaseId":"r1","releaseVersion":"1.0","role":"worker","bundleId":"bundle-1111111111111111","lockDigest":"` + strings.Repeat("1", 64) + `","contentDigest":"` + strings.Repeat("2", 64) + `"}` + `],"total":1}`
	server, _ := releasePlatformFixture(t, http.StatusOK, listBody)
	platformClientOverride = server.Client()
	t.Cleanup(func() { platformClientOverride = nil })
	token := writeTokenFile(t)
	code, out, errOut := runCLI(t, "release", "list",
		"--platform-url", server.URL, "--token-file", token, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("list failed: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, `"role": "worker"`) && !strings.Contains(out, "release-1") {
		t.Fatalf("unexpected list output: %s", out)
	}
}

func TestReleaseRequiresValidRole(t *testing.T) {
	lockPath, bundlePath, cacheDir := buildReleaseCLIFixture(t)
	code, _, errOut := runCLI(t, "release", "verify",
		"--bundle", bundlePath, "--lock", lockPath, "--cache-dir", cacheDir, "--role", "gateway")
	if code != ExitUsage || !strings.Contains(errOut, "--role console, control-plane or worker") {
		t.Fatalf("invalid role accepted: %d %s", code, errOut)
	}
}

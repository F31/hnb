package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"gopkg.in/yaml.v3"
)

func prepareCLIFixture(t *testing.T) (configPath, lockPath, cache string) {
	t.Helper()
	dir := t.TempDir()
	cache = filepath.Join(dir, "cache")
	blobDir := filepath.Join(cache, "blobs", "sha256")
	if err := os.MkdirAll(blobDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{"os":"linux","architecture":"amd64"}`)
	layer := []byte("fixture layer")
	digest := func(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
	manifest := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":%q,"size":%d},"layers":[{"digest":%q,"size":%d}]}`, digest(config), len(config), digest(layer), len(layer)))
	for _, data := range [][]byte{manifest, config, layer} {
		if err := os.WriteFile(filepath.Join(blobDir, strings.TrimPrefix(digest(data), "sha256:")), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	lock := toolkit.ReleaseLock{SchemaVersion: toolkit.LockSchemaVersion, ReleaseID: "fixture", ReleaseVersion: "test", Images: []toolkit.PinnedImage{{Name: "fixture", Ref: "registry.invalid/fixture:v1", Digest: digest(manifest)}}, Compatibility: toolkit.LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}, Profiles: []string{"Minimal"}}}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	lockPath = filepath.Join(dir, "release.lock.yaml")
	if err := os.WriteFile(lockPath, lockData, 0600); err != nil {
		t.Fatal(err)
	}
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	for from, to := range map[string]string{"kubeconfigFile: \"\"": "kubeconfigFile: ./unused-kubeconfig", "publicBaseURL: \"\"": "publicBaseURL: https://fixture.invalid", "existingSecret: \"\"": "existingSecret: fixture-tls", "className: \"\"": "className: fixture-storage"} {
		draft = strings.Replace(draft, from, to, 1)
	}
	configPath = filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(configPath, []byte(draft), 0600); err != nil {
		t.Fatal(err)
	}
	return configPath, lockPath, cache
}

func TestPrepareCLIUsesVerifiedCacheWithoutClaimingInstallation(t *testing.T) {
	configPath, lockPath, cache := prepareCLIFixture(t)
	code, out, progress := runCLI(t, "prepare", "-f", configPath, "--lock", lockPath, "--cache-dir", cache)
	if code != ExitSuccess {
		t.Fatalf("prepare failed: %d %s %s", code, out, progress)
	}
	var result toolkit.PrepareResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "verified" || result.LockDigest == "" || result.StartedAt.IsZero() || result.FinishedAt.Before(result.StartedAt) || result.InstallAuthorized || result.FromCache != 3 || !strings.Contains(progress, "cache-hit") {
		t.Fatalf("incorrect preparation result: %+v %s", result, progress)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), "unused-kubeconfig")); !os.IsNotExist(err) {
		t.Fatal("prepare accessed/created cluster credentials")
	}
}

func TestPrepareMirrorRequiresExplicitOriginBeforeCreatingCache(t *testing.T) {
	configPath, lockPath, _ := prepareCLIFixture(t)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(strings.Replace(string(data), "mode: online", "mode: mirror", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "must-not-exist")
	code, out, msg := runCLI(t, "prepare", "-f", configPath, "--lock", lockPath, "--cache-dir", cache)
	if code != ExitUsage || out != "" || !strings.Contains(msg, "requires --mirror-origin") {
		t.Fatalf("mirror mode silently fell back to origin: %d %s %s", code, out, msg)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("missing mirror mapping created cache")
	}
}

func TestPrepareFailureOutputsVerifiedPartialResultAndCanRetry(t *testing.T) {
	configPath, lockPath, cache := prepareCLIFixture(t)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := toolkit.DecodeReleaseLock(data)
	if err != nil {
		t.Fatal(err)
	}
	cliBytes := []byte("test-cli-content")
	lock.CLI = []toolkit.PinnedCLI{{OS: "linux", Arch: "amd64", Ref: "hnbctl", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(cliBytes))}}
	data, err = yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	code, out, msg := runCLI(t, "prepare", "-f", configPath, "--lock", lockPath, "--cache-dir", cache)
	if code == ExitSuccess || !strings.Contains(msg, "PREPARE_FAILED") {
		t.Fatalf("missing artifact reported success: %d %s %s", code, out, msg)
	}
	var partial toolkit.PrepareResult
	if err := json.Unmarshal([]byte(out), &partial); err != nil {
		t.Fatalf("missing structured partial result: %v %s", err, out)
	}
	if partial.Status != "failed" || partial.Error == "" || partial.LockDigest == "" || len(partial.Artifacts) != 3 || partial.FromCache != 3 || partial.BytesVerified <= 0 || partial.InstallAuthorized {
		t.Fatalf("incorrect failed preparation summary: %+v", partial)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(lockPath), "hnbctl"), cliBytes, 0600); err != nil {
		t.Fatal(err)
	}
	code, out, msg = runCLI(t, "prepare", "-f", configPath, "--lock", lockPath, "--cache-dir", cache)
	if code != ExitSuccess {
		t.Fatalf("retry failed: %d %s %s", code, out, msg)
	}
	var completed toolkit.PrepareResult
	if err := json.Unmarshal([]byte(out), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.Status != "verified" || completed.Error != "" || completed.LockDigest != partial.LockDigest || len(completed.Artifacts) != 4 || completed.FromCache != 3 {
		t.Fatalf("retry lost verified state or release identity: %+v", completed)
	}
}

func TestPrepareMirrorMappingIsPreparationOnly(t *testing.T) {
	configPath, lockPath, cache := prepareCLIFixture(t)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "mode: online", "mode: mirror", 1))
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	code, out, msg := runCLI(t, "prepare", "-f", configPath, "--lock", lockPath, "--cache-dir", cache, "--mirror-origin", "https://mirror.invalid")
	if code != ExitSuccess {
		t.Fatalf("explicit mirror preparation is unreachable: %d %s %s", code, out, msg)
	}
	var result toolkit.PrepareResult
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Status != "verified" || result.FromCache != 3 || result.InstallAuthorized {
		t.Fatalf("mirror preparation incorrectly authorized installation: %+v %v", result, err)
	}
	cfg, err := toolkit.DecodeConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) == 0 || cfg.Spec.Artifacts.Mode != toolkit.ArtifactsMirror {
		t.Fatal("preparation validation relaxed installation or changed the configured mode")
	}
	for _, origin := range []string{"http://mirror.invalid", "https://mirror.invalid/path", "https://user:password@mirror.invalid", "https://mirror.invalid?token=must-not-leak"} {
		code, out, msg := runCLI(t, "prepare", "-f", configPath, "--lock", lockPath, "--cache-dir", cache, "--mirror-origin", origin)
		if code != ExitUsage || out != "" || strings.Contains(msg, "must-not-leak") {
			t.Fatalf("invalid mirror mapping accepted or leaked: %d %s %s", code, out, msg)
		}
	}
}

func TestPreparePreloadRefusesBeforeAnyFilesOrNodes(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "must-not-exist")
	code, _, msg := runCLI(t, "prepare", "-f", "missing.yaml", "--cache-dir", cache, "--preload")
	if code != ExitBlocked || !strings.Contains(msg, "ownership") {
		t.Fatalf("preload bypassed authorization: %d %s", code, msg)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("blocked preload created cache")
	}
}

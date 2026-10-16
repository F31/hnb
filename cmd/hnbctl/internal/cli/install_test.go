package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// TestInstallExistingEmitsPlanAndRefusesAuthorization proves the 4.3 boundary at
// the CLI surface: an existing-cluster config yields a dependency plan plus an
// ownership manifest, external CNI/owner are preserved, no node-init action is
// produced, and the command still refuses to authorize an install.
func TestInstallExistingEmitsPlanAndRefusesAuthorization(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "hnb.yaml")
	outputPath := filepath.Join(dir, "plan.json")

	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	draft = strings.Replace(draft, "kubeconfigFile: \"\"", "kubeconfigFile: ./fixture-kubeconfig", 1)
	draft = strings.Replace(draft, "publicBaseURL: \"\"", "publicBaseURL: https://fixture.invalid", 1)
	draft = strings.Replace(draft, "existingSecret: \"\"", "existingSecret: fixture-tls", 1)
	draft = strings.Replace(draft, "className: \"\"", "className: fixture-storage", 1)
	lock := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\nimages:\n  - name: fixture\n    ref: registry.invalid/fixture:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\ncompatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"
	for name, data := range map[string]string{"hnb.yaml": draft, "release.lock.yaml": lock, "fixture-kubeconfig": "fixture-only"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	code, _, errOut := runCLI(t, "install", "-f", configPath, "--runs-dir", filepath.Join(dir, "runs"), "--output", outputPath)
	if code != ExitNotVerified {
		t.Fatalf("existing install must not report authorization: %d %s", code, errOut)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(outputPath)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("plan file must be private: %v %v", fi, err)
	}
	var doc struct {
		Ownership toolkit.OwnershipManifest   `json:"ownership"`
		Plan      toolkit.ExistingInstallPlan `json:"plan"`
		Reasons   []string                    `json:"reasons"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Ownership.NoNodeInitAction || !doc.Ownership.FitsBoundary {
		t.Fatalf("ownership boundary broken: %+v", doc.Ownership)
	}
	if doc.Plan.RoutingMode != "external-cluster cluster.reader/shadow" {
		t.Fatalf("wrong routing mode: %+v", doc.Plan)
	}
	if len(doc.Plan.Stages) == 0 {
		t.Fatal("no dependency stages produced")
	}
	joined := string(data)
	for _, want := range []string{"NO_NODE_INIT_ACTION", "PLANNING_ONLY", "external CNI daemonsets/deployments"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("plan missing required boundary statement %q: %s", want, joined)
		}
	}
	for _, forbidden := range []string{"kubeadm", "password=", "BEGIN EC PRIVATE KEY"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("plan leaked a node-init action or secret: %s", forbidden)
		}
	}
}

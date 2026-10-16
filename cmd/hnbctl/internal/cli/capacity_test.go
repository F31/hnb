package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const capacityNodesYAML = `- name: cp-1
  cpuMilli: 16000
  memoryMi: 32768
  diskGi: 200
`

const capacityManifestYAML = `kind: Deployment
metadata: {name: apiserver}
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: main
        resources: {requests: {cpu: 500m, memory: 256Mi}}
      - name: sidecar
        resources: {requests: {cpu: 100m, memory: 64Mi}}
      initContainers:
      - name: migration
        resources: {requests: {cpu: "1", memory: 128Mi}}
---
kind: PersistentVolumeClaim
metadata: {name: data}
spec:
  resources: {requests: {storage: 10Gi}}
`

func TestCapacityWithoutEvidenceNeverAuthorizes(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "rendered.yaml")
	nodesPath := filepath.Join(dir, "nodes.yaml")
	if err := os.WriteFile(manifestPath, []byte(capacityManifestYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodesPath, []byte(capacityNodesYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, "capacity", "--manifest", manifestPath, "--nodes", nodesPath)
	if code != ExitSuccess {
		t.Fatalf("capacity planning should succeed without evidence: %d %s", code, errOut)
	}
	var report struct {
		FitsDeclaredCapacity bool     `json:"fitsDeclaredCapacity"`
		InstallAuthorized    bool     `json:"installAuthorized"`
		Reasons              []string `json:"reasons"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.InstallAuthorized {
		t.Fatalf("capacity without evidence must not authorize: %+v", report)
	}
	for _, reason := range report.Reasons {
		if strings.HasPrefix(reason, "INSTALL_AUTHORIZATION_DENIED") {
			t.Fatalf("no evidence input must not add denial reason: %v", report.Reasons)
		}
	}
}

// --evidence wires the capability gate: with the existing-route Minimal delivery
// certified, a fitting capacity plus material-verified authorizes the install.
func TestCapacityEvidenceGateAuthorizesCertifiedExisting(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	manifestPath := filepath.Join(t.TempDir(), "rendered.yaml")
	nodesPath := filepath.Join(t.TempDir(), "nodes.yaml")
	if err := os.WriteFile(manifestPath, []byte(capacityManifestYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodesPath, []byte(capacityNodesYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runCLI(t, "capacity", "--manifest", manifestPath, "--nodes", nodesPath,
		"--evidence", configPath, "--material-verified")
	if code != ExitSuccess {
		t.Fatalf("certified evidence with fitting capacity must authorize: %d %s", code, errOut)
	}
	var report struct {
		InstallAuthorized bool     `json:"installAuthorized"`
		FitsDeclared      bool     `json:"fitsDeclaredCapacity"`
		Reasons           []string `json:"reasons"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if !report.InstallAuthorized || !report.FitsDeclared {
		t.Fatalf("expected authorization with certified evidence: %+v", report)
	}
	for _, reason := range report.Reasons {
		if strings.HasPrefix(reason, "INSTALL_AUTHORIZATION_DENIED") {
			t.Fatalf("certified evidence must not add denial reason: %+v", report.Reasons)
		}
	}
}

// An invalid evidence config is refused before any report is produced.
func TestCapacityEvidenceRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "rendered.yaml")
	nodesPath := filepath.Join(dir, "nodes.yaml")
	if err := os.WriteFile(manifestPath, []byte(capacityManifestYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodesPath, []byte(capacityNodesYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	badConfig := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badConfig, []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "capacity", "--manifest", manifestPath, "--nodes", nodesPath, "--evidence", badConfig)
	if code == ExitSuccess || !strings.Contains(errOut, "evidence config") {
		t.Fatalf("invalid evidence config must be refused: %d %s", code, errOut)
	}
}

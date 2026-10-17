package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func TestAutoUninstallAllDataPlan(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *"get deployments"*) echo deployment.apps/apiserver ;;
  *"get all"*) echo deployment.apps/apiserver ;;
  *"get persistentvolume"*) echo '{"items":[]}' ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, out, errOut := runCLI(t, "uninstall", "--all", "--data", "--plan", "--format", "json", "--kubeconfig", "/tmp/kubeconfig")
	if code != ExitSuccess {
		t.Fatalf("auto plan failed: %d %s %s", code, out, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Delete, "\n")
	for _, want := range []string{"persistentvolumeclaim/*", "persistentvolume/*", "secret/*", "namespace/hnb-system", "namespace/topolvm-system", "storageclass/hnb-topolvm", "crd/logicalvolumes.topolvm.io"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in delete plan: %v", want, plan.Delete)
		}
	}
	if strings.Contains(strings.Join(plan.Retain, "\n"), "namespace/hnb-system") {
		t.Fatalf("data plan retained namespace: %v", plan.Retain)
	}
}

func TestAutoUninstallDefaultsToComponentsOnly(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *"get deployments"*) echo deployment.apps/apiserver ;;
  *"get all"*) echo deployment.apps/apiserver ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, out, errOut := runCLI(t, "uninstall", "--plan", "--format", "json", "--kubeconfig", "/tmp/kubeconfig")
	if code != ExitSuccess {
		t.Fatalf("auto plan failed: %d %s %s", code, out, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Delete, "\n")
	if strings.Contains(joined, "persistentvolumeclaim") || strings.Contains(joined, "secret/*") {
		t.Fatalf("default plan deleted data: %v", plan.Delete)
	}
	if strings.Contains(strings.Join(plan.Retain, "\n"), "namespace/topolvm-system") == false {
		t.Fatalf("default plan did not retain TopoLVM: %v", plan.Retain)
	}
}

func TestAutoUninstallDetectsTopolvmRemnants(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *"get deployments"*) exit 0 ;;
  *"get all"*) exit 0 ;;
  *"get storageclass hnb-topolvm"*) echo storageclass.storage.k8s.io/hnb-topolvm ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, out, errOut := runCLI(t, "uninstall", "-all", "-data", "--plan", "--format", "json", "--kubeconfig", "/tmp/kubeconfig")
	if code != ExitSuccess {
		t.Fatalf("auto remnant plan failed: %d %s %s", code, out, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Delete, "\n"), "namespace/topolvm-system") {
		t.Fatalf("topolvm remnants not scheduled: %v", plan.Delete)
	}
}

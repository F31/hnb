package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

const fixtureKubeconfigYAML = `apiVersion: v1
kind: Config
current-context: demo
clusters:
- name: demo
  cluster:
    server: https://10.0.0.1:6443
contexts:
- name: demo
  context:
    cluster: demo
`

// init --from-kubeconfig 反推 existing 草稿：填入 kubeconfigFile 并报告 API 端点.
func TestInitFromKubeconfigFillsDraft(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "cluster.kubeconfig")
	if err := os.WriteFile(kubeconfig, []byte(fixtureKubeconfigYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "hnb.yaml")
	code, stdout, errOut := runCLI(t, "init", "--from-kubeconfig", kubeconfig, "-f", out)
	if code != ExitSuccess {
		t.Fatalf("init --from-kubeconfig failed: %d %s", code, errOut)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(kubeconfig)
	if !strings.Contains(string(data), "kubeconfigFile: \""+abs+"\"") {
		t.Fatalf("kubeconfigFile not filled: %s", data)
	}
	if !strings.Contains(string(data), "mode: existing") {
		t.Fatalf("draft is not existing-mode: %s", data)
	}
	if !strings.Contains(stdout, "10.0.0.1:6443") {
		t.Fatalf("observed API server not reported: %s", stdout)
	}
	// 草稿仍保留必填占位，不虚构入口/证书/存储.
	for _, required := range []string{"publicBaseURL", "existingSecret", "className"} {
		if !strings.Contains(string(data), required) {
			t.Fatalf("draft dropped required placeholder %q", required)
		}
	}
}

func TestInitFromKubeconfigRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.kubeconfig")
	if err := os.WriteFile(bad, []byte(strings.Replace(fixtureKubeconfigYAML, "https://", "http://", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCLI(t, "init", "--from-kubeconfig", bad, "-f", filepath.Join(dir, "x.yaml")); code == ExitSuccess || !strings.Contains(errOut, "https") {
		t.Fatalf("non-https kubeconfig accepted: %d %s", code, errOut)
	}
	if code, _, errOut := runCLI(t, "init", "--from-kubeconfig", bad, "--mode", "bootstrap", "-f", filepath.Join(dir, "y.yaml")); code == ExitSuccess || !strings.Contains(errOut, "implies --mode existing") {
		t.Fatalf("mode conflict accepted: %d %s", code, errOut)
	}
}

// logs 过滤：--phase/--after 只返回匹配事件.
func TestLogsFiltersByPhaseAndAfter(t *testing.T) {
	args, root := quickExistingArgs(t)
	planShortcut(t, args)

	code, out, errOut := runCLI(t, "logs", "--runs-dir", root, "--phase", "planning", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("logs --phase failed: %d %s", code, errOut)
	}
	var doc struct {
		Events []toolkit.RunEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Events) == 0 {
		t.Fatal("phase filter returned nothing")
	}
	for _, event := range doc.Events {
		if event.Phase != toolkit.PhasePlanning {
			t.Fatalf("non-matching phase returned: %+v", event)
		}
	}

	first := doc.Events[0].Sequence
	code, out, _ = runCLI(t, "logs", "--runs-dir", root, "--phase", "planning", "--after", strconv.FormatInt(first, 10), "--format", "json")
	if code != ExitSuccess {
		t.Fatal("logs --after failed")
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	for _, event := range doc.Events {
		if event.Sequence <= first {
			t.Fatalf("--after did not exclude earlier event: %+v", event)
		}
	}

	code, out, _ = runCLI(t, "logs", "--runs-dir", root, "--phase", "no-such-phase", "--format", "json")
	if code != ExitSuccess {
		t.Fatal("missing phase must be an empty result")
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Events) != 0 {
		t.Fatalf("unexpected events for missing phase: %+v", doc.Events)
	}
}

// history 过滤：--state 只返回匹配状态，且计数基于过滤后集合.
func TestHistoryFiltersByState(t *testing.T) {
	args, root := quickExistingArgs(t)
	planShortcut(t, args)

	code, out, _ := runCLI(t, "history", "--runs-dir", root, "--state", "blocked", "--format", "json")
	if code != ExitSuccess {
		t.Fatal("history --state failed")
	}
	var doc struct {
		Total int                  `json:"total"`
		Runs  []toolkit.RunSummary `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Total != 1 || len(doc.Runs) != 1 || doc.Runs[0].Status != "blocked" {
		t.Fatalf("state filter mismatch: %+v", doc)
	}

	code, out, _ = runCLI(t, "history", "--runs-dir", root, "--state", "succeeded", "--format", "json")
	if code != ExitSuccess {
		t.Fatal("history --state succeeded failed")
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Total != 0 || len(doc.Runs) != 0 {
		t.Fatalf("expected empty result: %+v", doc)
	}
}

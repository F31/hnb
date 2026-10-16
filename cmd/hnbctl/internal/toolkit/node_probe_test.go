package toolkit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const nodeProbeFixture = `os=ubuntu
osVersion=24.04
kernel=6.8.0-31-generic
arch=x86_64
cgroup=cgroup2fs
uid=0
cpu=2
memoryKiB=4000000
diskKiB=20000000
swapEntries=0
owner=none
`

type fixtureProbe struct {
	mu       sync.Mutex
	response string
	err      error
	calls    int
}

func (r *fixtureProbe) Run(_ context.Context, _ NodeConfig, script string) (string, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if script != NodeProbeScript {
		return "", fmt.Errorf("unexpected program")
	}
	return r.response, r.err
}

func TestNodeProbeCandidateChecksAndBlockingConditions(t *testing.T) {
	cases := []struct{ field, value, check string }{
		{"os=ubuntu", "os=debian", "node.os-kernel"},
		{"kernel=6.8.0-31-generic", "kernel=6.8.0-unverified", "node.os-kernel"},
		{"arch=x86_64", "arch=aarch64", "node.arch"},
		{"cgroup=cgroup2fs", "cgroup=tmpfs", "node.cgroup"},
		{"uid=0", "uid=1000", "node.permission"},
		{"cpu=2", "cpu=1", "node.cpu"},
		{"memoryKiB=4000000", "memoryKiB=1000000", "node.memory"},
		{"diskKiB=20000000", "diskKiB=1000000", "node.disk"},
		{"swapEntries=0", "swapEntries=1", "node.swap"},
		{"owner=none", "owner=hnb", "node.owner-markers"},
		{"owner=none", "owner=kubernetes", "node.owner-markers"},
	}
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fixtureProbe{response: nodeProbeFixture}
	checks := ProbeNodes(context.Background(), cfg, runner)
	if len(checks) != 9 || runner.calls != 1 {
		t.Fatalf("missing checks: %+v", checks)
	}
	for _, check := range checks {
		if check.Status != StatusPass || check.ObservedAt.IsZero() || check.Node != "cp-1" {
			t.Fatalf("unexpected candidate verdict: %+v", check)
		}
	}
	for _, tc := range cases {
		t.Run(tc.check+tc.value, func(t *testing.T) {
			runner := &fixtureProbe{response: strings.Replace(nodeProbeFixture, tc.field, tc.value, 1)}
			found := false
			for _, check := range ProbeNodes(context.Background(), cfg, runner) {
				if check.CheckID == tc.check {
					found = true
					if check.Status != StatusBlocked || check.Remediation == "" {
						t.Fatalf("unsafe observation passed: %+v", check)
					}
				}
			}
			if !found {
				t.Fatal("missing blocking check")
			}
		})
	}
}

func TestNodeProbeRejectsMalformedOrUnavailableObservations(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{
		"", nodeProbeFixture + "token=secret\n", nodeProbeFixture + "uid=0\n",
		strings.Replace(nodeProbeFixture, "cpu=2", "cpu=-1", 1),
		strings.Replace(nodeProbeFixture, "kernel=6.8.0-31-generic", "kernel=postgres://user:secret@db", 1),
		strings.Repeat("x", 17000),
	} {
		checks := ProbeNodes(context.Background(), cfg, &fixtureProbe{response: response})
		if len(checks) != 1 || checks[0].ReasonCode != "NODE_OBSERVATION_INVALID" || strings.Contains(checks[0].Actual, "secret") {
			t.Fatalf("invalid response leaked/passed: %+v", checks)
		}
	}
	checks := ProbeNodes(context.Background(), cfg, &fixtureProbe{err: fmt.Errorf("password=secret")})
	if len(checks) != 1 || checks[0].ReasonCode != "NODE_PROBE_UNAVAILABLE" || strings.Contains(checks[0].Actual, "secret") {
		t.Fatalf("transport error leaked/passed: %+v", checks)
	}
	cfg.Spec.Mode = ModeManaged
	runner := &fixtureProbe{response: nodeProbeFixture}
	checks = ProbeNodes(context.Background(), cfg, runner)
	if runner.calls != 0 || checks[0].ReasonCode != "CONFIG_INVALID" {
		t.Fatal("invalid configuration contacted a node")
	}
}

func TestUbuntuMultiNodeEvidenceCannotBeInferred(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	worker := cfg.Spec.Cluster.Nodes[0]
	worker.Name, worker.SSHAddress, worker.InternalAddress = "worker", "10.0.0.11", "10.0.0.11"
	worker.Roles = []NodeRole{RoleWorker}
	cfg.Spec.Cluster.Nodes = append(cfg.Spec.Cluster.Nodes, worker)
	checks := ProbeNodes(context.Background(), cfg, &fixtureProbe{response: nodeProbeFixture})
	found := 0
	for _, check := range checks {
		if check.ReasonCode == "NODE_TOPOLOGY_UNSUPPORTED" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("Ubuntu multi-node evidence inferred: %+v", checks)
	}
}

func TestObservedPlatformMustMatchSelectedLock(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	lock := &ReleaseLock{Compatibility: LockCompatibility{OS: []string{"rocky-10.1"}, Arch: []string{"arm64"}}}
	checks := ProbeLockedNodes(context.Background(), cfg, lock, &fixtureProbe{response: nodeProbeFixture})
	found := 0
	for _, check := range checks {
		if check.ReasonCode == "NODE_OS_RECIPE_MISMATCH" || check.ReasonCode == "NODE_ARCH_RECIPE_MISMATCH" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("observed node silently used another recipe: %+v", checks)
	}
	runner := &fixtureProbe{response: nodeProbeFixture}
	checks = ProbeLockedNodes(context.Background(), cfg, nil, runner)
	if runner.calls != 0 || checks[0].ReasonCode != "NODE_RECIPE_MISSING" {
		t.Fatal("missing recipe contacted a node")
	}
}

func TestRealProbeProgramDoesNotChangeFixtureConfiguration(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"etc/os-release":             "ID=\"ubuntu\"\nVERSION_ID=\"24.04\"\n",
		"proc/meminfo":               "MemTotal: 4000000 kB\n",
		"proc/swaps":                 "Filename Type Size Used Priority\n",
		"etc/kubernetes/admin.conf":  "synthetic kubeconfig contents must never be read or printed",
		"etc/sysctl.conf":            "customer sysctl configuration",
		"etc/containerd/config.toml": "customer runtime configuration",
	}
	for path, contents := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "sys/fs/cgroup"), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		state := map[string]string{}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			value := info.Mode().String()
			if info.Mode().IsRegular() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += string(data)
			}
			state[path] = value
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := snapshot()
	script := strings.Replace(NodeProbeScript, "ROOT=/", fmt.Sprintf("ROOT=%q", root), 1)
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader(script)
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	observation, err := parseNodeObservation(string(data))
	if err != nil {
		t.Fatalf("%v; non-sensitive fixture observations: %q", err, data)
	}
	if observation.owner != "kubernetes" || observation.os != "ubuntu" || observation.memoryKiB != 4000000 {
		t.Fatalf("wrong observations: %+v", observation)
	}
	if strings.Contains(string(data), "synthetic kubeconfig") {
		t.Fatal("credential contents were read or printed")
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("node software/configuration changed")
	}
}

package provision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// addNodesRunner is a stateful fake: once a host has performed a real join,
// subsequent join scripts observe the already-joined marker, which is how the
// replay scenarios are driven across retries.
type addNodesRunner struct {
	joined     map[string]bool
	failPrereq map[string]bool
	failJoin   map[string]bool
	acceptance string // overrides the nodes-acceptance output when set
	tokenOut   string // overrides the join-token output when set
	scripts    map[string][]string
}

func newAddNodesRunner() *addNodesRunner {
	return &addNodesRunner{
		joined:     map[string]bool{},
		failPrereq: map[string]bool{},
		failJoin:   map[string]bool{},
	}
}

func (f *addNodesRunner) Run(_ context.Context, node NodeSpec, _ []byte, script string) (string, error) {
	if f.scripts == nil {
		f.scripts = map[string][]string{}
	}
	f.scripts[node.Host] = append(f.scripts[node.Host], script)
	switch {
	case strings.Contains(script, "__HNB_NODES_READY__"):
		if f.acceptance != "" {
			return f.acceptance, nil
		}
		return "__HNB_NODES_READY__", nil
	case strings.Contains(script, "kubeadm join"):
		if f.failJoin[node.Host] {
			return "", fmt.Errorf("connection refused")
		}
		if f.joined[node.Host] {
			return "__KUBEADM_ALREADY_JOINED__", nil
		}
		f.joined[node.Host] = true
		return "__KUBEADM_JOIN_OK__", nil
	case strings.Contains(script, "kubeadm init"):
		if f.tokenOut != "" {
			return f.tokenOut, nil
		}
		return "__HNB_JOIN_TOKEN__=abcdef.0123456789abcdef\n__HNB_CA_HASH__=sha256:" + strings.Repeat("a", 64), nil
	case strings.Contains(script, "swap must be disabled"):
		if f.failPrereq[node.Host] {
			return "", fmt.Errorf("swap is enabled")
		}
		return "__PREREQ_PRESENT__", nil
	default:
		return "ok", nil
	}
}

func addNodesProvisioner(runner SSHRunner) *Provisioner {
	return New(WithBootstrapper(&NodeBootstrapper{SSH: runner, Key: fakeKeyResolver{}, Store: &fakeStore{}}))
}

func addNodesSpec(hosts ...string) AddNodesSpec {
	nodes := make([]NodeSpec, 0, len(hosts))
	for _, host := range hosts {
		nodes = append(nodes, NodeSpec{ID: host, Host: host, Role: RoleWorker})
	}
	return AddNodesSpec{
		TenantID: "t1", TargetID: "00000000-0000-4000-8000-0000000000aa",
		ControlPlaneEndpoint: "10.0.0.10:6443", DesiredVersion: "v1.31.4",
		IdempotencyKey: "step:key", Nodes: nodes,
		SSHCredential: SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"},
	}
}

func TestAddNodesHappyPath(t *testing.T) {
	runner := newAddNodesRunner()
	res, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1", "worker-2.pool.internal"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Requested != 2 || res.Joined != 2 || res.AlreadyJoined != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Endpoint != "10.0.0.10:6443" {
		t.Fatalf("unexpected endpoint: %q", res.Endpoint)
	}
	// The control plane only minted a join token and observed readiness.
	if got := len(runner.scripts["10.0.0.10"]); got != 2 {
		t.Fatalf("control plane ran %d scripts, want 2: %v", got, runner.scripts["10.0.0.10"])
	}
	// Each worker ran prereq + containerd config + join, in order.
	for _, host := range []string{"worker-1", "worker-2.pool.internal"} {
		if got := len(runner.scripts[host]); got != 3 {
			t.Fatalf("%s ran %d scripts, want 3", host, got)
		}
		join := runner.scripts[host][2]
		if !strings.Contains(join, "--node-name "+host) {
			t.Fatalf("join script for %s does not pin the node name: %s", host, join)
		}
		if !strings.Contains(join, "10.0.0.10:6443") {
			t.Fatalf("join script does not target the recorded endpoint: %s", join)
		}
	}
}

// kubeadm init must never run again: the join-token script carries the
// already-initialized guard ahead of any init invocation.
func TestAddNodesNeverRepeatsInit(t *testing.T) {
	runner := newAddNodesRunner()
	if _, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1")); err != nil {
		t.Fatal(err)
	}
	cpScripts := runner.scripts["10.0.0.10"]
	var initScript string
	for _, script := range cpScripts {
		if strings.Contains(script, "kubeadm init") {
			initScript = script
		}
	}
	if initScript == "" {
		t.Fatal("expected the init script (token-minting branch) on the control plane")
	}
	guard := strings.Index(initScript, "/etc/kubernetes/admin.conf")
	init := strings.Index(initScript, "kubeadm init --config")
	if guard < 0 || init < 0 || guard > init {
		t.Fatalf("already-initialized guard must precede kubeadm init: guard=%d init=%d", guard, init)
	}
	for host, scripts := range runner.scripts {
		if host == "10.0.0.10" {
			continue
		}
		for _, script := range scripts {
			if strings.Contains(script, "kubeadm init --config") {
				t.Fatalf("worker %s was asked to run kubeadm init", host)
			}
		}
	}
}

func TestAddNodesReplaysAlreadyJoinedNodes(t *testing.T) {
	runner := newAddNodesRunner()
	runner.joined["worker-1"] = true
	runner.joined["worker-2"] = true
	res, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1", "worker-2"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Joined != 0 || res.AlreadyJoined != 2 {
		t.Fatalf("expected a pure replay, got %+v", res)
	}
}

func TestAddNodesPrereqFailureThenRetryReplaysJoinedNode(t *testing.T) {
	runner := newAddNodesRunner()
	runner.failPrereq["worker-2"] = true
	res, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1", "worker-2"))
	if err == nil {
		t.Fatal("expected a preparation failure")
	}
	var step *StepError
	if !errors.As(err, &step) || step.Step != "prereq-node:worker-2" {
		t.Fatalf("expected prereq-node:worker-2 step error, got %v", err)
	}
	if res.Joined != 1 {
		t.Fatalf("worker-1 should have joined before the failure, got %+v", res)
	}
	// The operator fixes the node; the same request replays worker-1 as an
	// already-joined node and joins worker-2 exactly once.
	runner.failPrereq = map[string]bool{}
	res, err = addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1", "worker-2"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Joined != 1 || res.AlreadyJoined != 1 {
		t.Fatalf("expected replay of worker-1 and one fresh join, got %+v", res)
	}
}

func TestAddNodesJoinFailureSurfacesStep(t *testing.T) {
	runner := newAddNodesRunner()
	runner.failJoin["worker-1"] = true
	_, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1"))
	var step *StepError
	if !errors.As(err, &step) || step.Step != "join-node:worker-1" {
		t.Fatalf("expected join-node:worker-1 step error, got %v", err)
	}
}

func TestAddNodesAcceptanceRequiresReadyEvidence(t *testing.T) {
	runner := newAddNodesRunner()
	runner.acceptance = "node NotReady"
	_, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1"))
	var step *StepError
	if !errors.As(err, &step) || step.Step != "nodes-acceptance" {
		t.Fatalf("expected nodes-acceptance step error, got %v", err)
	}
}

func TestAddNodesParseFailureWhenTokenMissing(t *testing.T) {
	runner := newAddNodesRunner()
	runner.tokenOut = "ok"
	_, err := addNodesProvisioner(runner).AddNodes(context.Background(), addNodesSpec("worker-1"))
	var step *StepError
	if !errors.As(err, &step) || step.Step != "parse-join" {
		t.Fatalf("expected parse-join step error, got %v", err)
	}
}

func TestAddNodesValidatesSpec(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AddNodesSpec)
		want   string
	}{
		{"no nodes", func(s *AddNodesSpec) { s.Nodes = nil }, "at least one node"},
		{"control-plane role", func(s *AddNodesSpec) { s.Nodes[0].Role = RoleControlPlane }, "worker nodes only"},
		{"name not address", func(s *AddNodesSpec) { s.Nodes[0].Host = "10.0.0.11" }, "must equal its SSH address"},
		{"duplicate", func(s *AddNodesSpec) { s.Nodes = append(s.Nodes, s.Nodes[0]) }, "duplicate node address"},
		{"bad endpoint", func(s *AddNodesSpec) { s.ControlPlaneEndpoint = "10.0.0.10:7443" }, "port 6443"},
		{"bad version", func(s *AddNodesSpec) { s.DesiredVersion = "latest" }, "exact Kubernetes patch version"},
		{"bad name", func(s *AddNodesSpec) { s.Nodes[0].ID = "Worker_1"; s.Nodes[0].Host = "Worker_1" }, "invalid node name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := addNodesSpec("worker-1")
			tc.mutate(&spec)
			runner := newAddNodesRunner()
			_, err := addNodesProvisioner(runner).AddNodes(context.Background(), spec)
			var step *StepError
			if !errors.As(err, &step) || step.Step != "plan" || !strings.Contains(step.Err.Error(), tc.want) {
				t.Fatalf("expected plan error %q, got %v", tc.want, err)
			}
			if len(runner.scripts) != 0 {
				t.Fatalf("invalid spec must run no scripts, ran %v", runner.scripts)
			}
		})
	}
}

func TestAddNodesSSHKeyErrors(t *testing.T) {
	p := New(WithBootstrapper(&NodeBootstrapper{SSH: newAddNodesRunner(), Key: failingKeyResolver{}}))
	_, err := p.AddNodes(context.Background(), addNodesSpec("worker-1"))
	var step *StepError
	if !errors.As(err, &step) || step.Step != "ssh-key" {
		t.Fatalf("expected ssh-key step error, got %v", err)
	}
}

package toolkit

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type networkFixture struct {
	mu      sync.Mutex
	output  string
	scripts []string
	nodes   []string
}

func (r *networkFixture) Run(_ context.Context, node NodeConfig, script string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts = append(r.scripts, script)
	r.nodes = append(r.nodes, node.Name)
	return r.output, nil
}

func TestSSHReachabilityCannotProveAPIPath(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Nodes[0].SSHAddress = "198.51.100.10"
	for _, check := range ProbeNodes(context.Background(), cfg, &fixtureProbe{response: nodeProbeFixture}) {
		if check.Status != StatusPass {
			t.Fatal(check)
		}
	}
	runner := &networkFixture{output: "tcp=unreachable\n"}
	checks := ProbeNodeAPIPaths(context.Background(), cfg, runner)
	if len(checks) != 1 || checks[0].ReasonCode != "NODE_API_TCP_UNREACHABLE" || checks[0].Node != "cp-1" {
		t.Fatalf("SSH success concealed API failure: %+v", checks)
	}
	if !strings.Contains(runner.scripts[0], "/dev/tcp/10.0.0.10/6443") || strings.Contains(runner.scripts[0], "198.51.100.10") {
		t.Fatal("wrong source/destination path checked")
	}
	worker := cfg.Spec.Cluster.Nodes[0]
	worker.Name, worker.SSHAddress, worker.InternalAddress = "worker", "198.51.100.11", "10.0.0.11"
	worker.Roles = []NodeRole{RoleWorker}
	cfg.Spec.Cluster.Nodes = append(cfg.Spec.Cluster.Nodes, worker)
	runner = &networkFixture{output: "tcp=reachable\n"}
	checks = ProbeNodeAPIPaths(context.Background(), cfg, runner)
	if len(checks) != 2 || len(runner.nodes) != 2 || !containsStr(runner.nodes, "worker") {
		t.Fatal("worker source was not observed")
	}
	for _, check := range checks {
		if check.Status != StatusPass || !strings.Contains(check.Actual, "readiness not checked") {
			t.Fatal("TCP was promoted to API readiness")
		}
	}
}

func TestNetworkObservationsFailClosed(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	for output, reason := range map[string]string{"tcp=tool-unavailable\n": "NETWORK_PROBE_TOOL_MISSING", "tcp=reachable\npassword=secret": "NETWORK_OBSERVATION_INVALID"} {
		check := ProbeNodeAPIPaths(context.Background(), cfg, &networkFixture{output: output})[0]
		if check.Status != StatusBlocked || check.ReasonCode != reason || strings.Contains(check.Actual, "secret") {
			t.Fatalf("invalid observation leaked/passed: %+v", check)
		}
	}
	cfg.Spec.Cluster.APIEndpoint = "10.0.0.10:6443;reboot"
	runner := &networkFixture{}
	checks := ProbeNodeAPIPaths(context.Background(), cfg, runner)
	if len(runner.scripts) != 0 || checks[0].Status != StatusBlocked {
		t.Fatal("invalid destination contacted a node")
	}
}

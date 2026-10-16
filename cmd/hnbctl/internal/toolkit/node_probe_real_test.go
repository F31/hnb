package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// This independent method-level test never initializes nodes or uses a synthetic
// release lock to claim platform delivery. Existing owner markers MUST block.
func TestNodeProbeRealSSH(t *testing.T) {
	raw := os.Getenv("HNB_NODE_PROBE_E2E_SPEC")
	if raw == "" {
		t.Skip("set HNB_NODE_PROBE_E2E_SPEC for the explicit read-only SSH test")
	}
	var node NodeConfig
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&node); err != nil {
		t.Fatal(err)
	}
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Nodes = []NodeConfig{node}
	cfg.Spec.Cluster.APIEndpoint = node.InternalAddress + ":6443"
	if problems := cfg.Validate(CurrentEvidence); len(problems) > 0 {
		t.Fatal(problems)
	}
	if problems := cfg.ValidateCredentialFiles(); len(problems) > 0 {
		t.Fatal(problems)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	runner := SSHNodeProbe{}
	const snapshotScript = `set -eu
export LC_ALL=C
for file in /etc/fstab /etc/sysctl.conf /etc/containerd/config.toml /etc/containerd/hnb-bootstrap.toml /var/lib/kubelet/config.yaml; do
  if [ -f "$file" ]; then sha256sum "$file"; else printf '%s absent\n' "$file"; fi
done
for path in /etc/kubernetes/admin.conf /etc/kubernetes/kubelet.conf /etc/kubernetes/manifests /etc/hnb/bootstrap /var/lib/hnb /etc/rancher/rke2 /etc/rancher/k3s; do
  if [ -e "$path" ]; then stat -c '%n %a %s %Y' "$path"; else printf '%s absent\n' "$path"; fi
done
if command -v rpm >/dev/null 2>&1; then rpm -qa | sort | sha256sum; elif command -v dpkg-query >/dev/null 2>&1; then dpkg-query -W | sort | sha256sum; else printf 'package inventory unavailable\n'; fi
`
	before, err := runner.Run(ctx, node, snapshotScript)
	if err != nil {
		t.Fatal("before snapshot:", err)
	}
	checks := ProbeNodes(ctx, cfg, runner)
	networkChecks := ProbeNodeAPIPaths(ctx, cfg, runner)
	after, err := runner.Run(ctx, node, snapshotScript)
	if err != nil {
		t.Fatal("after snapshot:", err)
	}
	if before != after {
		t.Fatal("node configuration/package state changed during the read-only probe")
	}
	if len(checks) != 9 {
		t.Fatalf("incomplete real observation: %+v", checks)
	}
	ownerBlocked := false
	for _, check := range checks {
		t.Logf("%s status=%s actual=%s reason=%s", check.CheckID, check.Status, check.Actual, check.ReasonCode)
		if check.CheckID == "node.owner-markers" && check.Status == StatusBlocked {
			ownerBlocked = true
		}
		if check.CheckID == "node.os-kernel" && check.Status != StatusPass {
			t.Fatal("real OS/kernel is not an evidenced candidate")
		}
	}
	if !ownerBlocked {
		t.Fatal("expected this existing cluster node to refuse bootstrap")
	}
	if len(networkChecks) != 1 {
		t.Fatal("missing real API source-path observation")
	}
	for _, check := range networkChecks {
		t.Logf("%s source=%s expected=%s status=%s actual=%s reason=%s", check.CheckID, check.Node, check.Expected, check.Status, check.Actual, check.ReasonCode)
		if check.Status != StatusPass {
			t.Fatal("existing Ubuntu node API TCP path is unavailable; this is not overridden by SSH success")
		}
	}
	t.Logf("configuration/package snapshot before=after sha256:%x", sha256.Sum256([]byte(before)))
	t.Logf("fixed probe script sha256:%x", sha256.Sum256([]byte(NodeProbeScript)))
}

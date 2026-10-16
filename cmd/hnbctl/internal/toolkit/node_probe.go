package toolkit

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/F31/hnb/pkg/provision"
)

// NodeProbeScript is a fixed read-only observation program. No caller-provided
// command, path or version is interpolated into it. ROOT is changed only by the
// local integration test to observe an isolated filesystem fixture.
const NodeProbeScript = `set -eu
export LC_ALL=C
ROOT=/
os_value() { awk -F= -v key="$1" '$1 == key {gsub(/"/, "", $2); print $2}' "${ROOT%/}/etc/os-release"; }
printf 'os=%s\n' "$(os_value ID)"
printf 'osVersion=%s\n' "$(os_value VERSION_ID)"
printf 'kernel=%s\n' "$(uname -r)"
printf 'arch=%s\n' "$(uname -m)"
printf 'cgroup=%s\n' "$(stat -fc %T "${ROOT%/}/sys/fs/cgroup" | awk '{print $1}')"
printf 'uid=%s\n' "$(id -u)"
printf 'cpu=%s\n' "$(getconf _NPROCESSORS_ONLN)"
printf 'memoryKiB=%s\n' "$(awk '$1 == "MemTotal:" {print $2}' "${ROOT%/}/proc/meminfo")"
printf 'diskKiB=%s\n' "$(df -Pk "$ROOT" | awk 'NR == 2 {print $4}')"
printf 'swapEntries=%s\n' "$(awk 'END {print NR-1}' "${ROOT%/}/proc/swaps")"
owner=none
if [ -e "${ROOT%/}/etc/kubernetes/admin.conf" ] || [ -e "${ROOT%/}/etc/kubernetes/kubelet.conf" ] || [ -e "${ROOT%/}/var/lib/kubelet/config.yaml" ] || [ -d "${ROOT%/}/etc/kubernetes/manifests" ]; then owner=kubernetes; fi
if [ -d "${ROOT%/}/etc/rancher/rke2" ] || [ -d "${ROOT%/}/var/lib/rancher/rke2" ]; then owner=rke2; fi
if [ -d "${ROOT%/}/etc/rancher/k3s" ] || [ -d "${ROOT%/}/var/lib/rancher/k3s" ]; then owner=k3s; fi
if [ -d "${ROOT%/}/etc/hnb/bootstrap" ] || [ -d "${ROOT%/}/var/lib/hnb" ]; then owner=hnb; fi
printf 'owner=%s\n' "$owner"
`

type NodeProbeRunner interface {
	Run(context.Context, NodeConfig, string) (string, error)
}

// NodeStreamReader extends node script execution with a one-way binary stream:
// a fixed script runs via `sh -c` while data (an image archive) arrives on the
// node's stdin. It is required only by authorized bootstrap preload; plain
// observation and stage execution keep working without it.
type NodeStreamReader interface {
	NodeProbeRunner
	Stream(context.Context, NodeConfig, string, io.Reader) (string, error)
}

// SSHNodeProbe uses the operator's existing known_hosts trust. It neither adds
// host keys nor modifies remote policy. Non-root users need existing sudo rights.
type SSHNodeProbe struct {
	Password     string
	SudoPassword string
}

func (p SSHNodeProbe) Run(ctx context.Context, node NodeConfig, script string) (string, error) {
	if node.SSHAuth == "password" || p.SudoPassword != "" {
		return p.authenticatedRun(ctx, node, script, nil, true)
	}
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no",
		"-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none", "-o", "ConnectTimeout=10",
		"-o", "LogLevel=ERROR", "-i", node.SSHKeyFile, "--", node.User+"@"+node.SSHAddress,
		`if [ "$(id -u)" -eq 0 ]; then sh -s; else sudo -n -- sh -s; fi`)
	cmd.Stdin = strings.NewReader(script)
	// A hostile/unexpected remote response cannot produce an unbounded report.
	output := &probeOutput{}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("SSH/privilege observation failed")
	}
	return output.String(), nil
}

// Stream pipes data into a fixed node-side program. The command is built from
// the fixed script only; the data stream never touches the command line.
func (p SSHNodeProbe) Stream(ctx context.Context, node NodeConfig, script string, data io.Reader) (string, error) {
	if node.SSHAuth == "password" || p.SudoPassword != "" {
		return p.authenticatedRun(ctx, node, script, data, true)
	}
	command := provision.ShQuote(script)
	remote := `if [ "$(id -u)" -eq 0 ]; then sh -c ` + command + `; else sudo -n -- sh -c ` + command + `; fi`
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no",
		"-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none", "-o", "ConnectTimeout=10",
		"-o", "LogLevel=ERROR", "-i", node.SSHKeyFile, "--", node.User+"@"+node.SSHAddress,
		remote)
	cmd.Stdin = data
	output := &probeOutput{}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("SSH/privilege stream failed")
	}
	return output.String(), nil
}

type probeOutput struct{ strings.Builder }

func (b *probeOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 16*1024 {
		return 0, fmt.Errorf("probe response exceeds limit")
	}
	return b.Builder.Write(data)
}

type nodeObservation struct {
	os, osVersion, kernel, arch, cgroup, owner string
	uid, cpu, memoryKiB, diskKiB, swapEntries  int64
}

func parseNodeObservation(raw string) (nodeObservation, error) {
	var result nodeObservation
	fields := map[string]*string{"os": &result.os, "osVersion": &result.osVersion, "kernel": &result.kernel, "arch": &result.arch, "cgroup": &result.cgroup, "owner": &result.owner}
	numbers := map[string]*int64{"uid": &result.uid, "cpu": &result.cpu, "memoryKiB": &result.memoryKiB, "diskKiB": &result.diskKiB, "swapEntries": &result.swapEntries}
	seen := map[string]bool{}
	if len(raw) > 16*1024 {
		return result, fmt.Errorf("probe response exceeds limit")
	}
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || seen[key] || value == "" {
			return result, fmt.Errorf("invalid probe response")
		}
		seen[key] = true
		if field, ok := fields[key]; ok {
			// Only bounded observation tokens are accepted, never arbitrary logs or credentials.
			if len(value) > 128 || strings.ContainsAny(value, " \t\r\x00=:") || (key != "cgroup" && strings.Contains(value, "/")) {
				return result, fmt.Errorf("invalid observation token")
			}
			*field = value
		} else if field, ok := numbers[key]; ok {
			value, err := strconv.ParseInt(value, 10, 64)
			if err != nil || value < 0 {
				return result, fmt.Errorf("invalid numeric observation")
			}
			*field = value
		} else {
			return result, fmt.Errorf("unknown probe field")
		}
	}
	if len(seen) != len(fields)+len(numbers) {
		return result, fmt.Errorf("incomplete probe response")
	}
	return result, nil
}

// ProbeNodes gathers candidate node prerequisites. A pass here is not complete
// bootstrap ownership proof, network readiness, or full platform certification.
func ProbeNodes(ctx context.Context, cfg *DeploymentConfig, runner NodeProbeRunner) []PreflightCheck {
	return probeNodes(ctx, cfg, nil, runner)
}

// ProbeLockedNodes also binds observed OS/architecture to the selected release
// recipe; a method-level candidate must not silently use another lock's recipe.
func ProbeLockedNodes(ctx context.Context, cfg *DeploymentConfig, lock *ReleaseLock, runner NodeProbeRunner) []PreflightCheck {
	if lock == nil {
		return []PreflightCheck{{CheckID: "node.lock", Scope: "node", ObservedAt: time.Now().UTC(), Status: StatusBlocked, Expected: "release recipe", Actual: "missing release lock", ReasonCode: "NODE_RECIPE_MISSING", Remediation: "Provide a validated release lock before probing an installation."}}
	}
	return probeNodes(ctx, cfg, lock, runner)
}

func probeNodes(ctx context.Context, cfg *DeploymentConfig, lock *ReleaseLock, runner NodeProbeRunner) []PreflightCheck {
	if problems := cfg.Validate(CurrentEvidence); len(problems) > 0 {
		return []PreflightCheck{{CheckID: "config.structure", Scope: "node", ObservedAt: time.Now().UTC(), Status: StatusBlocked, Expected: "valid configuration before SSH", Actual: "invalid configuration", ReasonCode: "CONFIG_INVALID", Remediation: "Run validate and correct the input before probing."}}
	}
	nodes := cfg.Spec.Cluster.Nodes
	multi := len(nodes) > 1
	results := parallelNodeProbes(nodes, func(node NodeConfig) []PreflightCheck {
		nodeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return probeOneNode(nodeCtx, node, lock, runner, multi)
	})
	var checks []PreflightCheck
	for _, result := range results {
		checks = append(checks, result...)
	}
	return checks
}

// probeOneNode gathers the fixed-schema prerequisites for one node. A pass here
// is not complete bootstrap ownership proof, network readiness, or certification.
func probeOneNode(ctx context.Context, node NodeConfig, lock *ReleaseLock, runner NodeProbeRunner, multi bool) []PreflightCheck {
	var checks []PreflightCheck
	raw, err := runner.Run(ctx, node, NodeProbeScript)
	observedAt := time.Now().UTC()
	add := func(id, expected, actual string, pass bool, reason, remediation string) {
		status := StatusBlocked
		if pass {
			status = StatusPass
			reason = "CHECK_PASSED"
			remediation = ""
		}
		checks = append(checks, PreflightCheck{CheckID: id, Scope: "node", Node: node.Name, ObservedAt: observedAt, Expected: expected, Actual: actual, Status: status, ReasonCode: reason, Remediation: remediation})
	}
	if err != nil {
		add("node.ssh-permission", "trusted SSH host key and root/passwordless sudo", "observation unavailable", false, "NODE_PROBE_UNAVAILABLE", "Check known_hosts, SSH authentication and existing sudo rights; preflight does not modify them.")
		return checks
	}
	obs, err := parseNodeObservation(raw)
	if err != nil {
		add("node.observation", "complete fixed-schema observation", "invalid or incomplete response", false, "NODE_OBSERVATION_INVALID", "Inspect the node's read-only probe prerequisites; remote logs are not included in this report.")
		return checks
	}
	osKey := obs.os + "-" + obs.osVersion
	if lock != nil {
		arch := obs.arch
		if arch == "x86_64" {
			arch = "amd64"
		}
		if arch == "aarch64" {
			arch = "arm64"
		}
		add("node.lock-os", "OS declared by the selected release recipe", osKey, containsStr(lock.Compatibility.OS, osKey), "NODE_OS_RECIPE_MISMATCH", "Select independently verified materials matching this node OS; do not reuse another OS recipe.")
		add("node.lock-arch", "architecture declared by the selected release recipe", arch, containsStr(lock.Compatibility.Arch, arch), "NODE_ARCH_RECIPE_MISMATCH", "Select verified materials matching this node architecture.")
	}
	arch := obs.arch
	if arch == "x86_64" {
		arch = "amd64"
	}
	osMatch := CurrentEvidence.ObservedOSKernelSupported(osKey, obs.kernel, arch)
	add("node.os-kernel", "observed OS/kernel/architecture in the route-scoped compatibility matrix", osKey+"/"+obs.kernel, osMatch, "NODE_OS_KERNEL_UNSUPPORTED", "Use an observed OS/kernel combination; OS recognition alone does not certify bootstrap or platform delivery.")
	add("node.arch", "amd64", obs.arch, obs.arch == "x86_64", "NODE_ARCH_UNSUPPORTED", "Use the evidenced amd64 architecture.")
	add("node.cgroup", "cgroup v2", obs.cgroup, obs.cgroup == "cgroup2fs", "NODE_CGROUP_UNSUPPORTED", "Prepare the OS cgroup configuration explicitly before retrying.")
	add("node.permission", "uid=0 via root or existing sudo", strconv.FormatInt(obs.uid, 10), obs.uid == 0, "NODE_PRIVILEGE_MISSING", "Provide existing passwordless sudo rights or a root account.")
	add("node.cpu", "at least 2 CPUs (node prerequisite, not platform capacity)", strconv.FormatInt(obs.cpu, 10), obs.cpu >= 2, "NODE_CPU_INSUFFICIENT", "Increase node resources; full platform capacity is a separate gate.")
	add("node.memory", "at least 3 GiB (node prerequisite, not platform capacity)", fmt.Sprintf("%d KiB", obs.memoryKiB), obs.memoryKiB >= 3*1024*1024, "NODE_MEMORY_INSUFFICIENT", "Increase node memory; full platform capacity is a separate gate.")
	add("node.disk", "at least 10 GiB available on root filesystem", fmt.Sprintf("%d KiB", obs.diskKiB), obs.diskKiB >= 10*1024*1024, "NODE_DISK_INSUFFICIENT", "Provide disk/cache/storage capacity; runtime and platform mount budgets still require checks.")
	add("node.swap", "no active swap", strconv.FormatInt(obs.swapEntries, 10), obs.swapEntries == 0, "NODE_SWAP_ACTIVE", "Review swap policy explicitly; preflight will not disable it.")
	add("node.owner-markers", "no known Kubernetes/HNB/RKE2/K3s markers", obs.owner, obs.owner == "none", "NODE_OWNER_PRESENT", "Use the owning lifecycle control plane; do not bootstrap or reset this node.")
	if multi && osKey != "rocky-10.1" {
		add("node.topology", "Rocky 10.1 control-plane+worker candidate", osKey, false, "NODE_TOPOLOGY_UNSUPPORTED", "Ubuntu evidence covers one control-plane only; obtain a separate multi-node report.")
	}
	return checks
}

// maxParallelNodeProbes bounds concurrent read-only node probes so a large
// declaration cannot exhaust the operator host's file descriptors or SSH limits.
const maxParallelNodeProbes = 8

// parallelNodeProbes runs one probe per node concurrently and returns the
// per-node results in declaration order, keeping output deterministic.
func parallelNodeProbes(nodes []NodeConfig, probe func(NodeConfig) []PreflightCheck) [][]PreflightCheck {
	results := make([][]PreflightCheck, len(nodes))
	sem := make(chan struct{}, maxParallelNodeProbes)
	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = probe(nodes[i])
		}(i)
	}
	wg.Wait()
	return results
}

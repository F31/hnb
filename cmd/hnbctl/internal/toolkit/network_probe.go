package toolkit

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func observedCheck(id, scope, expected, actual, reason, remediation string, pass bool) PreflightCheck {
	status := StatusBlocked
	if pass {
		status, reason, remediation = StatusPass, "CHECK_PASSED", ""
	}
	return PreflightCheck{CheckID: id, Scope: scope, Expected: expected, Actual: actual, ReasonCode: reason, Remediation: remediation, Status: status, ObservedAt: time.Now().UTC()}
}

// ProbeNodeAPIPaths observes each declared node -> API path, not the installer's
// path or SSH path. TCP availability alone does not establish API readiness.
func ProbeNodeAPIPaths(ctx context.Context, cfg *DeploymentConfig, runner NodeProbeRunner) []PreflightCheck {
	if len(cfg.Validate(CurrentEvidence)) > 0 || cfg.Spec.Cluster.Strategy != StrategyProvision {
		return []PreflightCheck{observedCheck("network.recipe", "node", "valid provision network recipe", "recipe unavailable", "NETWORK_RECIPE_UNAVAILABLE", "Provide a valid candidate recipe; external cluster paths need their own observed source binding.", false)}
	}
	host, port, _ := net.SplitHostPort(cfg.Spec.Cluster.APIEndpoint)
	// host/port are strictly validated IP/6443 tokens, never shell fragments.
	script := fmt.Sprintf(`set -eu
if ! command -v timeout >/dev/null 2>&1 || ! command -v bash >/dev/null 2>&1; then
  printf 'tcp=tool-unavailable\n'
elif timeout 5 bash -c 'exec 3<>/dev/tcp/%s/%s' 2>/dev/null; then
  printf 'tcp=reachable\n'
else
  printf 'tcp=unreachable\n'
fi
`, host, port)
	var checks []PreflightCheck
	results := parallelNodeProbes(cfg.Spec.Cluster.Nodes, func(node NodeConfig) []PreflightCheck {
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		output, err := runner.Run(probeCtx, node, script)
		actual, reason := "source-path observation unavailable", "NODE_API_PATH_UNAVAILABLE"
		pass := false
		if err == nil {
			switch output {
			case "tcp=reachable\n":
				pass, actual = true, "source node -> API TCP reachable; API readiness not checked"
			case "tcp=unreachable\n":
				actual, reason = "source node -> API TCP unreachable", "NODE_API_TCP_UNREACHABLE"
			case "tcp=tool-unavailable\n":
				actual, reason = "read-only TCP probe tools missing", "NETWORK_PROBE_TOOL_MISSING"
			default:
				reason = "NETWORK_OBSERVATION_INVALID"
			}
		}
		check := observedCheck("node.api-tcp", "node", "source node -> "+cfg.Spec.Cluster.APIEndpoint+" TCP connection", actual, reason, "Check the declared source node's route, firewall and API listener; SSH reachability does not prove this path. Pre-init API absence needs stage-aware validation, not a fabricated pass.", pass)
		check.Node = node.Name
		return []PreflightCheck{check}
	})
	for _, result := range results {
		checks = append(checks, result...)
	}
	return checks
}

func probeDigest(raw string) (string, bool) {
	digest := raw
	if !strings.HasPrefix(digest, "sha256:") {
		digest = "sha256:" + digest
	}
	return digest, digestPattern.MatchString(digest) && digest != placeholderDigest
}

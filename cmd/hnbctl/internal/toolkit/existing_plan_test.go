package toolkit

import (
	"strings"
	"testing"
)

func existingConfigFixture(t *testing.T) *DeploymentConfig {
	t.Helper()
	cfg, err := DecodeConfig([]byte(validExistingConfig))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestExistingOwnershipPreservesExternalOwnerAndCNI(t *testing.T) {
	cfg := existingConfigFixture(t)
	m, problems := cfg.Ownership()
	if len(problems) > 0 {
		t.Fatalf("ownership problems: %v", problems)
	}
	if !m.NoNodeInitAction || !m.FitsBoundary {
		t.Fatalf("boundary not clean: %+v", m)
	}
	// External owner/CNI must be preserved, never claimed.
	for _, want := range []string{"external kubernetes cluster lifecycle owner", "external CNI daemonsets/deployments"} {
		if !containsSubstring(m.PreservedExternal, want) {
			t.Fatalf("external owner not preserved: %+v", m.PreservedExternal)
		}
	}
	// No claimed resource may target the external CNI or the cluster lifecycle.
	for _, claimed := range m.Claimed {
		if strings.Contains(claimed, "cilium") || strings.Contains(claimed, "calico") || strings.Contains(claimed, "node") && !strings.Contains(claimed, "namespace") {
			t.Fatalf("claimed scope touches external owner: %s", claimed)
		}
	}
	// Operator-provided secrets are required-existing, not claimed/deleted.
	for _, secret := range []string{"secret/fixture-tls", "secret/tenant/platform-pg", "secret/tenant/market-nats"} {
		if !containsSubstring(m.RequiredExisting, secret) {
			t.Fatalf("required-existing secret missing: %+v", m.RequiredExisting)
		}
		if containsSubstring(m.Claimed, secret) {
			t.Fatalf("operator secret wrongly claimed for deletion: %s", secret)
		}
	}
}

func TestExistingOwnershipRejectsAnyBootstrapNode(t *testing.T) {
	cfg := existingConfigFixture(t)
	cfg.Spec.Cluster.Nodes = []NodeConfig{{Name: "cp-1", Roles: []NodeRole{RoleControlPlane}, SSHAddress: "10.0.0.10", InternalAddress: "10.0.0.10", User: "ubuntu", SSHKeyFile: "./k"}}
	m, problems := cfg.Ownership()
	if len(problems) == 0 || m.NoNodeInitAction {
		t.Fatal("a declared bootstrap node must break the existing ownership boundary")
	}
}

func TestExistingPlanHasNoNodeInitAndPreservesExternalStage(t *testing.T) {
	cfg := existingConfigFixture(t)
	lock := &ReleaseLock{}
	plan, problems := cfg.PlanExistingDependencies(lock)
	if len(problems) > 0 {
		t.Fatalf("plan problems: %v", problems)
	}
	if plan.RoutingMode != "external-cluster cluster.reader/shadow" {
		t.Fatalf("wrong routing mode: %+v", plan)
	}
	if plan.FailureDomains != 1 || !plan.SingleNodeEntry {
		t.Fatal("plan must not imply HA for a single external cluster")
	}
	// No stage may describe a bootstrap/join/reset/node-init action.
	for _, stage := range plan.Stages {
		combined := strings.ToLower(stage.Description + " " + stage.Creates + " " + strings.Join(stage.Requires, " "))
		for _, forbidden := range []string{"kubeadm init", "kubeadm reset", "join node", "bootstrap node", "install cni", "cilium install", "reset the node", "init node"} {
			if strings.Contains(combined, forbidden) {
				t.Fatalf("stage %q contains a node-init action %q", stage.Phase, forbidden)
			}
		}
	}
	// Preflight/prepare/handoff stages must be read-only on nodes.
	readOnly := map[PhaseName]bool{}
	for _, stage := range plan.Stages {
		if stage.ReadOnlyNode {
			readOnly[stage.Phase] = true
		}
	}
	for _, phase := range []PhaseName{PhasePreflight, PhasePreparing, PhaseHandoff} {
		if !readOnly[phase] {
			t.Fatalf("stage %q must be read-only on the external cluster", phase)
		}
	}
}

func TestExistingPlanRejectsMixedBackingsAndProvisionStrategy(t *testing.T) {
	cfg := existingConfigFixture(t)
	cfg.Spec.Platform.Infrastructure.NATS.Mode = "bundled"
	cfg.Spec.Platform.Infrastructure.NATS.SecretRef = ""
	if _, problems := cfg.PlanExistingDependencies(&ReleaseLock{}); len(problems) == 0 {
		t.Fatal("mixed backing modes must be rejected")
	}

	cfg = existingConfigFixture(t)
	cfg.Spec.Cluster.Strategy = StrategyProvision
	cfg.Spec.Mode = ModeBootstrap
	if _, problems := cfg.PlanExistingDependencies(&ReleaseLock{}); len(problems) == 0 {
		t.Fatal("provision strategy must be rejected by the existing-cluster planner")
	}
}

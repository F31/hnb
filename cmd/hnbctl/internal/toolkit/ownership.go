package toolkit

import (
	"sort"
	"strings"
)

// OwnershipManifest distinguishes what this existing-cluster install creates
// (claimed) from everything it must never touch (preserved external family).
// It is derived from the validated DeploymentConfig, never free-form input.
type OwnershipManifest struct {
	SchemaVersion     string   `yaml:"schemaVersion" json:"schemaVersion"`
	ReleaseID         string   `yaml:"releaseId" json:"releaseId"`
	Namespace         string   `yaml:"namespace" json:"namespace"`
	ReleaseName       string   `yaml:"releaseName" json:"releaseName"`
	Claimed           []string `yaml:"claimed" json:"claimed"`
	RequiredExisting  []string `yaml:"requiredExistingSecrets" json:"requiredExistingSecrets"`
	PreservedExternal []string `yaml:"preservedExternal" json:"preservedExternal"`
	NoNodeInitAction  bool     `yaml:"noNodeInitAction" json:"noNodeInitAction"`
	FitsBoundary      bool     `yaml:"fitsExistingClusterBoundary" json:"fitsExistingClusterBoundary"`
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

// Ownership derives the strict ownership boundary for the existing-cluster
// route. Claimed resources are only the release inside the chosen namespace
// plus chart-created PVCs/RBAC. Everything referenced by the operator (TLS,
// storage class, each plane credential, external backings, image pull, identity
// secrets) is required-existing and preserved, never deleted or replaced.
func (c *DeploymentConfig) Ownership() (*OwnershipManifest, []string) {
	m := &OwnershipManifest{
		SchemaVersion:    "ownership-manifest/v1",
		ReleaseID:        c.Spec.ReleaseLock,
		Namespace:        c.Spec.Platform.Namespace,
		ReleaseName:      "hnb-" + c.Metadata.Name,
		NoNodeInitAction: true,
	}
	var problems []string
	claimed := []string{
		"namespace/" + m.Namespace,
		"release/" + m.ReleaseName + " (" + m.Namespace + ")",
		"serviceaccount/observer+executor (" + m.Namespace + ")",
		"clusterrole/" + m.ReleaseName + "-*-(observer|executor)",
		"clusterrolebinding/" + m.ReleaseName + "-*-(observer|executor)",
		"persistentvolumeclaim/postgresql+jetstream data (" + m.Namespace + ")",
	}
	m.Claimed = uniqueSorted(claimed)

	var existing []string
	existing = append(existing, "secret/"+c.Spec.Platform.TLS.ExistingSecret)
	existing = append(existing, "storageclass/"+c.Spec.Platform.Storage.ClassName)
	if infra := c.Spec.Platform.Infrastructure; infra.PostgreSQL.Mode == "external" && infra.PostgreSQL.SecretRef != "" {
		existing = append(existing, "secret/"+infra.PostgreSQL.SecretRef)
	}
	if infra := c.Spec.Platform.Infrastructure; infra.NATS.Mode == "external" && infra.NATS.SecretRef != "" {
		existing = append(existing, "secret/"+infra.NATS.SecretRef)
	}
	if infra := c.Spec.Platform.Infrastructure; infra.PostgreSQL.Mode == "external" && infra.PostgreSQL.SecretRef != "" {
		// External backings themselves are customer-owned; HNB never reconfigures them.
		m.PreservedExternal = append(m.PreservedExternal,
			"external postgresql backing (never reconfigured or deleted)",
			"external nats/jetstream backing (never reconfigured or deleted)")
	}
	for _, plane := range c.Spec.Platform.Infrastructure.Planes {
		existing = append(existing, "secret/"+plane.PostgreSQLSecretRef, "secret/"+plane.NATSSecretRef)
	}
	m.RequiredExisting = uniqueSorted(existing)

	// The external cluster, its CNI and any existing workload/owner markers are
	// preserved unconditionally. The install performs no node initialization.
	m.PreservedExternal = uniqueSorted(append(m.PreservedExternal,
		"external kubernetes cluster lifecycle owner (kubeconfig is the entry point)",
		"external CNI daemonsets/deployments (never modified or replaced)",
		"external namespaces/pvs/business workloads outside "+m.Namespace,
	))

	if c.Spec.Cluster.Strategy != StrategyExisting {
		problems = append(problems, "ownership manifest requires cluster.strategy=existing")
	}
	if len(c.Spec.Cluster.Nodes) != 0 {
		problems = append(problems, "existing-cluster ownership forbids any declared bootstrap node")
		m.NoNodeInitAction = false
	}
	seen := map[string]bool{}
	for _, item := range m.RequiredExisting {
		if seen[item] {
			problems = append(problems, "duplicate required-existing reference: "+item)
		}
		seen[item] = true
	}
	m.FitsBoundary = len(problems) == 0
	return m, problems
}

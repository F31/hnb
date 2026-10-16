package toolkit

import (
	"strings"
)

// ExistingDependencyStage is one phase of an existing-cluster HNB install.
// Every stage either reads or creates only claimed resources; it never runs a
// node-init action, and it records what its success would depend on.
type ExistingDependencyStage struct {
	Phase        PhaseName `yaml:"phase" json:"phase"`
	Description  string    `yaml:"description" json:"description"`
	Creates      string    `yaml:"creates" json:"creates"`
	Requires     []string  `yaml:"requires" json:"requires"`
	ReadOnlyNode bool      `yaml:"readOnlyNodeAction" json:"readOnlyNodeAction"`
}

// ExistingInstallPlan is the strict planning output for the existing-kubernetes
// route. It is not an execution receipt: no resource has been created until each
// stage actually runs against the target, which belongs to the 4.5/5.x work.
type ExistingInstallPlan struct {
	SchemaVersion string `yaml:"schemaVersion" json:"schemaVersion"`
	Mode          Mode   `yaml:"mode" json:"mode"`
	RoutingMode   string `yaml:"routingMode" json:"routingMode"`
	ReleaseLock   string `yaml:"releaseLock" json:"releaseLock"`

	Namespaces        string `yaml:"namespaces" json:"namespaces"`
	StorageClass      string `yaml:"storageClass" json:"storageClass"`
	PostgreSQLBacking string `yaml:"postgresqlBacking" json:"postgresqlBacking"`
	NATSBacking       string `yaml:"natsBacking" json:"natsBacking"`
	BundledBackings   bool   `yaml:"bundledBackings" json:"bundledBackings"`
	ExternalBackings  bool   `yaml:"externalBackings" json:"externalBackings"`
	FailureDomains    int    `yaml:"failureDomains" json:"failureDomains"`
	SingleNodeEntry   bool   `yaml:"singleNodeEntry" json:"singleNodeEntry"`

	Stages       []ExistingDependencyStage `yaml:"stages" json:"stages"`
	InstallsTo   []string                  `yaml:"installsTo" json:"installsTo"`
	ManifestRefs []string                  `yaml:"manifestRefs" json:"manifestRefs"`
}

// PlanExistingDependencies builds a deterministic, non-destructive step order
// from the validated config and lock. It never performs any cluster/node/API
// call; it only derives the conflicting or required facts.
func (c *DeploymentConfig) PlanExistingDependencies(lock *ReleaseLock) (*ExistingInstallPlan, []string) {
	var problems []string
	if c.Spec.Mode != ModeExisting || c.Spec.Cluster.Strategy != StrategyExisting {
		problems = append(problems, "existing dependency plan requires mode=existing with strategy=existing")
	}
	if lock == nil {
		problems = append(problems, "existing dependency plan requires a validated release lock")
	}
	p := &ExistingInstallPlan{
		SchemaVersion: "existing-install-plan/v1",
		Mode:          c.Spec.Mode,
		RoutingMode:   "external-cluster cluster.reader/shadow",
		ReleaseLock:   c.Spec.ReleaseLock,
		Namespaces:    c.Spec.Platform.Namespace,
		StorageClass:  c.Spec.Platform.Storage.ClassName,
	}
	if p.Namespaces == "" || p.StorageClass == "" {
		problems = append(problems, "existing dependency plan requires platform.namespace and storage.className")
	}
	if infra := c.Spec.Platform.Infrastructure; infra.PostgreSQL.Mode == "bundled" && infra.NATS.Mode == "bundled" {
		p.BundledBackings = true
		p.PostgreSQLBacking, p.NATSBacking = "bundled", "bundled"
	} else if infra.PostgreSQL.Mode == "external" && infra.NATS.Mode == "external" {
		p.ExternalBackings = true
		p.PostgreSQLBacking, p.NATSBacking = strings.ToUpper(infra.PostgreSQL.SecretRef), strings.ToUpper(infra.NATS.SecretRef)
	} else {
		problems = append(problems, "existing install supports bundled-both or external-both backings; mixed backings lack evidence")
	}

	// Availability facts are conservative planning inputs only; they do not
	// substitute for readiness checks or HA certification.
	p.FailureDomains = 1
	p.SingleNodeEntry = true

	p.Stages = []ExistingDependencyStage{
		{Phase: PhaseValidating, Description: "strict config + release-lock compatibility", Creates: "validated DeploymentConfig evidence", Requires: []string{"config validate", "lock validate"}},
		{Phase: PhasePreflight, Description: "existing-cluster API TLS, entry, storage, source probes; preserve external CNI/owner", Requires: []string{"KUBECONFIG valid", "API reachable", "provider/storage/source check sets"}, ReadOnlyNode: true},
		{Phase: PhasePreparing, Description: "download pinned CLI/Chart/images/migrations into verified cache", Requires: []string{"release lock", "configuration source policy"}, ReadOnlyNode: true},
		{Phase: PhaseIdentity, Description: "instance identity, KMS materials, service credentials, admin init (existingSecret or generated)", Requires: []string{"identity plan", "kms material", "admin secret"}, ReadOnlyNode: true},
		{Phase: PhaseInfrastructure, Description: "NAMESPACE + TLS + StorageClass-volume (PVC) for bundled backings, or external backings untouched", Requires: []string{"namespace claimed", "tls secret", "storage class"}, ReadOnlyNode: false},
		{Phase: PhaseDatabaseMigrate, Description: "create/verify owned plane databases/roles and migrate owner schema in repair-safe order", Requires: []string{"PostgreSQL ready", "external or bundled PG credentials", "migration lock"}, ReadOnlyNode: false},
		{Phase: PhasePlatformRelease, Description: "install HNB Helm release into claimed namespace", Requires: []string{"locked images/chart installed from verified cache", "existing secrets bound", "migrations applied"}, ReadOnlyNode: false},
		{Phase: PhasePlatformVerified, Description: "entry + login + PG/JetStream + provider + agent/read path layered gates", Requires: []string{"platform ready", "no node-init action"}, ReadOnlyNode: false},
		{Phase: PhaseHandoff, Description: "record operationId + attach receipts; local receipt is not the Operation store", Requires: []string{"formal registry operation", "observability wiring"}, ReadOnlyNode: true},
	}
	p.ManifestRefs = uniqueSorted([]string{
		"svc/apiserver,tunnel-server", "ingress/<publicBaseURL>", "deploy/" + c.Spec.Platform.Namespace + "/hnb-*",
		"pvc/<namespace>/postgresql-data,jetstream-data", "secret/<namespace>/hnb-*existing*",
	})
	return p, problems
}

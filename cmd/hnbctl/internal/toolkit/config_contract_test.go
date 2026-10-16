package toolkit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInvalidConfigCombinations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*DeploymentConfig)
		reason string
	}{
		{"existing-provision", func(c *DeploymentConfig) { c.Spec.Mode = ModeExisting }, "mode and cluster.strategy conflict"},
		{"managed-without-context", func(c *DeploymentConfig) { c.Spec.Mode = ModeManaged }, "platform context"},
		{"missing-kubernetes", func(c *DeploymentConfig) { c.Spec.Cluster.KubernetesVersion = "" }, "kubernetesVersion is required"},
		{"missing-cni-version", func(c *DeploymentConfig) { c.Spec.Cluster.Network.Version = "" }, "network.version is required"},
		{"missing-endpoint", func(c *DeploymentConfig) { c.Spec.Cluster.APIEndpoint = "" }, "apiEndpoint is required"},
		{"wrong-endpoint", func(c *DeploymentConfig) { c.Spec.Cluster.APIEndpoint = "10.0.0.20:6443" }, "internalAddress"},
		{"missing-internal-ip", func(c *DeploymentConfig) { c.Spec.Cluster.Nodes[0].InternalAddress = "" }, "internalAddress"},
		{"ipv6", func(c *DeploymentConfig) { c.Spec.Cluster.Nodes[0].SSHAddress = "2001:db8::1" }, "IPv6 route lacks evidence"},
		{"loopback", func(c *DeploymentConfig) { c.Spec.Cluster.Nodes[0].SSHAddress = "127.0.0.1" }, "unicast IPv4"},
		{"duplicate-role", func(c *DeploymentConfig) {
			c.Spec.Cluster.Nodes[0].Roles = []NodeRole{RoleControlPlane, RoleControlPlane}
		}, "duplicate role"},
		{"invalid-user", func(c *DeploymentConfig) { c.Spec.Cluster.Nodes[0].User = "root;reboot" }, "SSH account"},
		{"key-content", func(c *DeploymentConfig) { c.Spec.Cluster.Nodes[0].SSHKeyFile = "-----BEGIN PRIVATE KEY-----\nsecret" }, "sshKeyFile"},
		{"existing-key-in-bootstrap", func(c *DeploymentConfig) { c.Spec.Cluster.KubeconfigFile = "./kubeconfig" }, "must not include"},
		{"node-service-overlap", func(c *DeploymentConfig) { c.Spec.Cluster.Network.ServiceCIDR = "10.0.0.0/24" }, "address overlaps"},
		{"missing-storage", func(c *DeploymentConfig) { c.Spec.Platform.Storage.ClassName = "" }, "storage.className"},
		{"missing-tls", func(c *DeploymentConfig) { c.Spec.Platform.TLS.ExistingSecret = "" }, "tls.existingSecret"},
		{"plaintext-entry", func(c *DeploymentConfig) { c.Spec.Platform.PublicBaseURL = "http://entry.internal" }, "HTTPS origin"},
		{"entry-credentials", func(c *DeploymentConfig) { c.Spec.Platform.PublicBaseURL = "https://user:secret@entry.internal" }, "HTTPS origin"},
		{"entry-query", func(c *DeploymentConfig) { c.Spec.Platform.PublicBaseURL = "https://entry.internal?token=secret" }, "HTTPS origin"},
		{"entry-port", func(c *DeploymentConfig) { c.Spec.Platform.PublicBaseURL = "https://entry.internal:65536" }, "invalid port"},
		{"missing-pg-mode", func(c *DeploymentConfig) { c.Spec.Platform.Infrastructure.PostgreSQL.Mode = "" }, "postgresql.mode"},
		{"external-dsn", func(c *DeploymentConfig) {
			c.Spec.Platform.Infrastructure.PostgreSQL = InfraBacking{Mode: "external", SecretRef: "postgres://user:secret@host/db"}
		}, "secretRef"},
		{"bundled-external-ref", func(c *DeploymentConfig) { c.Spec.Platform.Infrastructure.NATS.SecretRef = "external-nats" }, "bundled infrastructure"},
		{"online-bundle", func(c *DeploymentConfig) { c.Spec.Artifacts.BundlePath = "./bundle" }, "only applies to offline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := DecodeConfig([]byte(validConfig))
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(cfg)
			if problems := cfg.Validate(CurrentEvidence); !containsSubstring(problems, tc.reason) {
				t.Fatalf("missing %q: %v", tc.reason, problems)
			}
		})
	}
}

func TestDuplicateNodeIdentitiesAndUnsupportedTopology(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	worker := cfg.Spec.Cluster.Nodes[0]
	worker.Name = "worker"
	worker.Roles = []NodeRole{RoleWorker}
	cfg.Spec.Cluster.Nodes = append(cfg.Spec.Cluster.Nodes, worker)
	problems := cfg.Validate(CurrentEvidence)
	for _, field := range []string{"duplicates sshAddress", "duplicates internalAddress"} {
		if !containsSubstring(problems, field) {
			t.Fatalf("duplicate address accepted: %v", problems)
		}
	}
	cfg.Spec.Cluster.Nodes = append(cfg.Spec.Cluster.Nodes, worker)
	if !containsSubstring(cfg.Validate(CurrentEvidence), "at most") {
		t.Fatal("unverified topology accepted")
	}
}

func TestExistingDoesNotClaimExternalOwnership(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validExistingConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Network.Plugin = "cilium"
	if !containsSubstring(cfg.Validate(CurrentEvidence), "external owner") {
		t.Fatal("existing route accepted network mutation input")
	}
	cfg.Spec.Cluster.Network = NetworkConfig{}
	cfg.Spec.Mode = ModeBootstrap
	if !containsSubstring(cfg.Validate(CurrentEvidence), "mode and cluster.strategy conflict") {
		t.Fatal("bootstrap accepted existing strategy")
	}
}

func TestNormalizeCanonicalAndIdempotent(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Network.PodCIDR = "10.244.8.10/16"
	cfg.Spec.Platform.PublicBaseURL = "https://ENTRY.INTERNAL/"
	base, home := t.TempDir(), t.TempDir()
	if err := cfg.Normalize(base, home); err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.ReleaseLock != filepath.Join(base, "release.lock.yaml") || cfg.Spec.Cluster.Nodes[0].SSHKeyFile != filepath.Join(home, ".ssh/hnb_install") || cfg.Spec.Cluster.Network.PodCIDR != "10.244.0.0/16" || cfg.Spec.Platform.PublicBaseURL != "https://entry.internal" {
		t.Fatalf("unexpected normalization: %+v", cfg)
	}
	copy := *cfg
	copy.Spec.Cluster.Nodes = append([]NodeConfig(nil), cfg.Spec.Cluster.Nodes...)
	if err := cfg.Normalize(base, home); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(copy, *cfg) {
		t.Fatal("normalization is not idempotent")
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatal("normalization wrote files")
	}
}

func TestCredentialFilesReadOnlyAndPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	data := []byte("synthetic credential fixture")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Nodes[0].SSHKeyFile = path
	if problems := cfg.ValidateCredentialFiles(); len(problems) > 0 {
		t.Fatal(problems)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(data) {
		t.Fatal("credential mutated")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := cfg.ValidateCredentialFiles(); len(problems) == 0 || strings.Contains(strings.Join(problems, " "), string(data)) {
		t.Fatal("unsafe file accepted or credential leaked")
	}
	cfg.Spec.Cluster.Nodes[0].SSHKeyFile = dir
	if len(cfg.ValidateCredentialFiles()) == 0 {
		t.Fatal("directory accepted as key")
	}
}

func TestLockCandidateMatrixAndIndependentDeliveryGates(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	lock := ReleaseLock{Compatibility: LockCompatibility{Profiles: []string{"Minimal"}, KubernetesVersions: []string{"1.36.5"}, OS: []string{"ubuntu-24.04", "rocky-10.1"}, Arch: []string{"amd64"}}}
	if problems := lock.ValidateCompatibility(cfg, CurrentEvidence); len(problems) > 0 {
		t.Fatal(problems)
	}
	lock.Compatibility.Arch = []string{"arm64"}
	lock.Compatibility.OS = []string{"unknown-os"}
	lock.Compatibility.KubernetesVersions = []string{"1.35.0"}
	problems := lock.ValidateCompatibility(cfg, CurrentEvidence)
	for _, value := range []string{"architecture", "OS", "Kubernetes"} {
		if !containsSubstring(problems, value) {
			t.Fatalf("unknown matrix passed: %v", problems)
		}
	}
	// Synthetic matrix proves profile certification cannot implicitly certify a route or offline.
	ev := *CurrentEvidence
	ev.Profiles = []EvidenceProfile{{Name: "Minimal", Status: "certified"}}
	cfg.Spec.Artifacts.Mode = ArtifactsOffline
	problems = EvidenceGate(cfg, &ev)
	if !containsSubstring(problems, "route") || !containsSubstring(problems, "offline") {
		t.Fatalf("scope escalation: %v", problems)
	}
}

func TestStrictConfigRejectsInjectedAndDuplicateFields(t *testing.T) {
	for _, input := range []string{
		strings.Replace(validConfig, "sshKeyFile: ~/.ssh/hnb_install", "password: secret", 1),
		strings.Replace(validConfig, "mode: bootstrap", "mode: bootstrap\n  mode: existing", 1),
		validConfig + "\n---\nkind: DeploymentConfig\n",
		strings.Replace(validConfig, "readiness: 10m", "readiness: 10m\n    readiness: 5m", 1),
	} {
		if _, err := DecodeConfig([]byte(input)); err == nil {
			t.Fatal("invalid YAML input accepted")
		}
	}
}

package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfig = `apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: management
spec:
  mode: bootstrap
  profile: Minimal
  releaseLock: ./release.lock.yaml
  cluster:
    strategy: provision
    distribution: kubeadm
    kubernetesVersion: 1.36.5
    apiEndpoint: 10.0.0.10:6443
    nodes:
      - name: cp-1
        roles: [control-plane]
        sshAddress: 10.0.0.10
        internalAddress: 10.0.0.10
        user: ubuntu
        sshKeyFile: ~/.ssh/hnb_install
    network:
      plugin: cilium
      version: 1.20.1
      podCIDR: 10.244.0.0/16
      serviceCIDR: 10.96.0.0/12
      routingMode: native
  platform:
    namespace: hnb-system
    publicBaseURL: https://hnb.example.internal
    tls:
      existingSecret: fixture-tls
    storage:
      className: fixture-storage
    infrastructure:
      postgresql:
        mode: bundled
      nats:
        mode: bundled
      planes:
        - name: platform
          database: hnb_platform
          role: hnb_platform
          postgresqlSecretRef: platform-postgresql
          natsSecretRef: platform-nats
        - name: market
          database: hnb_market
          role: hnb_market
          postgresqlSecretRef: market-postgresql
          natsSecretRef: market-nats
        - name: artifact
          database: hnb_artifact
          role: hnb_artifact
          postgresqlSecretRef: artifact-postgresql
          natsSecretRef: artifact-nats
  artifacts:
    mode: online
  timeouts:
    download: 15m
    readiness: 10m
`

const validExistingConfig = `apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: existing
spec:
  mode: existing
  profile: Minimal
  releaseLock: ./release.lock.yaml
  cluster:
    strategy: existing
    kubeconfigFile: /home/impl/.kube/config
  platform:
    namespace: hnb-system
    publicBaseURL: https://hnb.example.internal
    tls:
      existingSecret: fixture-tls
    storage:
      className: fixture-storage
    infrastructure:
      postgresql:
        mode: external
        secretRef: tenant/external-pg
      nats:
        mode: external
        secretRef: tenant/external-nats
      planes:
        - name: platform
          database: hnb_platform
          role: hnb_platform
          postgresqlSecretRef: tenant/platform-pg
          natsSecretRef: tenant/platform-nats
        - name: market
          database: hnb_market
          role: hnb_market
          postgresqlSecretRef: tenant/market-pg
          natsSecretRef: tenant/market-nats
        - name: artifact
          database: hnb_artifact
          role: hnb_artifact
          postgresqlSecretRef: tenant/artifact-pg
          natsSecretRef: tenant/artifact-nats
  artifacts:
    mode: offline
    bundlePath: ./bundle
  timeouts:
    download: 5m
    readiness: 10m
`

const invalidUnknownField = `apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: management
spec:
  mode: bootstrap
  profile: Minimal
  releaseLock: ./release.lock.yaml
  notARealField: true
`

func TestDecodeConfigStrictRejectsUnknownFields(t *testing.T) {
	if _, err := DecodeConfig([]byte(invalidUnknownField)); err == nil {
		t.Fatal("expected unknown-field rejection")
	} else if !strings.Contains(err.Error(), "notARealField") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsUnknownTimeoutFields(t *testing.T) {
	data := strings.Replace(validConfig, "download: 15m", "download: 15m\n    command: reboot", 1)
	if _, err := DecodeConfig([]byte(data)); err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("nested unknown timeout field accepted: %v", err)
	}
}

func TestEvidenceGateFailsClosed(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Profile = "unknown"
	for _, matrix := range []*EvidenceMatrix{CurrentEvidence, {}, nil} {
		if len(EvidenceGate(cfg, matrix)) == 0 {
			t.Fatal("missing evidence passed")
		}
	}
}

func TestValidConfigPasses(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if problems := cfg.Validate(CurrentEvidence); len(problems) > 0 {
		t.Fatalf("valid config rejected: %v", problems)
	}
}

func TestValidExistingConfigPasses(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validExistingConfig))
	if err != nil {
		t.Fatal(err)
	}
	if problems := cfg.Validate(CurrentEvidence); len(problems) > 0 {
		t.Fatalf("valid existing config rejected: %v", problems)
	}
}

func TestRejectsUncertifiedProfile(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Profile = ProfileStandardHA
	problems := EvidenceGate(cfg, CurrentEvidence)
	if !containsSubstring(problems, "StandardHA") {
		t.Fatalf("expected StandardHA rejection, got: %v", problems)
	}
}

func TestRejectsMultiControlPlane(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Nodes = append(cfg.Spec.Cluster.Nodes, NodeConfig{
		Name: "cp-2", Roles: []NodeRole{RoleControlPlane},
		SSHAddress: "10.0.0.11", User: "ubuntu", SSHKeyFile: "/tmp/key",
	})
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "exactly one control-plane") {
		t.Fatalf("expected multi-control-plane rejection, got: %v", problems)
	}
}

func TestRejectsUncertifiedKubernetesVersion(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.KubernetesVersion = "1.35.0"
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "1.35.0") {
		t.Fatalf("expected uncertified version rejection, got: %v", problems)
	}
}

func TestRejectsOverlappingCIDRs(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Network.ServiceCIDR = "10.244.64.0/18"
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "overlap") {
		t.Fatalf("expected overlap rejection, got: %v", problems)
	}
}

func TestRejectsEmptyPublicBaseURL(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Platform.PublicBaseURL = ""
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "publicBaseURL") {
		t.Fatalf("expected publicBaseURL rejection, got: %v", problems)
	}
}

func TestRejectsManagedProvision(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Mode = ModeManaged
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "not valid for mode=managed") {
		t.Fatalf("expected managed provision rejection, got: %v", problems)
	}
}

func TestRejectsFlannelNetwork(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Network.Plugin = "flannel"
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "flannel") {
		t.Fatalf("expected flannel rejection, got: %v", problems)
	}
}

func containsSubstring(problems []string, needle string) bool {
	for _, p := range problems {
		if strings.Contains(p, needle) {
			return true
		}
	}
	return false
}

func TestReleaseLockRejectsLatestAndPlaceholder(t *testing.T) {
	lock := ReleaseLock{
		SchemaVersion: LockSchemaVersion,
		ReleaseID:     "hnb-0.2.0", ReleaseVersion: "0.2.0",
		Images: []PinnedImage{
			{Name: "apiserver", Ref: "registry.internal/hnb/apiserver:latest", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		},
		Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}},
	}
	problems := lock.Validate()
	if !containsSubstring(problems, "latest") || !containsSubstring(problems, "real digest") {
		t.Fatalf("expected latest+placeholder rejection, got: %v", problems)
	}
}

func TestReleaseLockAcceptsRealDigests(t *testing.T) {
	lock := ReleaseLock{
		SchemaVersion: LockSchemaVersion,
		ReleaseID:     "hnb-0.2.0", ReleaseVersion: "0.2.0",
		Images: []PinnedImage{
			{Name: "apiserver", Ref: "registry.internal/hnb/apiserver:v0.2.0", Digest: "sha256:" + strings.Repeat("a", 64)},
			{Name: "cilium", Ref: "quay.io/cilium/cilium:v1.20.1", Digest: "sha256:" + strings.Repeat("b", 64)},
		},
		Charts: []PinnedChart{
			{Name: "hnb", Version: "0.2.0", Digest: "sha256:" + strings.Repeat("c", 64)},
		},
		NodeArtifacts: []NodeArtifact{
			{OS: "ubuntu-24.04", Arch: "amd64", Kind: "binary", Ref: "kubeadm", Digest: strings.Repeat("d", 64)},
			{OS: "ubuntu-24.04", Arch: "amd64", Kind: "container", Ref: "pause:3.10", Digest: "sha256:" + strings.Repeat("e", 64)},
		},
		Migrations: []Migration{
			{Name: "101_secret_reference_admission_unique", Checksum: strings.Repeat("f", 64)},
		},
		Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}},
	}
	if problems := lock.Validate(); len(problems) > 0 {
		t.Fatalf("real lock rejected: %v", problems)
	}
}

func TestRedactSecrets(t *testing.T) {
	input := "password=hunter2 postgres://u:p@localhost/db PRIVATE KEY BEGIN abc END kubeconfig 127.0.0.1:6443"
	redacted := Redact(input)
	if strings.Contains(redacted, "hunter2") || strings.Contains(redacted, "u:p@localhost") {
		t.Fatalf("secret leaked: %s", redacted)
	}
	if !strings.Contains(redacted, "127.0.0.1:6443") {
		t.Fatalf("non-secret data over-redacted: %s", redacted)
	}
}

func TestDiagnosisBundleRedactMap(t *testing.T) {
	raw := map[string]any{
		"kubeconfig": "client-certificate-data: abc",
		"dsn":        "postgres://admin:s3cret@10.0.0.2/hnb",
		"nodes":      []any{"cp-1"},
	}
	RedactMap(raw)
	if strings.Contains(raw["kubeconfig"].(string), "abc") || strings.Contains(raw["dsn"].(string), "s3cret") {
		t.Fatal("RedactMap leaked secrets")
	}
}

func TestPreflightBlockingCount(t *testing.T) {
	report := PreflightReport{
		RunID: "run-1",
		Checks: []PreflightCheck{
			{Status: StatusPass},
			{Status: StatusBlocked},
			{Status: StatusWarn},
		},
	}
	if report.BlockingCount() != 1 {
		t.Fatalf("expected 1 blocked check, got %d", report.BlockingCount())
	}
}

// TestFixturesRoundTrip parses the checked-in fixture files used by the portal
// and CLI and verifies the expected verdict.
func TestFixturesRoundTrip(t *testing.T) {
	fixtures := map[string]struct {
		path      string
		wantValid bool
	}{
		"bootstrap-minimal":       {"bootstrap-minimal.yaml", true},
		"existing-minimal":        {"existing-minimal.yaml", true},
		"standard-ha-unsupported": {"standard-ha-unsupported.yaml", false},
	}
	for name, fx := range fixtures {
		data, err := os.ReadFile(filepath.Join("testdata", fx.path))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cfg, err := DecodeConfig(data)
		if err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		problems := cfg.Validate(CurrentEvidence)
		if fx.wantValid && len(problems) > 0 {
			t.Fatalf("%s should be valid but got: %v", name, problems)
		}
		if !fx.wantValid && len(problems) == 0 {
			t.Fatalf("%s should be rejected but passed", name)
		}
	}
}

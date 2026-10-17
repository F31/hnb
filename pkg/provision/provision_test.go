package provision

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestSSHOutputConcurrentCopiesPreserveMarkers(t *testing.T) {
	var out synchronizedBuffer
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, _ = io.Copy(&out, strings.NewReader(strings.Repeat("__HNB_JOIN_TOKEN__=abcdef.0123456789abcdef\n", 1000)))
		}()
	}
	workers.Wait()
	if strings.Count(out.String(), "__HNB_JOIN_TOKEN__=abcdef.0123456789abcdef\n") != 8000 {
		t.Fatal("concurrent SSH copies corrupted or dropped bootstrap output")
	}
}

func TestProvisionSpecNodesAndEndpoint(t *testing.T) {
	spec := ProvisionSpec{
		Nodes: []NodeSpec{
			{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane},
			{ID: "w1", Host: "10.0.0.11", Role: RoleWorker},
		},
	}
	if got := len(spec.ControlPlaneNodes()); got != 1 {
		t.Fatalf("ControlPlaneNodes = %d, want 1", got)
	}
	if got := len(spec.WorkerNodes()); got != 1 {
		t.Fatalf("WorkerNodes = %d, want 1", got)
	}
	if ep := spec.Endpoint(); ep != "10.0.0.10:6443" {
		t.Fatalf("Endpoint = %q, want 10.0.0.10:6443", ep)
	}
	// explicit endpoint wins
	spec.ControlPlaneEndpoint = "lb.hnb.local:8443"
	if ep := spec.Endpoint(); ep != "lb.hnb.local:8443" {
		t.Fatalf("Endpoint with override = %q", ep)
	}
}

func TestNodeEffectiveDefaults(t *testing.T) {
	n := NodeSpec{Host: "h", Port: 0, User: ""}
	if n.EffectivePort() != 22 || n.EffectiveUser() != "root" {
		t.Fatalf("defaults not applied: %d/%q", n.EffectivePort(), n.EffectiveUser())
	}
	n2 := NodeSpec{Host: "h", Port: 2222, User: "admin"}
	if n2.EffectivePort() != 2222 || n2.EffectiveUser() != "admin" {
		t.Fatalf("explicit values lost: %d/%q", n2.EffectivePort(), n2.EffectiveUser())
	}
}

func TestControlPlaneConfigRendered(t *testing.T) {
	spec := ProvisionSpec{
		DesiredVersion: "v1.31.4",
		Nodes: []NodeSpec{
			{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane},
			{ID: "w1", Host: "10.0.0.11", Role: RoleWorker},
		},
	}
	cp := spec.ControlPlaneNodes()[0]
	cfg := ControlPlaneConfig(spec, cp)
	for _, want := range []string{
		"kubeadm.k8s.io/v1beta4", "v1.31.4", "10.0.0.10:6443",
		"10.244.0.0/16", "10.96.0.0/12", "cluster.local",
		"advertiseAddress: 10.0.0.10",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config missing %q:\n%s", want, cfg)
		}
	}
	if !strings.Contains(cfg, "- 10.0.0.11") {
		t.Fatalf("worker node IP missing from certSANs:\n%s", cfg)
	}
}

func TestCiliumProfileUsesInternalAPIAndPinnedImages(t *testing.T) {
	spec := ProvisionSpec{
		DesiredVersion: "1.36.5", NetworkPlugin: "cilium", NetworkVersion: "1.20.1",
		ImageRepository: "harbor.hnb.local/hnb/k8s", CNIImageRepository: "harbor.hnb.local/hnb/cilium",
		HelmChartRepository:  "https://harbor.hnb.local/hnb/charts",
		AdvertiseAddress:     "10.9.31.180",
		ControlPlaneEndpoint: "10.9.31.180:6443",
		Nodes:                []NodeSpec{{ID: "cp", Host: "106.75.8.151", Role: RoleControlPlane}},
	}
	if err := validateSSHSpec(spec); err != nil {
		t.Fatal(err)
	}
	cfg := ControlPlaneConfig(spec, spec.Nodes[0])
	if !strings.Contains(cfg, "advertiseAddress: 10.9.31.180") || !strings.Contains(cfg, "imageRepository: harbor.hnb.local/hnb/k8s") || !strings.Contains(cfg, "- 106.75.8.151") {
		t.Fatalf("NAT-aware configuration lost: %s", cfg)
	}
	if !strings.Contains(InitScript(spec, spec.Nodes[0]), "--skip-phases=addon/kube-proxy") {
		t.Fatal("Cilium replacement must not install iptables kube-proxy")
	}
	network := NetworkScript(spec)
	for _, want := range []string{"--version '1.20.1'", "cilium-operator:v1.20.1", "ipam.mode=kubernetes", "autoDirectNodeRoutes=true", "k8sServiceHost='10.9.31.180'"} {
		if !strings.Contains(network, want) {
			t.Fatalf("missing %q in network script", want)
		}
	}
	// Every node image and the chart source must come from the platform
	// repository; no implicit upstream public registry.
	for _, forbidden := range []string{"quay.io", "registry.k8s.io", "helm.cilium.io", "registry.aliyuncs.com", "--repo "} {
		if strings.Contains(network, forbidden) {
			t.Fatalf("network script still reaches %s:\n%s", forbidden, network)
		}
	}
	// Harbor serves charts as OCI artifacts, so the chart must be an explicit
	// oci:// project reference rather than a classic index.yaml repository.
	for _, want := range []string{"harbor.hnb.local/hnb/cilium/cilium:v1.20.1", "harbor.hnb.local/hnb/cilium/cilium-operator:v1.20.1", "'oci://harbor.hnb.local/hnb/charts/cilium'"} {
		if !strings.Contains(network, want) {
			t.Fatalf("missing platform-resolved reference %q", want)
		}
	}
	accept := AcceptanceScript(spec)
	if strings.Contains(accept, "kube-flannel") || !strings.Contains(accept, "daemonset/cilium") || !strings.Contains(accept, "deployment/cilium-operator") {
		t.Fatal("acceptance must observe selected Cilium datapath")
	}
	for _, plugin := range []string{"unknown", "cilium'; exit 0"} {
		spec.NetworkPlugin = plugin
		if validateSSHSpec(spec) == nil {
			t.Fatalf("accepted unsupported plugin %q", plugin)
		}
	}
	spec.NetworkPlugin = "cilium"
	spec.NetworkVersion = "1.20.1'; exit 0"
	if validateSSHSpec(spec) == nil {
		t.Fatal("accepted unpinned/injected Cilium version")
	}
}

func TestParseJoinDetails(t *testing.T) {
	out := `[preflight] Running pre-flight checks
kubeadm join 10.0.0.10:6443 --token abcdef.0123456789abcdef \
	--discovery-token-ca-cert-hash sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
[init] done
__HNB_JOIN_TOKEN__=abcdef.0123456789abcdef
__HNB_CA_HASH__=sha256:0000000000000000000000000000000000000000000000000000000000000000
`
	token, hash, err := ParseJoinDetails(out)
	if err != nil {
		t.Fatal(err)
	}
	if token != "abcdef.0123456789abcdef" {
		t.Fatalf("token = %q", token)
	}
	if hash != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" && hash != "sha256:0000000000000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("hash = %q", hash)
	}
}

func TestParseJoinDetailsMissing(t *testing.T) {
	if _, _, err := ParseJoinDetails("kubeadm init done, nothing else"); err == nil {
		t.Fatal("expected error when join token missing")
	}
}

func TestMinorVersionAndSanitize(t *testing.T) {
	if got := minorVersion("v1.31.4"); got != "1.31" {
		t.Fatalf("minorVersion = %q", got)
	}
	if got := sanitizeNodeName(NodeSpec{ID: "My_CP_1"}); got != "my-cp-1" {
		t.Fatalf("sanitizeNodeName = %q", got)
	}
}

// fakeRunner records scripts per host and returns canned output.
type fakeRunner struct {
	scripts map[string][]string
}

func (f *fakeRunner) Run(_ context.Context, node NodeSpec, key []byte, script string) (string, error) {
	if f.scripts == nil {
		f.scripts = map[string][]string{}
	}
	f.scripts[node.Host] = append(f.scripts[node.Host], script)
	if strings.Contains(script, "__HNB_CLUSTER_ACCEPTED__") {
		return "__HNB_CLUSTER_ACCEPTED__", nil
	}
	role := node.Role
	if strings.Contains(script, "kubeadm init") || strings.Contains(script, "kubernetesVersion") {
		return "__HNB_JOIN_TOKEN__=abcdef.0123456789abcdef\n__HNB_CA_HASH__=sha256:aa" + strings.Repeat("a", 62) + "\n" + adminConf, nil
	}
	if strings.Contains(script, "cat /etc/kubernetes/admin.conf") {
		return adminConf, nil
	}
	_ = role
	return "ok", nil
}

const adminConf = `apiVersion: v1
kind: Config
clusters:
- name: dev
  cluster:
    server: https://10.0.0.10:6443
contexts:
- name: dev
  context:
    cluster: dev
    user: dev
current-context: dev`

type fakeKeyResolver struct{}

func (fakeKeyResolver) ValidateVersion(context.Context, string) error { return nil }

func (fakeKeyResolver) ResolveSSHKey(_ context.Context, _ string, _ SecretReference) ([]byte, error) {
	return []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----\n"), nil
}

type fakeStore struct {
	saved SecretReference
}

func (f *fakeStore) StoreKubeconfig(_ context.Context, _, _, _ string, _ []byte) (SecretReference, error) {
	f.saved = SecretReference{Provider: "local", Scope: "tenant", Name: "kubeconf", Version: "1"}
	return f.saved, nil
}

func TestProvisionHappyPath(t *testing.T) {
	runner := &fakeRunner{}
	store := &fakeStore{}
	p := New(WithBootstrapper(&NodeBootstrapper{
		SSH: runner, Key: fakeKeyResolver{}, Store: store,
	}))
	spec := ProvisionSpec{
		TenantID: "t1", TargetID: "00000000-0000-4000-8000-0000000000aa", DisplayName: "prod",
		DesiredVersion: "v1.31.4",
		Nodes: []NodeSpec{
			{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane},
			{ID: "w1", Host: "10.0.0.11", Role: RoleWorker},
		},
		SSHCredential: SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"},
	}
	res, err := p.Provision(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if res.NodeCount != 2 || res.ControlPlaneEndpoint != "10.0.0.10:6443" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if store.saved.Name == "" {
		t.Fatal("kubeconfig was not stored")
	}
}

func TestProvisionRequiresControlPlane(t *testing.T) {
	p := New(WithBootstrapper(&NodeBootstrapper{SSH: &fakeRunner{}, Key: fakeKeyResolver{}}))
	_, err := p.Provision(context.Background(), ProvisionSpec{Nodes: []NodeSpec{{ID: "w", Host: "h", Role: RoleWorker}}})
	if err == nil {
		t.Fatal("expected error with no control-plane node")
	}
}

func TestProvisionSSHKeyError(t *testing.T) {
	p := New(WithBootstrapper(&NodeBootstrapper{
		SSH:   &fakeRunner{},
		Key:   failingKeyResolver{},
		Store: &fakeStore{},
	}))
	_, err := p.Provision(context.Background(), ProvisionSpec{
		DesiredVersion: "v1.31.4",
		Nodes:          []NodeSpec{{ID: "cp", Host: "10.0.0.10", Role: RoleControlPlane}},
	})
	if err == nil || !strings.Contains(err.Error(), "ssh-key") {
		t.Fatalf("expected ssh-key step error, got %v", err)
	}
}

type failingKeyResolver struct{}

func (failingKeyResolver) ValidateVersion(context.Context, string) error { return nil }

func (failingKeyResolver) ResolveSSHKey(context.Context, string, SecretReference) ([]byte, error) {
	return nil, errors.New("no such secret")
}

// Bootstrap must never fall back to an implicit upstream registry: the platform
// repository is the only node image source, which is what makes air-gapped
// installs (tasks 7.2/7.3) possible.
func TestCiliumBootstrapRefusesImplicitPublicRegistries(t *testing.T) {
	base := ProvisionSpec{
		DesiredVersion: "1.36.5", NetworkPlugin: "cilium", NetworkVersion: "1.20.1",
		Nodes: []NodeSpec{{ID: "cp", Host: "10.9.31.180", Role: RoleControlPlane}},
	}
	// No image repository and no chart repository.
	if err := validateSSHSpec(base); err == nil || !strings.Contains(err.Error(), "CNI image repository") {
		t.Fatalf("missing image repository accepted: %v", err)
	}
	noChart := base
	noChart.ImageRepository = "harbor.hnb.local/hnb/k8s"
	if err := validateSSHSpec(noChart); err == nil || !strings.Contains(err.Error(), "Helm chart repository") {
		t.Fatalf("missing chart repository accepted: %v", err)
	}
	// A malformed chart origin is rejected; an explicit internal HTTP registry is
	// allowed because air-gapped platform registries commonly serve plain HTTP.
	malformed := noChart
	malformed.HelmChartRepository = "harbor.hnb.local/hnb/charts"
	if err := validateSSHSpec(malformed); err == nil {
		t.Fatal("chart repository without a scheme accepted")
	}
	internal := noChart
	internal.HelmChartRepository = "http://harbor.hnb.internal/hnb/charts"
	if err := validateSSHSpec(internal); err != nil {
		t.Fatalf("explicit internal chart repository rejected: %v", err)
	}
	// Flannel stays independent of the Cilium-only gates.
	flannel := base
	flannel.NetworkPlugin = "flannel"
	flannel.NetworkVersion = ""
	if err := validateSSHSpec(flannel); err != nil {
		t.Fatalf("flannel wrongly gated on Cilium repositories: %v", err)
	}
}

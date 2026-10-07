package provider

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provision"
)

func fakeK8sClient(kubeconfig []byte) (kubernetes.Interface, error) {
	return fake.NewSimpleClientset(), nil
}

func TestKubernetesManagerCreateProvisionNamespace(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	manager := NewKubernetesManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.kubernetes.provision-and-register", "create", 1)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "create", 1)
	input.Parameters = map[string]any{"replicas": float64(3), "containerNetwork": "Kube-OVN", "podCidr": "10.244.0.0/16"}

	result, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if result.Outputs["action"] != "create" {
		t.Fatalf("action = %q", result.Outputs["action"])
	}
	if result.Outputs["managed"] != "true" {
		t.Fatalf("managed = %q", result.Outputs["managed"])
	}
	if result.Outputs["parameters"] == "" {
		t.Fatalf("parameters output missing: %+v", result.Outputs)
	}
	client := manager.client
	ns, err := client.CoreV1().Namespaces().Get(context.Background(), "hnb-target-515eba09", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	ann := ns.GetAnnotations()
	if ann["hnb.io/parameter.containerNetwork"] != "Kube-OVN" || ann["hnb.io/parameter.podCidr"] != "10.244.0.0/16" || ann["hnb.io/parameter.replicas"] != "3" {
		t.Fatalf("parameter annotations = %+v", ann)
	}
}

func TestKubernetesManagerImportValidateConnectivity(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	manager := NewKubernetesManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.kubernetes.register", "import", 2)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "import", 2)

	result, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Outputs["action"] != "import" {
		t.Fatalf("action = %q", result.Outputs["action"])
	}
}

func TestKubernetesManagerCreateProvisionOverSSH(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	fakeProvisioner := &fakeProvisionerDelegator{}
	manager := NewKubernetesManagerWithProvisioner(profile, fakeProvisioner)
	execution := testContext(profile, "runtime_target.kubernetes.provision-and-register", "create", 1)
	input := testInput(profile, "create", 1)
	input.SSHSecretRef = &SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"}
	input.Parameters = map[string]any{
		"nodes": []any{
			map[string]any{"id": "cp1", "host": "10.0.0.10", "role": "control-plane"},
			map[string]any{"id": "w1", "host": "10.0.0.11", "role": "worker"},
		},
		"podCidr":     "10.244.0.0/16",
		"serviceCidr": "10.96.0.0/12",
	}
	fakeProvisioner.result = provision.Result{
		ControlPlaneEndpoint: "10.0.0.10:6443",
		NodeCount:            2,
		KubeconfigRef:        provision.SecretReference{Provider: "local", Scope: "tenant", Name: "prov-kubeconfig", Version: "1"},
	}

	result, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("provision create: %v", err)
	}
	if result.Outputs["provisioned"] != "true" {
		t.Fatalf("provisioned output = %q", result.Outputs["provisioned"])
	}
	if result.Outputs["nodeCount"] != "2" {
		t.Fatalf("nodeCount output = %q", result.Outputs["nodeCount"])
	}
	if result.Outputs["endpoint"] != "10.0.0.10:6443" {
		t.Fatalf("endpoint output = %q", result.Outputs["endpoint"])
	}
	if fakeProvisioner.spec.TenantID != execution.TenantID || len(fakeProvisioner.spec.Nodes) != 2 {
		t.Fatalf("provisioner spec not translated: %+v", fakeProvisioner.spec)
	}
	if fakeProvisioner.spec.SSHCredential.Provider != "local" || fakeProvisioner.spec.SSHCredential.Name != "ssh-key" {
		t.Fatalf("ssh credential not carried: %+v", fakeProvisioner.spec.SSHCredential)
	}
}

type fakeProvisionerDelegator struct {
	spec   provision.ProvisionSpec
	result provision.Result
}

func (f *fakeProvisionerDelegator) Provision(_ context.Context, spec provision.ProvisionSpec) (provision.Result, error) {
	f.spec = spec
	return f.result, nil
}

func TestParseNodesValidation(t *testing.T) {
	if _, err := parseNodes(map[string]any{"nodes": "nope"}); err == nil {
		t.Fatal("expected error for non-array nodes")
	}
	if _, err := parseNodes(map[string]any{"nodes": []any{map[string]any{"host": "h", "role": "bogus"}}}); err == nil {
		t.Fatal("expected error for invalid role")
	}
	if _, err := parseNodes(map[string]any{"nodes": []any{map[string]any{"role": "worker"}}}); err == nil {
		t.Fatal("expected error for missing host")
	}
	nodes, err := parseNodes(map[string]any{"nodes": []any{
		map[string]any{"host": "10.0.0.1", "role": "control-plane", "port": float64(2222)},
		map[string]any{"host": "10.0.0.2", "role": "worker"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Port != 2222 {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

func TestKubernetesManagerReplayAndFencing(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	manager := NewKubernetesManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.kubernetes.register", "import", 2)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "import", 2)

	first, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	replay, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Checkpoint != first.Checkpoint {
		t.Fatalf("replay checkpoint mismatch: %s vs %s", replay.Checkpoint, first.Checkpoint)
	}

	stale := testContext(profile, "runtime_target.kubernetes.register", "import", 1)
	stale.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	staleInput := testInput(profile, "import", 1)
	_, err = manager.Apply(context.Background(), stale, staleInput)
	if err == nil {
		t.Fatal("expected fencing error for stale generation")
	}
}

func TestKubernetesManagerRestoresManagementFromNamespaceAnnotations(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	client := fake.NewSimpleClientset()
	clientFunc := func(kubeconfig []byte) (kubernetes.Interface, error) { return client, nil }
	firstManager := NewKubernetesManagerWithClient(profile, clientFunc)
	createExecution := testContext(profile, "runtime_target.kubernetes.provision-and-register", "create", 1)
	createExecution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	createInput := testInput(profile, "create", 1)
	if _, err := firstManager.Apply(context.Background(), createExecution, createInput); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Simulate provider restart: the second manager has an empty in-memory map
	// and must recover the management relation from namespace annotations.
	secondManager := NewKubernetesManagerWithClient(profile, clientFunc)
	unmanageExecution := testContext(profile, "runtime_target.kubernetes.unregister", "unmanage", 2)
	unmanageExecution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	unmanageInput := testInput(profile, "unmanage", 2)
	if _, err := secondManager.Apply(context.Background(), unmanageExecution, unmanageInput); err != nil {
		t.Fatalf("unmanage after restart: %v", err)
	}
}

func TestKubernetesManagerHonorsCancellation(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	manager := NewKubernetesManagerWithClient(profile, fakeK8sClient)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond)

	execution := testContext(profile, "runtime_target.kubernetes.upgrade", "upgrade", 3)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "upgrade", 3)
	_, err := manager.Apply(ctx, execution, input)
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestKubernetesManagerUnmanageRequiresExistingManagement(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	manager := NewKubernetesManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.kubernetes.unregister", "unmanage", 4)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "unmanage", 4)
	_, err := manager.Apply(context.Background(), execution, input)
	if err == nil {
		t.Fatal("expected error for unmanage on unmanaged target")
	}
}

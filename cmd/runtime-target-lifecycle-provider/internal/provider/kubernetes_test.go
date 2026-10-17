package provider

import (
	"context"
	"errors"
	"strings"
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
		"podCidr":              "10.244.0.0/16",
		"serviceCidr":          "10.96.0.0/12",
		"networkPlugin":        "cilium",
		"networkVersion":       "1.20.1",
		"advertiseAddress":     "10.0.0.10",
		"imageRepository":      "registry.aliyuncs.com/google_containers",
		"controlPlaneEndpoint": "10.0.0.10:6443",
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
	if fakeProvisioner.spec.NetworkPlugin != "cilium" || fakeProvisioner.spec.NetworkVersion != "1.20.1" || fakeProvisioner.spec.AdvertiseAddress != "10.0.0.10" || fakeProvisioner.spec.ImageRepository != "registry.aliyuncs.com/google_containers" || fakeProvisioner.spec.ControlPlaneEndpoint != "10.0.0.10:6443" {
		t.Fatalf("environment parameters not forwarded: %+v", fakeProvisioner.spec)
	}
}

type fakeProvisionerDelegator struct {
	spec   provision.ProvisionSpec
	result provision.Result

	addSpec   provision.AddNodesSpec
	addResult provision.AddNodesResult
	addErr    error
}

func (f *fakeProvisionerDelegator) Provision(_ context.Context, spec provision.ProvisionSpec) (provision.Result, error) {
	f.spec = spec
	return f.result, nil
}

func (f *fakeProvisionerDelegator) AddNodes(_ context.Context, spec provision.AddNodesSpec) (provision.AddNodesResult, error) {
	f.addSpec = spec
	if f.addErr != nil {
		return provision.AddNodesResult{}, f.addErr
	}
	if f.addResult.Endpoint == "" {
		f.addResult = provision.AddNodesResult{
			Endpoint: spec.ControlPlaneEndpoint, Requested: len(spec.Nodes),
			Joined: len(spec.Nodes),
		}
	}
	return f.addResult, nil
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

// ---- node add (task 6.3) ----

// sshProvisionedManager creates a cluster over SSH through the fake
// provisioner so the in-memory target record carries the endpoint/version a
// node-add request then depends on.
func sshProvisionedManager(t *testing.T, profile Profile, fakeProvisioner *fakeProvisionerDelegator) *KubernetesManager {
	t.Helper()
	manager := NewKubernetesManagerWithProvisioner(profile, fakeProvisioner)
	execution := testContext(profile, "runtime_target.kubernetes.provision-and-register", "create", 1)
	input := testInput(profile, "create", 1)
	input.SSHSecretRef = &SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"}
	input.Parameters = map[string]any{
		"nodes": []any{
			map[string]any{"id": "cp1", "host": "10.0.0.10", "role": "control-plane"},
			map[string]any{"id": "w1", "host": "10.0.0.11", "role": "worker"},
		},
	}
	fakeProvisioner.result = provision.Result{
		ControlPlaneEndpoint: "10.0.0.10:6443",
		NodeCount:            2,
		KubeconfigRef:        provision.SecretReference{Provider: "local", Scope: "tenant", Name: "prov-kubeconfig", Version: "1"},
	}
	if _, err := manager.Apply(context.Background(), execution, input); err != nil {
		t.Fatalf("provision create: %v", err)
	}
	return manager
}

func nodeAddInput(profile Profile, generation int64) LifecycleInput {
	input := testInput(profile, "node-add", generation)
	input.DesiredVersion = ""
	input.SSHSecretRef = &SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"}
	input.Parameters = map[string]any{
		"nodes": []any{map[string]any{"name": "worker-3", "role": "worker"}},
	}
	return input
}

func TestKubernetesManagerAddNodesOverSSH(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	fakeProvisioner := &fakeProvisionerDelegator{}
	manager := sshProvisionedManager(t, profile, fakeProvisioner)
	execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", 2)
	input := nodeAddInput(profile, 2)

	result, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("node add: %v", err)
	}
	if result.Outputs["nodeCount"] != "1" || result.Outputs["joinedCount"] != "1" || result.Outputs["alreadyJoinedCount"] != "0" {
		t.Fatalf("unexpected node-add outputs: %+v", result.Outputs)
	}
	if result.Outputs["endpoint"] != "10.0.0.10:6443" {
		t.Fatalf("endpoint output = %q", result.Outputs["endpoint"])
	}
	if fakeProvisioner.addSpec.ControlPlaneEndpoint != "10.0.0.10:6443" {
		t.Fatalf("endpoint not taken from the record: %+v", fakeProvisioner.addSpec)
	}
	if fakeProvisioner.addSpec.DesiredVersion != "v1.31.0" {
		t.Fatalf("desired version not taken from the record: %+v", fakeProvisioner.addSpec)
	}
	if len(fakeProvisioner.addSpec.Nodes) != 1 ||
		fakeProvisioner.addSpec.Nodes[0].ID != "worker-3" || fakeProvisioner.addSpec.Nodes[0].Host != "worker-3" ||
		fakeProvisioner.addSpec.Nodes[0].Role != provision.RoleWorker {
		t.Fatalf("node identities not mapped: %+v", fakeProvisioner.addSpec.Nodes)
	}
	if fakeProvisioner.addSpec.SSHCredential.Name != "ssh-key" {
		t.Fatalf("ssh credential not carried: %+v", fakeProvisioner.addSpec.SSHCredential)
	}
}

func TestKubernetesManagerAddNodesRefusesWithoutRecordedEndpoint(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	fakeProvisioner := &fakeProvisionerDelegator{}
	manager := NewKubernetesManagerWithProvisioner(profile, fakeProvisioner)
	manager.clientFunc = fakeK8sClient
	execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", 1)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := nodeAddInput(profile, 1)

	_, err := manager.Apply(context.Background(), execution, input)
	var status *StatusError
	if !errors.As(err, &status) || status.HTTPStatus != 409 || status.Retryable {
		t.Fatalf("expected a non-retryable 409, got %v", err)
	}
	if !strings.Contains(err.Error(), "recorded control-plane endpoint") {
		t.Fatalf("unexpected message: %v", err)
	}
	if fakeProvisioner.addSpec.TargetID != "" {
		t.Fatal("provisioner must not be invoked without a recorded endpoint")
	}
}

func TestKubernetesManagerConsecutiveNodeAddsPreserveClusterIdentity(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	delegator := &fakeProvisionerDelegator{}
	manager := sshProvisionedManager(t, profile, delegator)
	cluster := fake.NewSimpleClientset()
	manager.clientFunc = func([]byte) (kubernetes.Interface, error) { return cluster, nil }
	loads := 0
	manager.kubeconfigLoader = func(_ context.Context, _ string, ref provision.SecretReference) ([]byte, error) {
		if ref.Name != "prov-kubeconfig" {
			t.Fatalf("lost kubeconfig identity: %+v", ref)
		}
		loads++
		return []byte("admin-kubeconfig"), nil
	}
	for _, generation := range []int64{2, 3} {
		execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", generation)
		input := nodeAddInput(profile, generation)
		// Planner-authored node-add inputs do not restate desiredVersion.
		input.DesiredVersion = ""
		if _, err := manager.Apply(context.Background(), execution, input); err != nil {
			t.Fatalf("node add generation %d: %v", generation, err)
		}
		if delegator.addSpec.DesiredVersion != "v1.31.0" {
			t.Fatalf("node add lost recorded version: %+v", delegator.addSpec)
		}
		// An exact replay must return the stored result without joining again.
		delegator.addErr = errors.New("unexpected repeated join")
		replayed, err := manager.Apply(context.Background(), execution, input)
		delegator.addErr = nil
		if err != nil || replayed.Outputs["joinedCount"] != "1" {
			t.Fatalf("exact replay did not use recorded result: %+v, %v", replayed, err)
		}
	}
	if loads != 2 {
		t.Fatalf("each node add must persist its record, kubeconfig loads = %d", loads)
	}
	ns, err := cluster.CoreV1().Namespaces().Get(context.Background(), "hnb-target-515eba09", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ann := ns.GetAnnotations()
	if ann[kubernetesVersionAnnotation] != "v1.31.0" || ann[kubernetesFencingAnnotation] != "3" ||
		!strings.Contains(ann[kubernetesKubeconfigRefAnnotation], "prov-kubeconfig") {
		t.Fatalf("durable cluster identity lost: %+v", ann)
	}
}

func TestKubernetesManagerAddNodesSurfacesRetryableFailure(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	fakeProvisioner := &fakeProvisionerDelegator{}
	manager := sshProvisionedManager(t, profile, fakeProvisioner)
	fakeProvisioner.addErr = errors.New("ssh: connection refused")
	execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", 2)
	input := nodeAddInput(profile, 2)

	_, err := manager.Apply(context.Background(), execution, input)
	var status *StatusError
	if !errors.As(err, &status) || status.HTTPStatus != 500 || !status.Retryable || status.Code != ErrorTargetUnavailable {
		t.Fatalf("expected retryable target-unavailable, got %v", err)
	}
}

func TestKubernetesManagerAddNodesRejectsInvalidNodes(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	fakeProvisioner := &fakeProvisionerDelegator{}
	manager := sshProvisionedManager(t, profile, fakeProvisioner)
	execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", 2)
	input := nodeAddInput(profile, 2)
	input.Parameters = map[string]any{"nodes": []any{map[string]any{"name": "cp-1", "role": "control-plane"}}}

	_, err := manager.Apply(context.Background(), execution, input)
	var status *StatusError
	if !errors.As(err, &status) || status.HTTPStatus != 400 {
		t.Fatalf("expected 400 for invalid nodes, got %v", err)
	}
	if !strings.Contains(err.Error(), "worker role") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestKubernetesManagerCreatePersistsNamespaceRecord(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	fakeProvisioner := &fakeProvisionerDelegator{}
	cluster := fake.NewSimpleClientset()
	loadedRef := provision.SecretReference{}
	manager := NewKubernetesManagerWithProvisionerAndLoader(profile, fakeProvisioner,
		func(_ context.Context, _ string, ref provision.SecretReference) ([]byte, error) {
			loadedRef = ref
			return []byte("admin-kubeconfig"), nil
		})
	manager.clientFunc = func([]byte) (kubernetes.Interface, error) { return cluster, nil }
	execution := testContext(profile, "runtime_target.kubernetes.provision-and-register", "create", 1)
	input := testInput(profile, "create", 1)
	input.SSHSecretRef = &SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"}
	input.Parameters = map[string]any{
		"nodes": []any{
			map[string]any{"id": "cp1", "host": "10.0.0.10", "role": "control-plane"},
			map[string]any{"id": "w1", "host": "10.0.0.11", "role": "worker"},
		},
	}
	fakeProvisioner.result = provision.Result{
		ControlPlaneEndpoint: "10.0.0.10:6443",
		NodeCount:            2,
		KubeconfigRef:        provision.SecretReference{Provider: "local", Scope: "tenant", Name: "prov-kubeconfig", Version: "1"},
	}
	if _, err := manager.Apply(context.Background(), execution, input); err != nil {
		t.Fatalf("provision create: %v", err)
	}
	if loadedRef.Name != "prov-kubeconfig" {
		t.Fatalf("kubeconfig reference not resolved: %+v", loadedRef)
	}
	ns, err := cluster.CoreV1().Namespaces().Get(context.Background(), "hnb-target-515eba09", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	ann := ns.GetAnnotations()
	if ann[kubernetesEndpointAnnotation] != "10.0.0.10:6443" {
		t.Fatalf("endpoint annotation = %q", ann[kubernetesEndpointAnnotation])
	}
	if !strings.Contains(ann[kubernetesKubeconfigRefAnnotation], "prov-kubeconfig") {
		t.Fatalf("kubeconfig-ref annotation = %q", ann[kubernetesKubeconfigRefAnnotation])
	}
	if ann[kubernetesActionAnnotation] != "create" || ann[kubernetesFencingAnnotation] != "1" {
		t.Fatalf("record annotations = %+v", ann)
	}
}

// A provider restart must not strand node add: the durable namespace record
// carries the endpoint/kubeconfig reference forward, and a successful join
// refreshes the record (fencing, action) without repeating any bootstrap.
func TestKubernetesManagerAddNodesRecoversRecordAfterRestart(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	cluster := fake.NewSimpleClientset()
	loader := func(context.Context, string, provision.SecretReference) ([]byte, error) {
		return []byte("admin-kubeconfig"), nil
	}
	clientFunc := func([]byte) (kubernetes.Interface, error) { return cluster, nil }

	// First process: create over SSH and persist the record.
	fakeProvisioner := &fakeProvisionerDelegator{}
	manager := NewKubernetesManagerWithProvisionerAndLoader(profile, fakeProvisioner, loader)
	manager.clientFunc = clientFunc
	createExecution := testContext(profile, "runtime_target.kubernetes.provision-and-register", "create", 1)
	createInput := testInput(profile, "create", 1)
	createInput.SSHSecretRef = &SecretReference{Provider: "local", Scope: "tenant", Name: "ssh-key", Version: "1"}
	createInput.Parameters = map[string]any{
		"nodes": []any{
			map[string]any{"id": "cp1", "host": "10.0.0.10", "role": "control-plane"},
			map[string]any{"id": "w1", "host": "10.0.0.11", "role": "worker"},
		},
	}
	fakeProvisioner.result = provision.Result{
		ControlPlaneEndpoint: "10.0.0.10:6443",
		NodeCount:            2,
		KubeconfigRef:        provision.SecretReference{Provider: "local", Scope: "tenant", Name: "prov-kubeconfig", Version: "1"},
	}
	if _, err := manager.Apply(context.Background(), createExecution, createInput); err != nil {
		t.Fatalf("provision create: %v", err)
	}

	// Restart: a fresh manager with no in-memory record serves the node add.
	restartedProvisioner := &fakeProvisionerDelegator{}
	restarted := NewKubernetesManagerWithProvisionerAndLoader(profile, restartedProvisioner, loader)
	restarted.clientFunc = clientFunc
	execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", 2)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := nodeAddInput(profile, 2)

	result, err := restarted.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("node add after restart: %v", err)
	}
	if result.Outputs["joinedCount"] != "1" {
		t.Fatalf("unexpected outputs: %+v", result.Outputs)
	}
	if restartedProvisioner.addSpec.ControlPlaneEndpoint != "10.0.0.10:6443" {
		t.Fatalf("endpoint not recovered from the namespace record: %+v", restartedProvisioner.addSpec)
	}
	if restartedProvisioner.addSpec.DesiredVersion != "v1.31.0" {
		t.Fatalf("version not recovered from the namespace record: %+v", restartedProvisioner.addSpec)
	}

	ns, err := cluster.CoreV1().Namespaces().Get(context.Background(), "hnb-target-515eba09", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	ann := ns.GetAnnotations()
	if ann[kubernetesActionAnnotation] != "node-add" || ann[kubernetesFencingAnnotation] != "2" {
		t.Fatalf("record not refreshed by node add: %+v", ann)
	}
	if ann[kubernetesEndpointAnnotation] != "10.0.0.10:6443" || ann[kubernetesVersionAnnotation] != "v1.31.0" {
		t.Fatalf("durable identity annotations lost: %+v", ann)
	}
}

func TestParseLifecycleInputNodeAddContract(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	input := nodeAddInput(profile, 2)
	execution := testContext(profile, "runtime_target.kubernetes.add-nodes", "node-add", 2)
	execution.Inputs = inputMap(input)
	parsed, err := parseLifecycleInput(profile, execution)
	if err != nil {
		t.Fatalf("parse node-add input: %v", err)
	}
	if parsed.Action != "node-add" || parsed.SSHSecretRef == nil || len(parsed.Parameters["nodes"].([]any)) != 1 {
		t.Fatalf("unexpected parsed input: %+v", parsed)
	}
	incomplete := inputMap(input)
	delete(incomplete, "sshSecretRef")
	execution.Inputs = incomplete
	if _, err := parseLifecycleInput(profile, execution); err == nil || !strings.Contains(err.Error(), "sshSecretRef is required for node add") {
		t.Fatalf("node-add without sshSecretRef must be refused, got %v", err)
	}
}

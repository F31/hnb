package provider

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
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
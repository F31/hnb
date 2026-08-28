package provider

import (
	"context"
	"testing"
	"time"
)

func TestEdgeManagerImportProvisionCloudCoreNamespace(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	manager := NewEdgeManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.edge.register", "import", 1)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "import", 1)
	input.CloudCoreEndpoint = "https://cloudcore.internal:10002"

	result, err := manager.Apply(context.Background(), execution, input)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Outputs["action"] != "import" {
		t.Fatalf("action = %q", result.Outputs["action"])
	}
	if result.Outputs["managed"] != "true" {
		t.Fatalf("managed = %q", result.Outputs["managed"])
	}
	if result.Outputs["cloudCoreEndpoint"] != "https://cloudcore.internal:10002" {
		t.Fatalf("cloudCoreEndpoint = %q", result.Outputs["cloudCoreEndpoint"])
	}
}

func TestEdgeManagerReplayAndFencing(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	manager := NewEdgeManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.edge.register", "import", 2)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "import", 2)
	input.CloudCoreEndpoint = "https://cloudcore.internal:10002"

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

	stale := testContext(profile, "runtime_target.edge.register", "import", 1)
	stale.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	staleInput := testInput(profile, "import", 1)
	staleInput.CloudCoreEndpoint = "https://cloudcore.internal:10002"
	_, err = manager.Apply(context.Background(), stale, staleInput)
	if err == nil {
		t.Fatal("expected fencing error for stale generation")
	}
}

func TestEdgeManagerHonorsCancellation(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	manager := NewEdgeManagerWithClient(profile, fakeK8sClient)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond)

	execution := testContext(profile, "runtime_target.edge.upgrade", "upgrade", 3)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "upgrade", 3)
	input.CloudCoreEndpoint = "https://cloudcore.internal:10002"
	_, err := manager.Apply(ctx, execution, input)
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestEdgeManagerUnmanageRequiresExistingManagement(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	manager := NewEdgeManagerWithClient(profile, fakeK8sClient)
	execution := testContext(profile, "runtime_target.edge.unregister", "unmanage", 4)
	execution.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	input := testInput(profile, "unmanage", 4)
	input.CloudCoreEndpoint = "https://cloudcore.internal:10002"
	_, err := manager.Apply(context.Background(), execution, input)
	if err == nil {
		t.Fatal("expected error for unmanage on unmanaged target")
	}
}

func TestEdgeManagerUpgradeAfterImport(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	manager := NewEdgeManagerWithClient(profile, fakeK8sClient)
	importExec := testContext(profile, "runtime_target.edge.register", "import", 1)
	importExec.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	importInput := testInput(profile, "import", 1)
	importInput.CloudCoreEndpoint = "https://cloudcore.internal:10002"

	_, err := manager.Apply(context.Background(), importExec, importInput)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	upgradeExec := testContext(profile, "runtime_target.edge.upgrade", "upgrade", 2)
	upgradeExec.Inputs["_resolvedSecretContent"] = "dummy-kubeconfig"
	upgradeInput := testInput(profile, "upgrade", 2)
	result, err := manager.Apply(context.Background(), upgradeExec, upgradeInput)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if result.Outputs["action"] != "upgrade" {
		t.Fatalf("action = %q", result.Outputs["action"])
	}
	if result.Outputs["desiredVersion"] != "v1.31.0" {
		t.Fatalf("desiredVersion = %q", result.Outputs["desiredVersion"])
	}
}
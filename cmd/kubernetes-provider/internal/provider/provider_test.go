package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestExecutorIdempotentReplay(t *testing.T) {
	execution := testExecution("deploy", 1)
	client := fake.NewClientset(managedDeployment(execution))
	executor := NewExecutor(client, map[string]struct{}{"hnb-e2e": {}}, 10)

	result, err := executor.Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Outputs["uid"] != "deployment-uid" || result.Outputs["action"] != "deployed" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	if updates := actions(client, "update"); updates != 0 {
		t.Fatalf("updates = %d, want 0", updates)
	}
}

func TestExecutorCreatesAbsentDeployment(t *testing.T) {
	execution := testExecution("deploy", 1)
	client := fake.NewClientset()
	client.PrependReactor("create", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		created := action.(ktesting.CreateAction).GetObject().(*appsv1.Deployment).DeepCopy()
		created.UID = types.UID("created-uid")
		created.ResourceVersion = "1"
		created.Generation = 1
		created.Status.ObservedGeneration = 1
		created.Status.AvailableReplicas = *created.Spec.Replicas
		err := client.Tracker().Create(appsv1.SchemeGroupVersion.WithResource("deployments"), created, created.Namespace)
		return true, created, err
	})

	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if result.Outputs["uid"] != "created-uid" || actions(client, "create") != 1 {
		t.Fatalf("result=%#v actions=%#v", result, client.Actions())
	}
}

func TestExecutorRereadsAfterAlreadyExists(t *testing.T) {
	execution := testExecution("deploy", 1)
	client := fake.NewClientset(managedDeployment(execution))
	firstGet := true
	client.PrependReactor("get", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if firstGet {
			firstGet = false
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "demo")
		}
		return false, nil, nil
	})

	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("already exists reread: %v", err)
	}
	if result.Outputs["uid"] != "deployment-uid" || actions(client, "create") != 1 {
		t.Fatalf("result=%#v actions=%#v", result, client.Actions())
	}
}

func TestExecutorRejectsStaleGeneration(t *testing.T) {
	stored := testExecution("deploy", 2)
	stale := testExecution("deploy", 1)
	client := fake.NewClientset(managedDeployment(stored))

	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), stale)
	assertStatusError(t, err, ErrorFenced, false)
	if updates := actions(client, "update"); updates != 0 {
		t.Fatalf("updates = %d, want 0", updates)
	}
}

func TestAvailabilityPollRejectsNewerGeneration(t *testing.T) {
	execution := testExecution("deploy", 1)
	unavailable := managedDeployment(execution)
	unavailable.Status.AvailableReplicas = 0
	client := fake.NewClientset(unavailable)
	gets := 0
	client.PrependReactor("get", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		gets++
		if gets == 2 {
			newer := managedDeployment(testExecution("deploy", 2))
			return true, newer, nil
		}
		return false, nil, nil
	})

	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorFenced, false)
}

func TestExecutorFailsClosedOnMalformedStoredGeneration(t *testing.T) {
	execution := testExecution("deploy", 2)
	deployment := managedDeployment(testExecution("deploy", 1))
	deployment.Annotations[fencingGenerationAnnotation] = "01"
	client := fake.NewClientset(deployment)

	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorResourceConflict, false)
	if updates := actions(client, "update"); updates != 0 {
		t.Fatalf("updates = %d, want 0", updates)
	}
}

func TestExecutorTakesOverLowerGenerationWithCAS(t *testing.T) {
	stored := testExecution("deploy", 1)
	execution := testExecution("deploy", 2)
	client := fake.NewClientset(managedDeployment(stored))
	conflicts := 0
	client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if conflicts == 0 {
			conflicts++
			return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "demo", nil)
		}
		return false, nil, nil
	})

	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if result.Outputs["uid"] != "deployment-uid" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	current, err := client.AppsV1().Deployments("hnb-e2e").Get(context.Background(), "demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if current.Annotations[fencingGenerationAnnotation] != "2" || current.Annotations[executionAttemptIDAnnotation] != execution.ExecutionAttemptID {
		t.Fatalf("annotations = %#v", current.Annotations)
	}
}

func TestExecutorRejectsTakeoverWithDifferentSpec(t *testing.T) {
	stored := testExecution("deploy", 1)
	execution := testExecution("deploy", 2)
	execution.Inputs["image"] = "registry.k8s.io/pause:3.9"
	client := fake.NewClientset(managedDeployment(stored))

	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorResourceConflict, false)
	if updates := actions(client, "update"); updates != 0 {
		t.Fatalf("updates = %d, want 0", updates)
	}
}

func TestExecutorBoundsCASConflicts(t *testing.T) {
	stored := testExecution("deploy", 1)
	execution := testExecution("deploy", 2)
	client := fake.NewClientset(managedDeployment(stored))
	client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "demo", nil)
	})

	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorTargetUnavailable, true)
	if updates := actions(client, "update"); updates != maxCASAttempts {
		t.Fatalf("updates = %d, want %d", updates, maxCASAttempts)
	}
}

func TestExecutorLogicalDeleteRetainsTombstoneAndPreventsResurrection(t *testing.T) {
	deployed := testExecution("deploy", 1)
	client := fake.NewClientset(managedDeployment(deployed))
	installTombstoneUpdateReactor(client, 1)
	executor := NewExecutor(client, allowedNamespaces(), 10)
	remove := testExecution("delete", 2)
	remove.Inputs["expected_uid"] = "deployment-uid"

	result, err := executor.Execute(context.Background(), remove)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.Outputs["action"] != "deleted" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	tombstone, err := client.AppsV1().Deployments("hnb-e2e").Get(context.Background(), "demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("tombstone was physically deleted: %v", err)
	}
	if tombstone.Spec.Replicas == nil || *tombstone.Spec.Replicas != 0 || tombstone.Annotations[lastActionAnnotation] != "delete" {
		t.Fatalf("tombstone = %#v", tombstone)
	}

	_, err = executor.Execute(context.Background(), deployed)
	assertStatusError(t, err, ErrorFenced, false)
	replay, err := executor.Execute(context.Background(), remove)
	if err != nil || replay.Outputs["uid"] != "deployment-uid" {
		t.Fatalf("delete replay: result=%#v err=%v", replay, err)
	}
	current, _ := client.AppsV1().Deployments("hnb-e2e").Get(context.Background(), "demo", metav1.GetOptions{})
	if current.Spec.Replicas == nil || *current.Spec.Replicas != 0 {
		t.Fatalf("tombstone resurrected by stale request: %#v", current.Spec.Replicas)
	}

	higherDeploy := testExecution("deploy", 3)
	_, err = executor.Execute(context.Background(), higherDeploy)
	assertStatusError(t, err, ErrorResourceConflict, false)
	higherDeploy.Inputs["expected_uid"] = "deployment-uid"
	installAvailableUpdateReactor(client)
	redeployed, err := executor.Execute(context.Background(), higherDeploy)
	if err != nil || redeployed.Outputs["uid"] != "deployment-uid" {
		t.Fatalf("tombstone redeploy: result=%#v err=%v", redeployed, err)
	}
	current, _ = client.AppsV1().Deployments("hnb-e2e").Get(context.Background(), "demo", metav1.GetOptions{})
	if current.Spec.Replicas == nil || *current.Spec.Replicas != 1 || current.Annotations[lastActionAnnotation] != "deploy" {
		t.Fatalf("tombstone was not redeployed: %#v", current)
	}
}

func TestExecutorDeleteRequiresMatchingUID(t *testing.T) {
	deployed := testExecution("deploy", 1)
	client := fake.NewClientset(managedDeployment(deployed))
	remove := testExecution("delete", 2)
	remove.Inputs["expected_uid"] = "wrong"

	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), remove)
	assertStatusError(t, err, ErrorResourceConflict, false)
	if updates := actions(client, "update"); updates != 0 {
		t.Fatalf("updates = %d, want 0", updates)
	}
}

func TestExecutorValidatesAttemptAndGeneration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExecutionContext)
	}{
		{name: "attempt", mutate: func(e *ExecutionContext) { e.ExecutionAttemptID = "not-a-uuid" }},
		{name: "nil UUID", mutate: func(e *ExecutionContext) { e.ExecutionAttemptID = "00000000-0000-0000-0000-000000000000" }},
		{name: "generation", mutate: func(e *ExecutionContext) { e.FencingGeneration = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution := testExecution("deploy", 1)
			tt.mutate(&execution)
			_, err := NewExecutor(fake.NewClientset(), allowedNamespaces(), 10).Execute(context.Background(), execution)
			assertStatusError(t, err, ErrorInvalidRequest, false)
		})
	}
}

func TestHTTPContractV2EchoesFenceAndTypedError(t *testing.T) {
	stored := testExecution("deploy", 2)
	executor := NewExecutor(fake.NewClientset(managedDeployment(stored)), allowedNamespaces(), 10)
	server := httptest.NewServer(NewHandler(executor))
	defer server.Close()

	stale := testExecution("deploy", 1)
	response := postProviderExecution(t, server.URL, wireExecution(stale))
	if response.code != http.StatusConflict || response.body.Status != "failed" || response.body.ErrorCode != ErrorFenced {
		t.Fatalf("HTTP %d response %#v", response.code, response.body)
	}
	if response.body.ExecutionAttemptID != stale.ExecutionAttemptID || response.body.FencingGeneration != "1" {
		t.Fatalf("response did not echo fence: %#v", response.body)
	}
	if response.body.Retryable == nil || *response.body.Retryable {
		t.Fatalf("retryable = %#v", response.body.Retryable)
	}
}

func TestHTTPContractRejectsNonCanonicalGenerationAndUnknownFields(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewExecutor(fake.NewClientset(), allowedNamespaces(), 10)))
	defer server.Close()

	execution := wireExecution(testExecution("deploy", 1))
	execution.FencingGeneration = "01"
	response := postProviderExecution(t, server.URL, execution)
	if response.code != http.StatusBadRequest || response.body.ErrorCode != ErrorInvalidRequest || response.body.FencingGeneration != "01" {
		t.Fatalf("noncanonical response: HTTP %d %#v", response.code, response.body)
	}

	raw, err := http.Post(server.URL+"/v2/steps:execute", "application/json", strings.NewReader(`{"schemaVersion":"2.0.0","execution":{},"extra":true}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer raw.Body.Close()
	if raw.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", raw.StatusCode)
	}
}

type httpResult struct {
	code int
	body executeResponse
}

func postProviderExecution(t *testing.T, baseURL string, execution executionRequest) httpResult {
	t.Helper()
	body, err := json.Marshal(executeRequest{SchemaVersion: contractVersion, Execution: execution})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	response, err := http.Post(baseURL+"/v2/steps:execute", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer response.Body.Close()
	var result executeResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return httpResult{code: response.StatusCode, body: result}
}

func wireExecution(execution ExecutionContext) executionRequest {
	return executionRequest{
		StepID: execution.StepID, OperationID: execution.OperationID, TenantID: execution.TenantID,
		ProjectID: execution.ProjectID, EnvironmentID: execution.EnvironmentID, StepType: execution.StepType,
		Inputs: execution.Inputs, ProviderID: execution.ProviderID,
		ProviderVersion: execution.ProviderVersion, ProviderDigest: execution.ProviderDigest,
		ProviderProtocolVersion: execution.ProviderProtocolVersion,
		Checkpoint:              execution.Checkpoint, IdempotencyKey: execution.IdempotencyKey,
		ExecutionAttemptID: execution.ExecutionAttemptID,
		FencingGeneration:  strconv.FormatInt(execution.FencingGeneration, 10),
	}
}

func managedDeployment(execution ExecutionContext) *appsv1.Deployment {
	executor := NewExecutor(fake.NewClientset(), allowedNamespaces(), 10)
	deployment, err := executor.desiredDeployment(execution, "hnb-e2e", "demo")
	if err != nil {
		panic(err)
	}
	deployment.UID = types.UID("deployment-uid")
	deployment.ResourceVersion = "1"
	deployment.Generation = 1
	deployment.Status = appsv1.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: *deployment.Spec.Replicas}
	if execution.StepType == "delete" {
		zero := int32(0)
		deployment.Spec.Replicas = &zero
		deployment.Status.AvailableReplicas = 0
	}
	return deployment
}

func stringMapToAny(in map[string]string) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func testExecution(stepType string, generation int64) ExecutionContext {
	inputs := map[string]string{"namespace": "hnb-e2e", "name": "demo"}
	if stepType == "deploy" {
		inputs["image"] = "registry.k8s.io/pause:3.10"
		inputs["replicas"] = "1"
	}
	attempts := map[int64]string{
		1: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		2: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
		3: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3",
	}
	return ExecutionContext{
		StepID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222", TenantID: "tenant-a",
		StepType: stepType, ProviderID: "kind-provider", IdempotencyKey: "idempotency-a",
		ExecutionAttemptID: attempts[generation], FencingGeneration: generation, Inputs: stringMapToAny(inputs),
	}
}

func installTombstoneUpdateReactor(client *fake.Clientset, conflicts int) {
	client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if conflicts > 0 {
			conflicts--
			return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "demo", nil)
		}
		updated := action.(ktesting.UpdateAction).GetObject().(*appsv1.Deployment).DeepCopy()
		updated.Status.AvailableReplicas = 0
		updated.Status.ObservedGeneration = updated.Generation
		err := client.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), updated, updated.Namespace)
		return true, updated, err
	})
}

func installAvailableUpdateReactor(client *fake.Clientset) {
	client.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		updated := action.(ktesting.UpdateAction).GetObject().(*appsv1.Deployment).DeepCopy()
		updated.Status.AvailableReplicas = *updated.Spec.Replicas
		updated.Status.ObservedGeneration = updated.Generation
		err := client.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), updated, updated.Namespace)
		return true, updated, err
	})
}

func assertStatusError(t *testing.T, err error, code ErrorCode, retryable bool) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	_, gotCode, gotRetryable := statusDetails(err)
	if gotCode != code || gotRetryable != retryable {
		t.Fatalf("error = %v, code = %s, retryable = %t", err, gotCode, gotRetryable)
	}
}

func actions(client *fake.Clientset, verb string) int {
	count := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == verb && action.GetResource().Resource == "deployments" {
			count++
		}
	}
	return count
}

func allowedNamespaces() map[string]struct{} {
	return map[string]struct{}{"hnb-e2e": {}}
}

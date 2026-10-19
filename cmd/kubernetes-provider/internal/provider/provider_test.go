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
	corev1 "k8s.io/api/core/v1"
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
		updated.Status.UpdatedReplicas = *updated.Spec.Replicas
		updated.Status.ReadyReplicas = *updated.Spec.Replicas
		updated.Status.UnavailableReplicas = 0
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

func serviceExecution(generation int64, port string) ExecutionContext {
	execution := testExecution("service.deploy", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo-svc", "workload": "demo", "port": port,
	})
	return execution
}

func managedService(execution ExecutionContext) *corev1.Service {
	service := desiredService(execution, "hnb-e2e", "demo-svc", "demo", 8080)
	service.UID = types.UID("service-uid")
	service.ResourceVersion = "1"
	return service
}

func TestServiceDeployCreatesTypedAssociation(t *testing.T) {
	execution := serviceExecution(1, "8080")
	client := fake.NewClientset(managedDeployment(testExecution("deploy", 1)))
	client.PrependReactor("create", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		created := action.(ktesting.CreateAction).GetObject().(*corev1.Service).DeepCopy()
		created.UID = types.UID("service-uid")
		created.ResourceVersion = "1"
		err := client.Tracker().Create(corev1.SchemeGroupVersion.WithResource("services"), created, created.Namespace)
		return true, created, err
	})
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["uid"] != "service-uid" || result.Outputs["action"] != "service.deploy" {
		t.Fatalf("result = %#v", result)
	}
	service, err := client.CoreV1().Services("hnb-e2e").Get(context.Background(), "demo-svc", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if service.Spec.Selector["hnb.io/workload"] != "demo" || service.Spec.Ports[0].Port != 8080 || service.Annotations[tenantAnnotation] != "tenant-a" {
		t.Fatalf("service = %#v", service)
	}
}

func TestServiceDeployReplayAndFencing(t *testing.T) {
	execution := serviceExecution(1, "8080")
	service := managedService(execution)
	client := fake.NewClientset(managedDeployment(testExecution("deploy", 1)), service)
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil || result.Outputs["uid"] != "service-uid" {
		t.Fatalf("replay = %#v/%v", result, err)
	}
	if serviceActions(client, "update") != 0 {
		t.Fatal("exact replay updated the service")
	}

	stale := serviceExecution(1, "9090")
	_, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), stale)
	assertStatusError(t, err, ErrorResourceConflict, false)

	newer := managedService(serviceExecution(2, "8080"))
	newer.Annotations[operationAnnotation] = "22222222-2222-4222-8222-222222222222"
	client = fake.NewClientset(managedDeployment(testExecution("deploy", 1)), newer)
	_, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorFenced, false)
}

func TestServiceDeployDoesNotPartiallyCreateWithoutOwnedWorkload(t *testing.T) {
	execution := serviceExecution(1, "8080")
	client := fake.NewClientset()
	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorResourceConflict, false)
	if serviceActions(client, "create") != 0 {
		t.Fatal("service was created without an owned workload")
	}

	foreign := managedDeployment(testExecution("deploy", 1))
	foreign.Annotations[tenantAnnotation] = "tenant-b"
	client = fake.NewClientset(foreign)
	_, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorResourceConflict, false)
	if serviceActions(client, "create") != 0 {
		t.Fatal("service was created for a foreign workload")
	}
}

func TestServiceDeployRejectsInvalidPortBeforeWrite(t *testing.T) {
	execution := serviceExecution(1, "08080")
	client := fake.NewClientset(managedDeployment(testExecution("deploy", 1)))
	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorInvalidRequest, false)
	if serviceActions(client, "create") != 0 {
		t.Fatal("service was created for an invalid port")
	}
}

func serviceActions(client *fake.Clientset, verb string) int {
	count := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == verb && action.GetResource().Resource == "services" {
			count++
		}
	}
	return count
}

func TestServiceDeployRetriesCASConflict(t *testing.T) {
	execution := serviceExecution(2, "8080")
	previous := serviceExecution(1, "8080")
	client := fake.NewClientset(managedDeployment(testExecution("deploy", 1)), managedService(previous))
	conflicted := false
	client.PrependReactor("update", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		if !conflicted {
			conflicted = true
			return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "", Resource: "services"}, "demo-svc", nil)
		}
		updated := action.(ktesting.UpdateAction).GetObject().(*corev1.Service).DeepCopy()
		updated.UID = types.UID("service-uid")
		updated.ResourceVersion = "2"
		err := client.Tracker().Update(corev1.SchemeGroupVersion.WithResource("services"), updated, updated.Namespace)
		return true, updated, err
	})
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["uid"] != "service-uid" || serviceActions(client, "update") != 2 {
		t.Fatalf("result=%#v updates=%d", result, serviceActions(client, "update"))
	}
}

func pvcExecution(generation int64, size string) ExecutionContext {
	execution := testExecution("pvc.deploy", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo-data", "storage_class": "fast", "size": size,
	})
	return execution
}

func TestPVCCreateAndReplay(t *testing.T) {
	execution := pvcExecution(1, "128Mi")
	client := fake.NewClientset()
	client.PrependReactor("create", "persistentvolumeclaims", func(action ktesting.Action) (bool, runtime.Object, error) {
		created := action.(ktesting.CreateAction).GetObject().(*corev1.PersistentVolumeClaim).DeepCopy()
		created.UID = types.UID("pvc-uid")
		created.ResourceVersion = "1"
		created.Spec.VolumeName = "pv-1"
		created.Status.Phase = corev1.ClaimBound
		err := client.Tracker().Create(corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims"), created, created.Namespace)
		return true, created, err
	})
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["uid"] != "pvc-uid" || result.Outputs["volumeName"] != "pv-1" || result.Outputs["action"] != "pvc.deploy" {
		t.Fatalf("result = %#v", result)
	}
	claim, err := client.CoreV1().PersistentVolumeClaims("hnb-e2e").Get(context.Background(), "demo-data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if claim.Spec.Resources.Requests.Storage().String() != "128Mi" || *claim.Spec.StorageClassName != "fast" || claim.Annotations[tenantAnnotation] != "tenant-a" {
		t.Fatalf("claim = %#v", claim)
	}
	result, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil || result.Outputs["uid"] != "pvc-uid" {
		t.Fatalf("replay = %#v/%v", result, err)
	}
	if claims := actionsFor(client, "persistentvolumeclaims", "update"); claims != 0 {
		t.Fatal("exact replay updated the pvc")
	}
}

func TestPVCFencingAndInvalidSize(t *testing.T) {
	client := fake.NewClientset()
	desired := &corev1.PersistentVolumeClaim{}
	// Create a managed PVC at generation 2 without binding it.
	execution := pvcExecution(2, "128Mi")
	desired.ObjectMeta = metav1.ObjectMeta{Name: "demo-data", Namespace: "hnb-e2e", UID: types.UID("pvc-uid")}
	desired.Spec.StorageClassName = new(string)
	*desired.Spec.StorageClassName = "fast"
	setPVCExecutionAnnotations(desired, execution, "pvc.deploy")
	client = fake.NewClientset(desired)
	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), pvcExecution(1, "128Mi"))
	assertStatusError(t, err, ErrorFenced, false)

	client = fake.NewClientset()
	_, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), pvcExecution(1, "0Mi"))
	assertStatusError(t, err, ErrorInvalidRequest, false)
	if actionsFor(client, "persistentvolumeclaims", "create") != 0 {
		t.Fatal("invalid pvc was created")
	}
}

func TestDeploymentMountsPVCAndSecretWithoutReadingSecret(t *testing.T) {
	execution := testExecution("deploy", 1)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo", "image": "registry.example/app@sha256:" + strings.Repeat("a", 64),
		"replicas": "1", "pvc_name": "demo-data", "pvc_mount_path": "/data",
		"secret_name": "demo-secret", "secret_mount_path": "/etc/hnb-secret",
	})
	executor := NewExecutor(fake.NewClientset(), allowedNamespaces(), 10)
	deployment, err := executor.desiredDeployment(execution, "hnb-e2e", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(deployment.Spec.Template.Spec.Volumes) != 2 || len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) != 2 {
		t.Fatalf("volumes = %#v mounts = %#v", deployment.Spec.Template.Spec.Volumes, deployment.Spec.Template.Spec.Containers[0].VolumeMounts)
	}
	if deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != "demo-data" ||
		deployment.Spec.Template.Spec.Volumes[1].Secret.SecretName != "demo-secret" {
		t.Fatalf("volumes = %#v", deployment.Spec.Template.Spec.Volumes)
	}
}

func actionsFor(client *fake.Clientset, resource, verb string) int {
	count := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == verb && action.GetResource().Resource == resource {
			count++
		}
	}
	return count
}

func upgradeExecution(generation int64, source, target, compatible, backup string) ExecutionContext {
	execution := testExecution("workload.upgrade", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo", "source_image": source, "target_image": target,
		"expected_uid": "deployment-uid", "data_compatible": compatible, "backup_ref": backup,
	})
	return execution
}

func TestWorkloadUpgradeUsesPinnedSourceTargetAndBackup(t *testing.T) {
	base := managedDeployment(testExecution("deploy", 1))
	source := "registry.example/v1@sha256:" + strings.Repeat("c", 64)
	base.Spec.Template.Spec.Containers[0].Image = source
	target := "registry.example/v2@sha256:" + strings.Repeat("d", 64)
	execution := upgradeExecution(2, source, target, "true", "backup-20261018")
	client := fake.NewClientset(base)
	installAvailableUpdateReactor(client)
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["action"] != "workload.upgraded" {
		t.Fatalf("result = %#v", result)
	}
	current, err := client.AppsV1().Deployments("hnb-e2e").Get(context.Background(), "demo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.Spec.Template.Spec.Containers[0].Image != target ||
		current.Annotations[sourceImageAnnotation] != source ||
		current.Annotations[targetImageAnnotation] != target ||
		current.Annotations[backupRefAnnotation] != "backup-20261018" {
		t.Fatalf("deployment = %#v", current)
	}
}

func TestWorkloadUpgradeRejectsMissingCompatibilityBackupAndSource(t *testing.T) {
	base := managedDeployment(testExecution("deploy", 1))
	source := "registry.example/v1@sha256:" + strings.Repeat("c", 64)
	base.Spec.Template.Spec.Containers[0].Image = source
	target := "registry.example/v2@sha256:" + strings.Repeat("d", 64)
	cases := []struct {
		execution ExecutionContext
		code      ErrorCode
	}{
		{upgradeExecution(2, source, target, "false", "backup"), ErrorInvalidRequest},
		{upgradeExecution(2, source, target, "true", ""), ErrorInvalidRequest},
		{upgradeExecution(2, "registry.example/other@sha256:"+strings.Repeat("e", 64), target, "true", "backup"), ErrorResourceConflict},
	}
	for _, tc := range cases {
		client := fake.NewClientset(base)
		_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), tc.execution)
		assertStatusError(t, err, tc.code, false)
	}
}

func rollbackExecution(generation int64, source, target, irreversible, backup string) ExecutionContext {
	execution := testExecution("workload.rollback", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo", "source_image": source, "target_image": target,
		"expected_uid": "deployment-uid", "data_irreversible": irreversible, "backup_ref": backup,
	})
	return execution
}

func TestWorkloadRollbackRestoresPinnedTarget(t *testing.T) {
	base := managedDeployment(testExecution("deploy", 1))
	bad := "registry.example/bad@sha256:" + strings.Repeat("e", 64)
	good := "registry.example/v2@sha256:" + strings.Repeat("d", 64)
	base.Spec.Template.Spec.Containers[0].Image = bad
	execution := rollbackExecution(3, bad, good, "false", "lab-backup-v1")
	client := fake.NewClientset(base)
	installAvailableUpdateReactor(client)
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["action"] != "workload.rolledback" {
		t.Fatalf("result = %#v", result)
	}
	current, err := client.AppsV1().Deployments("hnb-e2e").Get(context.Background(), "demo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.Spec.Template.Spec.Containers[0].Image != good || current.Annotations[backupRefAnnotation] != "lab-backup-v1" {
		t.Fatalf("deployment = %#v", current)
	}
}

func TestWorkloadRollbackRejectsIrreversibleAndMissingBackup(t *testing.T) {
	base := managedDeployment(testExecution("deploy", 1))
	bad := "registry.example/bad@sha256:" + strings.Repeat("e", 64)
	good := "registry.example/v2@sha256:" + strings.Repeat("d", 64)
	base.Spec.Template.Spec.Containers[0].Image = bad
	for _, execution := range []ExecutionContext{
		rollbackExecution(3, bad, good, "true", "backup"),
		rollbackExecution(3, bad, good, "false", ""),
	} {
		client := fake.NewClientset(base)
		_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
		assertStatusError(t, err, ErrorInvalidRequest, false)
	}
}

func deletePVCExecution(generation int64, authorized, uid string) ExecutionContext {
	execution := testExecution("pvc.delete", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo-data", "expected_uid": uid, "data_cleanup_authorized": authorized,
	})
	return execution
}

func managedPVC(execution ExecutionContext) *corev1.PersistentVolumeClaim {
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "demo-data", Namespace: "hnb-e2e", UID: types.UID("pvc-uid")}}
	storageClass := "fast"
	claim.Spec.StorageClassName = &storageClass
	setPVCExecutionAnnotations(claim, execution, "pvc.deploy")
	return claim
}

func TestPVCDeleteRequiresExplicitAuthorizationAndOwner(t *testing.T) {
	base := managedPVC(pvcExecution(1, "128Mi"))
	client := fake.NewClientset(base)
	_, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), deletePVCExecution(2, "false", "pvc-uid"))
	assertStatusError(t, err, ErrorInvalidRequest, false)
	if actionsFor(client, "persistentvolumeclaims", "delete") != 0 {
		t.Fatal("unauthorized pvc delete reached the target")
	}
	_, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), deletePVCExecution(2, "true", "wrong-uid"))
	assertStatusError(t, err, ErrorResourceConflict, false)

	foreign := base.DeepCopy()
	foreign.Annotations[tenantAnnotation] = "tenant-b"
	client = fake.NewClientset(foreign)
	_, err = NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), deletePVCExecution(2, "true", "pvc-uid"))
	assertStatusError(t, err, ErrorResourceConflict, false)

	client = fake.NewClientset(base)
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), deletePVCExecution(2, "true", "pvc-uid"))
	if err != nil || result.Outputs["action"] != "pvc.deleted" {
		t.Fatalf("delete = %#v/%v", result, err)
	}
	if _, err := client.CoreV1().PersistentVolumeClaims("hnb-e2e").Get(context.Background(), "demo-data", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("pvc still exists: %v", err)
	}
}

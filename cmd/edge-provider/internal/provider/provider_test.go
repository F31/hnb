package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestConfigLoad(t *testing.T) {
	t.Setenv("ALLOWED_NAMESPACES", "hnb-e2e,hnb-workloads")
	t.Setenv("MAX_REPLICAS", "5")
	t.Setenv("CLOUDCORE_ENDPOINT", "https://cloudcore:10002")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.MaxReplicas != 5 {
		t.Fatalf("max replicas = %d", cfg.MaxReplicas)
	}
	if _, ok := cfg.AllowedNamespaces["hnb-e2e"]; !ok {
		t.Fatal("hnb-e2e missing")
	}
	if cfg.CloudCoreEndpoint != "https://cloudcore:10002" {
		t.Fatalf("endpoint = %s", cfg.CloudCoreEndpoint)
	}
}

func TestConfigLoadRequiresCloudCoreEndpoint(t *testing.T) {
	t.Setenv("ALLOWED_NAMESPACES", "hnb-e2e")
	t.Setenv("CLOUDCORE_ENDPOINT", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error for missing CLOUDCORE_ENDPOINT")
	}
}

func TestConfigLoadRequiresNamespaces(t *testing.T) {
	t.Setenv("ALLOWED_NAMESPACES", "")
	t.Setenv("CLOUDCORE_ENDPOINT", "https://cloudcore:10002")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error for missing ALLOWED_NAMESPACES")
	}
}

func TestExecutorValidatesInput(t *testing.T) {
	k8sClient := k8sfake.NewClientset()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	executor := NewExecutor(k8sClient, dynamicClient, nil, map[string]struct{}{"hnb-e2e": {}}, 10)

	tests := []struct {
		name   string
		mutate func(*ExecutionContext)
	}{
		{name: "missing tenant", mutate: func(e *ExecutionContext) { e.TenantID = "" }},
		{name: "missing operation", mutate: func(e *ExecutionContext) { e.OperationID = "" }},
		{name: "missing step", mutate: func(e *ExecutionContext) { e.StepID = "" }},
		{name: "missing idempotency", mutate: func(e *ExecutionContext) { e.IdempotencyKey = "" }},
		{name: "invalid attempt", mutate: func(e *ExecutionContext) { e.ExecutionAttemptID = "not-a-uuid" }},
		{name: "nil UUID attempt", mutate: func(e *ExecutionContext) { e.ExecutionAttemptID = "00000000-0000-0000-0000-000000000000" }},
		{name: "zero generation", mutate: func(e *ExecutionContext) { e.FencingGeneration = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution := testExecution("deploy", 1)
			tt.mutate(&execution)
			_, err := executor.Execute(context.Background(), execution)
			assertStatusError(t, err, ErrorInvalidRequest, false)
		})
	}
}

func TestExecutorUnsupportedStepType(t *testing.T) {
	k8sClient := k8sfake.NewClientset()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	executor := NewExecutor(k8sClient, dynamicClient, nil, map[string]struct{}{"hnb-e2e": {}}, 10)

	execution := testExecution("unsupported", 1)
	_, err := executor.Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorUnsupportedAction, false)
}

func TestExecutorValidatesResourceScope(t *testing.T) {
	k8sClient := k8sfake.NewClientset()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	executor := NewExecutor(k8sClient, dynamicClient, nil, map[string]struct{}{"hnb-e2e": {}}, 10)

	execution := testExecution("deploy", 1)
	execution.Inputs["namespace"] = "unauthorized"
	_, err := executor.Execute(context.Background(), execution)
	assertStatusError(t, err, ErrorScopeDenied, false)
}

func TestHTTPContractRejectsSchemaVersion(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewExecutor(
		k8sfake.NewClientset(),
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		nil, map[string]struct{}{"hnb-e2e": {}}, 10,
	)))
	defer server.Close()

	body := strings.NewReader(`{"schemaVersion":"1.0.0","execution":{"step_id":"s1","operation_id":"o1","tenant_id":"t1","step_type":"deploy","inputs":{},"provider_id":"p1","idempotency_key":"ik","execution_attempt_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","fencing_generation":"1"}}`)
	raw, err := http.Post(server.URL+"/v2/steps:execute", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer raw.Body.Close()
	if raw.StatusCode != http.StatusBadRequest {
		t.Fatalf("schema version rejection status = %d", raw.StatusCode)
	}
}

func TestHTTPContractRejectsUnknownFields(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewExecutor(
		k8sfake.NewClientset(),
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		nil, map[string]struct{}{"hnb-e2e": {}}, 10,
	)))
	defer server.Close()

	raw, err := http.Post(server.URL+"/v2/steps:execute", "application/json", strings.NewReader(`{"schemaVersion":"2.0.0","execution":{},"extra":true}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer raw.Body.Close()
	if raw.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", raw.StatusCode)
	}
}

func TestGetAnnotation(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAnnotations(map[string]string{"key": "value"})

	val, ok := getAnnotation(obj, "key")
	if !ok || val != "value" {
		t.Fatalf("getAnnotation = %q, %v", val, ok)
	}

	_, ok = getAnnotation(obj, "nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent key")
	}
}

func TestStoredFencingGeneration(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAnnotations(map[string]string{fencingGenerationAnnotation: "42"})

	gen, err := storedFencingGeneration(obj)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gen != 42 {
		t.Fatalf("gen = %d", gen)
	}

	obj.SetAnnotations(map[string]string{fencingGenerationAnnotation: "01"})
	_, err = storedFencingGeneration(obj)
	if err == nil {
		t.Fatal("expected error for non-canonical generation")
	}

	obj.SetAnnotations(map[string]string{})
	_, err = storedFencingGeneration(obj)
	if err == nil {
		t.Fatal("expected error for missing generation")
	}
}

func TestSameLogicalIdentity(t *testing.T) {
	execution := testExecution("deploy", 1)
	obj := &unstructured.Unstructured{}
	setExecutionAnnotations(obj, execution, "deploy")

	if !sameLogicalIdentity(obj, execution) {
		t.Fatal("expected same logical identity")
	}

	diff := execution
	diff.TenantID = "other"
	if sameLogicalIdentity(obj, diff) {
		t.Fatal("expected different logical identity")
	}
}

func TestEdgeAppResult(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("test-app")
	obj.SetNamespace("hnb-e2e")
	obj.SetUID("test-uid")
	obj.SetResourceVersion("1")

	result := edgeAppResult(obj, "deployed")
	if result.Outputs["name"] != "test-app" || result.Outputs["namespace"] != "hnb-e2e" {
		t.Fatalf("outputs = %#v", result.Outputs)
	}
	if result.Checkpoint != "edgeapplication:hnb-e2e/test-app:test-uid" {
		t.Fatalf("checkpoint = %s", result.Checkpoint)
	}
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
		inputs["expected_uid"] = ""
	}
	if stepType == "delete" {
		inputs["expected_uid"] = "test-uid"
	}
	attempts := map[int64]string{
		1: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		2: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
	}
	return ExecutionContext{
		StepID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222", TenantID: "tenant-a",
		StepType: stepType, ProviderID: "edge-provider", IdempotencyKey: "idempotency-a",
		ExecutionAttemptID: attempts[generation], FencingGeneration: generation, Inputs: stringMapToAny(inputs),
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
		Checkpoint:              execution.Checkpoint,
		IdempotencyKey:          execution.IdempotencyKey, ExecutionAttemptID: execution.ExecutionAttemptID,
		FencingGeneration: strconv.FormatInt(execution.FencingGeneration, 10),
	}
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

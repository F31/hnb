package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestKubernetesLifecycleProviderRoutesAllActions(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	server := httptest.NewServer(NewServer(profile, NewMemoryManager(profile), recordingSecretResolver{}, NewMemoryObserverRegistry()).Handler())
	defer server.Close()

	for _, tc := range []struct {
		stepType string
		action   string
		gen      int64
	}{
		{"runtime_target.kubernetes.provision-and-register", "create", 1},
		{"runtime_target.kubernetes.register", "import", 2},
		{"runtime_target.kubernetes.upgrade", "upgrade", 3},
	} {
		t.Run(tc.stepType, func(t *testing.T) {
			execution := testExecution(profile, tc.stepType, tc.action, tc.gen)
			response := postExecution(t, server.URL, execution)
			if response.code != http.StatusOK || response.body.Status != "succeeded" || response.body.Outputs["action"] != tc.action {
				t.Fatalf("HTTP %d %#v", response.code, response.body)
			}
		})
	}
}

func TestEdgeLifecycleProviderRoutesAndValidatesEndpoint(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	server := httptest.NewServer(NewServer(profile, NewMemoryManager(profile), recordingSecretResolver{}, NewMemoryObserverRegistry()).Handler())
	defer server.Close()

	importExecution := testExecution(profile, "runtime_target.edge.register", "import", 1)
	response := postExecution(t, server.URL, importExecution)
	if response.code != http.StatusOK || response.body.Outputs["observationKind"] != "CloudCore" {
		t.Fatalf("HTTP %d %#v", response.code, response.body)
	}

	badEndpoint := testExecution(profile, "runtime_target.edge.register", "import", 2)
	badEndpoint.Inputs["cloudCoreEndpoint"] = "https://user:pass@cloudcore.internal:10002?token=x"
	response = postExecution(t, server.URL, badEndpoint)
	if response.code != http.StatusBadRequest || response.body.ErrorCode != ErrorInvalidRequest || strings.Contains(response.body.Error, "pass") || strings.Contains(response.body.Error, "token=x") {
		t.Fatalf("unsafe endpoint response: HTTP %d %#v", response.code, response.body)
	}
}

func TestLifecycleProviderReplayAndFencing(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	server := httptest.NewServer(NewServer(profile, NewMemoryManager(profile), recordingSecretResolver{}, NewMemoryObserverRegistry()).Handler())
	defer server.Close()

	first := testExecution(profile, "runtime_target.kubernetes.register", "import", 2)
	response := postExecution(t, server.URL, first)
	if response.code != http.StatusOK {
		t.Fatalf("first HTTP %d %#v", response.code, response.body)
	}
	replay := postExecution(t, server.URL, first)
	if replay.code != http.StatusOK || replay.body.Checkpoint != response.body.Checkpoint {
		t.Fatalf("replay HTTP %d %#v", replay.code, replay.body)
	}
	stale := first
	stale.FencingGeneration = "1"
	stale.Inputs["fencingGeneration"] = float64(1)
	stale.ExecutionAttemptID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	stale.IdempotencyKey = "step-stale"
	stale.Inputs["idempotencyKey"] = "step-stale"
	response = postExecution(t, server.URL, stale)
	if response.code != http.StatusConflict || response.body.ErrorCode != ErrorFenced {
		t.Fatalf("stale HTTP %d %#v", response.code, response.body)
	}
}

func TestLifecycleProviderRejectsSchemaDriftAndSecretLeak(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.edge")
	resolver := recordingSecretResolver{}
	server := httptest.NewServer(NewServer(profile, NewMemoryManager(profile), resolver, NewMemoryObserverRegistry()).Handler())
	defer server.Close()

	execution := testExecution(profile, "runtime_target.edge.register", "import", 1)
	execution.Inputs["providerId"] = "runtime-target.lifecycle.edge"
	response := postExecution(t, server.URL, execution)
	if response.code != http.StatusBadRequest || response.body.ErrorCode != ErrorInvalidRequest {
		t.Fatalf("unsupported input HTTP %d %#v", response.code, response.body)
	}

	execution = testExecution(profile, "runtime_target.edge.register", "import", 1)
	execution.Inputs["credentialSecretRef"] = map[string]any{"provider": "vault", "scope": "tenant", "name": "edge-credential", "version": "1", "value": "secret"}
	response = postExecution(t, server.URL, execution)
	if response.code != http.StatusOK || strings.Contains(mustMarshal(t, response.body), "secret") {
		t.Fatalf("secret leaked or request failed: HTTP %d %#v", response.code, response.body)
	}
}

func TestLifecycleProviderHonorsCancellation(t *testing.T) {
	profile := mustProfile(t, "runtime-target.lifecycle.kubernetes")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewMemoryManager(profile).Apply(ctx, testContext(profile, "runtime_target.kubernetes.upgrade", "upgrade", 3), testInput(profile, "upgrade", 3))
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if statusCode(err) != http.StatusRequestTimeout {
		t.Fatalf("status = %d err=%v", statusCode(err), err)
	}
}

type recordingSecretResolver struct{}

func (recordingSecretResolver) Resolve(_ context.Context, tenantID, providerID, purpose string, ref SecretReference) ([]byte, error) {
	if tenantID != "tenant-a" || providerID == "" || purpose == "" || ref.Name == "" {
		return nil, invalid("bad secret metadata")
	}
	return []byte("kubeconfig-content"), nil
}

type httpResult struct {
	code int
	body executeResponse
}

func postExecution(t *testing.T, baseURL string, execution executionRequest) httpResult {
	t.Helper()
	payload, err := json.Marshal(executeRequest{SchemaVersion: ContractVersion, Execution: execution})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(baseURL+"/v2/steps:execute", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body executeResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return httpResult{code: response.StatusCode, body: body}
}

func testExecution(profile Profile, stepType, action string, generation int64) executionRequest {
	ctx := testContext(profile, stepType, action, generation)
	return executionRequest{
		StepID: ctx.StepID, OperationID: ctx.OperationID, TenantID: ctx.TenantID,
		StepType: ctx.StepType, Inputs: ctx.Inputs, ProviderID: ctx.ProviderID,
		ProviderVersion: ctx.ProviderVersion, ProviderDigest: ctx.ProviderDigest,
		ProviderProtocolVersion: ctx.ProviderProtocolVersion, IdempotencyKey: ctx.IdempotencyKey,
		ExecutionAttemptID: ctx.ExecutionAttemptID, FencingGeneration: ctxFencing(ctx),
	}
}

func testContext(profile Profile, stepType, action string, generation int64) ExecutionContext {
	attempts := map[int64]string{
		1: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		2: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
		3: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3",
	}
	return ExecutionContext{
		StepID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222",
		TenantID: "tenant-a", StepType: stepType, Inputs: inputMap(testInput(profile, action, generation)),
		ProviderID: profile.ProviderID, ProviderVersion: "1.0.0", ProviderDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ProviderProtocolVersion: ContractVersion, IdempotencyKey: "step-key", ExecutionAttemptID: attempts[generation], FencingGeneration: generation,
	}
}

func testInput(profile Profile, action string, generation int64) LifecycleInput {
	input := LifecycleInput{
		SchemaVersion: "1.0.0", TargetID: "515eba09-0a41-5b92-b972-69af1f0f655c", TargetKind: profile.TargetKind,
		Action: action, DisplayName: "prod-cluster", DesiredVersion: "v1.31.0",
		IdempotencyKey: "step-key", FencingGeneration: generation, ObservationVersion: 0,
	}
	if action == "create" || action == "import" {
		input.CredentialSecretRef = &SecretReference{Provider: "vault", Scope: "tenant", Name: "cluster-credential", Version: "1"}
	}
	if profile.TargetKind == "EdgeRuntimeTarget" {
		input.TargetID = "6d384d43-243b-5e14-b7e4-c03be376cb7c"
		input.CloudCoreEndpoint = "https://cloudcore.internal:10002"
	}
	return input
}

func inputMap(input LifecycleInput) map[string]any {
	out := map[string]any{
		"schemaVersion": input.SchemaVersion, "targetId": input.TargetID, "targetKind": input.TargetKind,
		"action": input.Action, "idempotencyKey": input.IdempotencyKey,
		"fencingGeneration": float64(input.FencingGeneration), "observationVersion": float64(input.ObservationVersion),
	}
	if input.DisplayName != "" {
		out["displayName"] = input.DisplayName
	}
	if input.DesiredVersion != "" {
		out["desiredVersion"] = input.DesiredVersion
	}
	if input.CloudCoreEndpoint != "" {
		out["cloudCoreEndpoint"] = input.CloudCoreEndpoint
	}
	if input.CredentialSecretRef != nil {
		out["credentialSecretRef"] = map[string]any{"provider": input.CredentialSecretRef.Provider, "scope": input.CredentialSecretRef.Scope, "name": input.CredentialSecretRef.Name, "version": input.CredentialSecretRef.Version}
	}
	return out
}

func ctxFencing(ctx ExecutionContext) string {
	return strconv.FormatInt(ctx.FencingGeneration, 10)
}

func mustProfile(t *testing.T, providerID string) Profile {
	t.Helper()
	profile, err := ProfileForProviderID(providerID)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func mustMarshal(t *testing.T, value any) string {
	if t != nil {
		t.Helper()
	}
	data, err := json.Marshal(value)
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return string(data)
}

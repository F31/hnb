package driver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

func TestHTTPRunnerExecutesConfiguredProvider(t *testing.T) {
	execution := testExecution()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("method = %s", req.Method)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q", got)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer worker-token" {
			t.Errorf("authorization = %q", got)
		}
		var body executeRequest
		decoder := json.NewDecoder(req.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.SchemaVersion != contractVersion {
			t.Errorf("schema version = %q", body.SchemaVersion)
		}
		if body.Execution.TenantID != execution.TenantID ||
			body.Execution.Checkpoint != execution.Checkpoint ||
			body.Execution.IdempotencyKey != execution.IdempotencyKey ||
			body.Execution.ExecutionAttemptID != execution.ExecutionAttemptID ||
			body.Execution.FencingGeneration != "42" {
			t.Errorf("execution metadata = %#v", body.Execution)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(executeResponse{
			SchemaVersion:      contractVersion,
			ExecutionAttemptID: execution.ExecutionAttemptID,
			FencingGeneration:  "42",
			Status:             "succeeded",
			Outputs:            map[string]string{"resource": "deployment/api"},
			Checkpoint:         "applied",
		})
	}))
	defer server.Close()

	runner, err := NewHTTPRunner(testProviderConfig(execution.ProviderID, server.URL), server.Client())
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	outputs, checkpoint, err := runner.Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if outputs["resource"] != "deployment/api" || checkpoint != "applied" {
		t.Fatalf("outputs = %#v, checkpoint = %q", outputs, checkpoint)
	}
}

func TestHTTPRunnerFailsClosedForUnknownProvider(t *testing.T) {
	runner, err := NewHTTPRunner(nil, nil)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	_, _, err = runner.Execute(context.Background(), testExecution())
	if err == nil || !strings.Contains(err.Error(), "is not configured") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPRunnerPreservesFailedCheckpoint(t *testing.T) {
	execution := testExecution()
	providerRetryable := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(executeResponse{
			SchemaVersion:      contractVersion,
			ExecutionAttemptID: execution.ExecutionAttemptID,
			FencingGeneration:  "42",
			Status:             "failed",
			Outputs:            map[string]string{"job": "job-42"},
			Checkpoint:         "job-submitted",
			ErrorCode:          ErrorTargetUnavailable,
			Error:              "target unavailable",
			Retryable:          &providerRetryable,
		})
	}))
	defer server.Close()

	runner, err := NewHTTPRunner(testProviderConfig(execution.ProviderID, server.URL), server.Client())
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	outputs, checkpoint, err := runner.Execute(context.Background(), execution)
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("error = %v", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Retryable {
		t.Fatalf("error = %#v, want client-classified retryable provider error", err)
	}
	if outputs["job"] != "job-42" || checkpoint != "job-submitted" {
		t.Fatalf("outputs = %#v, checkpoint = %q", outputs, checkpoint)
	}
}

func TestHTTPRunnerRejectsInvalidResponses(t *testing.T) {
	execution := testExecution()
	prefix := `{"schemaVersion":"2.0.0","executionAttemptId":"` + execution.ExecutionAttemptID + `","fencingGeneration":"42",`
	tests := []struct {
		name string
		body string
		part string
	}{
		{name: "unknown field", body: prefix + `"status":"succeeded","extra":true}`, part: "unknown field"},
		{name: "trailing JSON", body: prefix + `"status":"succeeded"}{}`, part: "trailing JSON"},
		{name: "wrong version", body: strings.Replace(prefix, "2.0.0", "1.0.0", 1) + `"status":"succeeded"}`, part: "unsupported schema version"},
		{name: "unknown status", body: prefix + `"status":"running"}`, part: "unknown status"},
		{name: "contradictory success", body: prefix + `"status":"succeeded","error":"bad"}`, part: "success with error fields"},
		{name: "noncanonical generation", body: strings.Replace(prefix, `"42"`, `"042"`, 1) + `"status":"succeeded"}`, part: "canonical positive decimal"},
		{name: "wrong attempt", body: strings.Replace(prefix, execution.ExecutionAttemptID, "3815a64f-b377-4fe9-aa9d-5247cf0f03b8", 1) + `"status":"succeeded"}`, part: "mismatched execution attempt"},
		{name: "oversized", body: strings.Repeat("x", maxResponseSize+1), part: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			runner, err := NewHTTPRunner(testProviderConfig(execution.ProviderID, server.URL), server.Client())
			if err != nil {
				t.Fatalf("new runner: %v", err)
			}
			_, _, err = runner.Execute(context.Background(), execution)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("error = %v, want containing %q", err, tt.part)
			}
		})
	}
}

func TestHTTPRunnerOwnsProviderRetryClassification(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		code      ErrorCode
		hint      bool
		retryable bool
	}{
		{name: "bad request stays permanent", status: http.StatusBadRequest, code: ErrorInvalidRequest, hint: true},
		{name: "scope denial stays permanent", status: http.StatusForbidden, code: ErrorScopeDenied, hint: true},
		{name: "resource conflict stays permanent", status: http.StatusConflict, code: ErrorResourceConflict, hint: true},
		{name: "server failure stays transient", status: http.StatusInternalServerError, code: ErrorInternal, hint: false, retryable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution := testExecution()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(executeResponse{
					SchemaVersion:      contractVersion,
					ExecutionAttemptID: execution.ExecutionAttemptID,
					FencingGeneration:  "42",
					Status:             "failed",
					ErrorCode:          tt.code,
					Error:              "provider failure",
					Retryable:          &tt.hint,
				})
			}))
			defer server.Close()
			runner, err := NewHTTPRunner(testProviderConfig(execution.ProviderID, server.URL), server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = runner.Execute(context.Background(), execution)
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Retryable != tt.retryable {
				t.Fatalf("error = %#v, retryable = %v", err, tt.retryable)
			}
		})
	}
}

func TestHTTPRunnerRejectsInvalidExecutionFence(t *testing.T) {
	runner, err := NewHTTPRunner(testProviderConfig("k8s-prod", "http://provider.example/execute"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, execution := range map[string]engine.ExecutionContext{
		"invalid attempt":     {ProviderID: "k8s-prod", ExecutionAttemptID: "not-a-uuid", FencingGeneration: 1},
		"zero generation":     {ProviderID: "k8s-prod", ExecutionAttemptID: "3815a64f-b377-4fe9-aa9d-5247cf0f03b8"},
		"negative generation": {ProviderID: "k8s-prod", ExecutionAttemptID: "3815a64f-b377-4fe9-aa9d-5247cf0f03b8", FencingGeneration: -1},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runner.Execute(context.Background(), execution)
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Code != ErrorInvalidRequest || providerErr.Retryable {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestHTTPRunnerCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}

	execution := testExecution()
	runner, err := NewHTTPRunner(testProviderConfig(execution.ProviderID, "http://provider.example/execute"), client)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := runner.Execute(ctx, execution)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider request was not canceled")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewHTTPRunnerRejectsInvalidEndpoints(t *testing.T) {
	for name, endpoints := range map[string]map[string]ProviderConfig{
		"empty provider":       {"": {Endpoint: "https://provider.example/execute", Audience: "audience", TokenSource: staticTokenSource("token")}},
		"relative URL":         {"provider": {Endpoint: "/execute", Audience: "audience", TokenSource: staticTokenSource("token")}},
		"credentials":          {"provider": {Endpoint: "https://user:pass@provider.example/execute", Audience: "audience", TokenSource: staticTokenSource("token")}},
		"fragment":             {"provider": {Endpoint: "https://provider.example/execute#fragment", Audience: "audience", TokenSource: staticTokenSource("token")}},
		"missing audience":     {"provider": {Endpoint: "https://provider.example/execute", TokenSource: staticTokenSource("token")}},
		"missing token source": {"provider": {Endpoint: "https://provider.example/execute", Audience: "audience"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewHTTPRunner(endpoints, nil); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHTTPRunnerFailsClosedWhenTokenSourceFails(t *testing.T) {
	execution := testExecution()
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected call")
	})}
	providers := testProviderConfig(execution.ProviderID, "https://provider.example/execute")
	providers[execution.ProviderID] = ProviderConfig{Endpoint: "https://provider.example/execute", Audience: "hnb-kubernetes-provider", TokenSource: failingTokenSource{}}
	runner, err := NewHTTPRunner(providers, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.Execute(context.Background(), execution); err == nil || called {
		t.Fatalf("error = %v, HTTP called = %v", err, called)
	}
}

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

type failingTokenSource struct{}

func (failingTokenSource) Token(context.Context) (string, error) {
	return "", errors.New("token unavailable")
}

func testProviderConfig(providerID, endpoint string) map[string]ProviderConfig {
	return map[string]ProviderConfig{providerID: {Endpoint: endpoint, Audience: "hnb-kubernetes-provider", TokenSource: staticTokenSource("worker-token")}}
}

func testExecution() engine.ExecutionContext {
	return engine.ExecutionContext{
		StepID:             "c33a3b57-ae74-4ae5-a32f-6688a9747db5",
		OperationID:        "8d466b02-7cf5-40e0-a49b-c151c7fcf60a",
		TenantID:           "tenant-a",
		ProjectID:          "project-a",
		EnvironmentID:      "production",
		StepType:           "deploy",
		Inputs:             map[string]any{"manifest": "oci://registry.example/app@sha256:abc"},
		ProviderID:         "k8s-prod",
		Checkpoint:         "validated",
		IdempotencyKey:     "step-key",
		ExecutionAttemptID: "f01971cb-c0f3-42e7-bd46-9f31f945c261",
		FencingGeneration:  42,
	}
}

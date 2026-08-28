package driver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	lifecycleVersion = "1.2.3"
	lifecycleDigest  = "sha256:" + "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
)

func lifecycleProviderConfig(providerID, endpoint string) ProviderConfig {
	return ProviderConfig{
		Endpoint:         endpoint,
		Audience:         "hnb-runtime-provider",
		TokenSource:      staticTokenSource("worker-token"),
		ProtocolVersion:  "2.0.0",
		ProviderVersion:  lifecycleVersion,
		ProviderDigest:   lifecycleDigest,
		RequiredProvider: providerID,
	}
}

func TestHTTPRunnerRoutesPinnedLifecycleProvider(t *testing.T) {
	execution := testExecution()
	execution.ProviderID = "runtime-target.lifecycle.kubernetes"
	execution.ProviderVersion = lifecycleVersion
	execution.ProviderDigest = lifecycleDigest
	var captured executeRequest
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		atomic.AddInt32(&calls, 1)
		if err := json.NewDecoder(req.Body).Decode(&captured); err != nil {
			t.Errorf("decode: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(executeResponse{
			SchemaVersion:      contractVersion,
			ExecutionAttemptID: execution.ExecutionAttemptID,
			IdempotencyKey:     execution.IdempotencyKey,
			ProviderVersion:    lifecycleVersion,
			ProviderDigest:     lifecycleDigest,
			FencingGeneration:  strconv.FormatInt(execution.FencingGeneration, 10),
			Status:             "succeeded",
			Outputs:            map[string]string{"resource": "kubernetes/deployment-api"},
			Checkpoint:         "applied",
		})
	}))
	defer server.Close()
	providers := map[string]ProviderConfig{lifecycleProviderKubernetes: lifecycleProviderConfig(lifecycleProviderKubernetes, server.URL)}
	runner, err := NewHTTPRunner(providers, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.Execute(context.Background(), execution); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if captured.Execution.ProviderProtocolVersion != contractVersion {
		t.Fatalf("provider protocol version = %q", captured.Execution.ProviderProtocolVersion)
	}
	if captured.Execution.ProviderVersion != lifecycleVersion || captured.Execution.ProviderDigest != lifecycleDigest {
		t.Fatalf("execution did not carry pinned providerVersion/digest: %+v", captured.Execution)
	}
	if captured.Execution.IdempotencyKey != execution.IdempotencyKey {
		t.Fatalf("idempotency key missing")
	}
}

func TestHTTPRunnerRejectsLifecycleVersionMismatch(t *testing.T) {
	execution := testExecution()
	execution.ProviderID = lifecycleProviderKubernetes
	execution.ProviderVersion = "9.9.9"
	execution.ProviderDigest = lifecycleDigest
	providers := map[string]ProviderConfig{lifecycleProviderKubernetes: lifecycleProviderConfig(lifecycleProviderKubernetes, "http://provider.example/execute")}
	runner, err := NewHTTPRunner(providers, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Execute(context.Background(), execution)
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || !strings.Contains(providerErr.Message, "pinned version") {
		t.Fatalf("expected pinned version error, got %v", err)
	}
}

func TestHTTPRunnerRoutesPinnedStorageDriverProvider(t *testing.T) {
	execution := testExecution()
	execution.StepType = "storage.driver.upgrade"
	execution.ProviderID = "storage.example/driver"
	execution.ProviderVersion = lifecycleVersion
	execution.ProviderDigest = lifecycleDigest
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(executeResponse{SchemaVersion: contractVersion, ExecutionAttemptID: execution.ExecutionAttemptID, IdempotencyKey: execution.IdempotencyKey, ProviderVersion: lifecycleVersion, ProviderDigest: lifecycleDigest, FencingGeneration: strconv.FormatInt(execution.FencingGeneration, 10), Status: "succeeded"})
	}))
	defer server.Close()
	runner, err := NewHTTPRunner(map[string]ProviderConfig{execution.ProviderID: lifecycleProviderConfig(execution.ProviderID, server.URL)}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.Execute(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("storage provider was not called exactly once")
	}
	execution.ProviderDigest = "sha256:" + strings.Repeat("b", 64)
	if _, _, err := runner.Execute(context.Background(), execution); err == nil || atomic.LoadInt32(&calls) != 1 {
		t.Fatal("unpinned storage route reached provider")
	}
}

func TestHTTPRunnerRejectsLifecycleEchoMismatch(t *testing.T) {
	cases := map[string]struct {
		mutate   func(*executeResponse)
		wantPart string
	}{
		"wrong schema": {
			mutate:   func(r *executeResponse) { r.SchemaVersion = "1.0.0" },
			wantPart: "unsupported schema version",
		},
		"wrong attempt": {
			mutate:   func(r *executeResponse) { r.ExecutionAttemptID = "11111111-1111-1111-1111-111111111111" },
			wantPart: "mismatched execution attempt",
		},
		"wrong idempotency": {
			mutate:   func(r *executeResponse) { r.IdempotencyKey = "different" },
			wantPart: "mismatched idempotency key",
		},
		"wrong providerVersion": {
			mutate:   func(r *executeResponse) { r.ProviderVersion = "9.9.9" },
			wantPart: "mismatched providerVersion",
		},
		"wrong providerDigest": {
			mutate:   func(r *executeResponse) { r.ProviderDigest = "sha256:" + strings.Repeat("b", 64) },
			wantPart: "mismatched providerDigest",
		},
		"wrong fencing": {
			mutate:   func(r *executeResponse) { r.FencingGeneration = "7" },
			wantPart: "mismatched fencing generation",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			execution := testExecution()
			execution.ProviderID = lifecycleProviderKubernetes
			execution.ProviderVersion = lifecycleVersion
			execution.ProviderDigest = lifecycleDigest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := executeResponse{
					SchemaVersion:      contractVersion,
					ExecutionAttemptID: execution.ExecutionAttemptID,
					IdempotencyKey:     execution.IdempotencyKey,
					ProviderVersion:    lifecycleVersion,
					ProviderDigest:     lifecycleDigest,
					FencingGeneration:  strconv.FormatInt(execution.FencingGeneration, 10),
					Status:             "succeeded",
					Outputs:            map[string]string{"resource": "x"},
				}
				tc.mutate(&resp)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()
			providers := map[string]ProviderConfig{lifecycleProviderKubernetes: lifecycleProviderConfig(lifecycleProviderKubernetes, server.URL)}
			runner, err := NewHTTPRunner(providers, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = runner.Execute(context.Background(), execution)
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || !strings.Contains(providerErr.Message, tc.wantPart) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantPart)
			}
		})
	}
}

func TestHTTPRunnerReplaysWithCurrentAttempt(t *testing.T) {
	execution := testExecution()
	execution.ProviderID = lifecycleProviderKubernetes
	execution.ProviderVersion = lifecycleVersion
	execution.ProviderDigest = lifecycleDigest
	execution.Checkpoint = "applied"
	execution.FencingGeneration = 9
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(executeResponse{
			SchemaVersion:      contractVersion,
			ExecutionAttemptID: execution.ExecutionAttemptID,
			IdempotencyKey:     execution.IdempotencyKey,
			ProviderVersion:    lifecycleVersion,
			ProviderDigest:     lifecycleDigest,
			FencingGeneration:  strconv.FormatInt(execution.FencingGeneration, 10),
			Status:             "succeeded",
			Outputs:            map[string]string{"resource": "x"},
			Checkpoint:         "resume",
		})
	}))
	defer server.Close()
	providers := map[string]ProviderConfig{lifecycleProviderKubernetes: lifecycleProviderConfig(lifecycleProviderKubernetes, server.URL)}
	runner, err := NewHTTPRunner(providers, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	outputs, checkpoint, err := runner.Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if outputs["resource"] != "x" || checkpoint != "resume" {
		t.Fatalf("outputs=%v checkpoint=%q", outputs, checkpoint)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected exactly one execution, got %d", calls)
	}
}

func TestNewHTTPRunnerRejectsUnpinnedLifecycleProvider(t *testing.T) {
	_, err := NewHTTPRunner(map[string]ProviderConfig{
		lifecycleProviderKubernetes: {
			Endpoint: "http://provider.example/execute", Audience: "audience", TokenSource: staticTokenSource("t"),
			ProtocolVersion: "2.0.0",
		},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "requiredProvider") {
		t.Fatalf("expected requiredProvider error, got %v", err)
	}
}

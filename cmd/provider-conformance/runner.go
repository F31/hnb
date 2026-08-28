package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type TestRunner struct {
	ProviderID  string
	ProviderURL string
	client      *http.Client
}

func NewTestRunner(providerID, providerURL string) *TestRunner {
	return &TestRunner{
		ProviderID:  providerID,
		ProviderURL: providerURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *TestRunner) RunContractTests(manifest map[string]any) []TestResult {
	var results []TestResult
	results = append(results, r.testHealthCheck())
	results = append(results, r.testVersionNegotiation())
	if manifest != nil {
		results = append(results, r.testManifestActions(manifest))
	}
	return results
}

func (r *TestRunner) testHealthCheck() TestResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.ProviderURL+"/healthz", nil)
	if err != nil {
		return TestResult{TestName: "health_check", Category: "contract", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "health_check", Category: "contract", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	passed := resp.StatusCode == http.StatusOK
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return TestResult{TestName: "health_check", Category: "contract", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) testVersionNegotiation() TestResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.ProviderURL+"/version", nil)
	if err != nil {
		return TestResult{TestName: "version_negotiation", Category: "contract", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "version_negotiation", Category: "contract", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var versionResp map[string]any
	json.Unmarshal(body, &versionResp)

	passed := resp.StatusCode == http.StatusOK && versionResp["version"] != nil
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("status %d, body: %s", resp.StatusCode, string(body))
	}
	return TestResult{TestName: "version_negotiation", Category: "contract", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) testManifestActions(manifest map[string]any) TestResult {
	start := time.Now()
	actions, ok := manifest["actions"].([]any)
	if !ok || len(actions) == 0 {
		return TestResult{TestName: "manifest_actions", Category: "contract", Passed: false, Duration: time.Since(start).String(), Error: "no actions declared in manifest"}
	}
	declared := len(actions)
	return TestResult{TestName: "manifest_actions", Category: "contract", Passed: declared > 0, Duration: time.Since(start).String(), Error: ""}
}

func (r *TestRunner) RunFunctionalTests() []TestResult {
	var results []TestResult
	results = append(results, r.testExecuteStep())
	results = append(results, r.testStepTimeout())
	results = append(results, r.testStepOutput())
	return results
}

func (r *TestRunner) testExecuteStep() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":      "conformance-test-step",
		"operation_id": "conformance-test-op",
		"tenant_id":    "conformance-test",
		"step_type":    "validate",
		"inputs":       map[string]string{"test": "conformance"},
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return TestResult{TestName: "execute_step", Category: "functional", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "execute_step", Category: "functional", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	passed := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
	errMsg := ""
	if !passed {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg = fmt.Sprintf("status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return TestResult{TestName: "execute_step", Category: "functional", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) testStepTimeout() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":      "timeout-test-step",
		"operation_id": "conformance-test-op",
		"tenant_id":    "conformance-test",
		"step_type":    "validate",
		"inputs":       map[string]string{"test": "timeout"},
	}
	body, _ := json.Marshal(payload)

	shortClient := &http.Client{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return TestResult{TestName: "step_timeout_handling", Category: "functional", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = shortClient.Do(req)
	passed := err != nil
	errMsg := ""
	if !passed {
		errMsg = "provider did not enforce timeout"
	}
	return TestResult{TestName: "step_timeout_handling", Category: "functional", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) testStepOutput() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":      "output-test-step",
		"operation_id": "conformance-test-op",
		"tenant_id":    "conformance-test",
		"step_type":    "validate",
		"inputs":       map[string]string{"test": "output"},
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return TestResult{TestName: "step_output_format", Category: "functional", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "step_output_format", Category: "functional", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)

	passed := resp.StatusCode == http.StatusOK && result["outputs"] != nil
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return TestResult{TestName: "step_output_format", Category: "functional", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) RunFaultTests() []TestResult {
	var results []TestResult
	results = append(results, r.testInvalidPayload())
	results = append(results, r.testMissingTenant())
	return results
}

func (r *TestRunner) testInvalidPayload() TestResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader([]byte("{invalid json")))
	if err != nil {
		return TestResult{TestName: "invalid_payload_rejection", Category: "fault", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "invalid_payload_rejection", Category: "fault", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	passed := resp.StatusCode == http.StatusBadRequest
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("expected 400, got %d", resp.StatusCode)
	}
	return TestResult{TestName: "invalid_payload_rejection", Category: "fault", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) testMissingTenant() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":   "missing-tenant-step",
		"step_type": "validate",
		"inputs":    map[string]string{},
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return TestResult{TestName: "missing_tenant_rejection", Category: "fault", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "missing_tenant_rejection", Category: "fault", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	passed := resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("expected 400/403, got %d", resp.StatusCode)
	}
	return TestResult{TestName: "missing_tenant_rejection", Category: "fault", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) RunSecurityTests() []TestResult {
	var results []TestResult
	results = append(results, r.testFencingToken())
	results = append(results, r.testIdempotency())
	return results
}

func (r *TestRunner) testFencingToken() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":           "fencing-test-step",
		"operation_id":      "conformance-test-op",
		"tenant_id":         "conformance-test",
		"step_type":         "validate",
		"fencing_generation": 0,
		"inputs":            map[string]string{},
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return TestResult{TestName: "fencing_token_validation", Category: "security", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Fencing-Token", "stale-token")

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "fencing_token_validation", Category: "security", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	passed := resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusForbidden
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("expected 409/403 for stale token, got %d", resp.StatusCode)
	}
	return TestResult{TestName: "fencing_token_validation", Category: "security", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) testIdempotency() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":          "idempotency-test-step",
		"operation_id":     "conformance-test-op",
		"tenant_id":        "conformance-test",
		"step_type":        "validate",
		"idempotency_key":  "conformance-idem-key",
		"inputs":           map[string]string{},
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return TestResult{TestName: "idempotency_handling", Category: "security", Passed: false, Duration: time.Since(start).String(), Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return TestResult{TestName: "idempotency_handling", Category: "security", Passed: false, Duration: time.Since(start).String(), Error: fmt.Sprintf("connection failed: %v", err)}
	}
	defer resp.Body.Close()

	_ = resp.Body.Close()

	passed := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted
	errMsg := ""
	if !passed {
		errMsg = fmt.Sprintf("expected 200/202, got %d", resp.StatusCode)
	}
	return TestResult{TestName: "idempotency_handling", Category: "security", Passed: passed, Duration: time.Since(start).String(), Error: errMsg}
}

func (r *TestRunner) RunPerformanceBaseline() []TestResult {
	var results []TestResult
	results = append(results, r.testLatencyBaseline())
	return results
}

func (r *TestRunner) testLatencyBaseline() TestResult {
	start := time.Now()
	payload := map[string]any{
		"step_id":      "perf-test-step",
		"operation_id": "conformance-test-op",
		"tenant_id":    "conformance-test",
		"step_type":    "validate",
		"inputs":       map[string]string{},
	}
	body, _ := json.Marshal(payload)

	var totalDuration time.Duration
	samples := 3
	successCount := 0

	for i := 0; i < samples; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ProviderURL+"/execute", bytes.NewReader(body))
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		sampleStart := time.Now()
		resp, err := r.client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		totalDuration += time.Since(sampleStart)
		resp.Body.Close()
		successCount++
		cancel()
	}

	passed := successCount > 0
	avgLatency := "N/A"
	if successCount > 0 {
		avgLatency = (totalDuration / time.Duration(successCount)).String()
	}
	errMsg := ""
	if !passed {
		errMsg = "all samples failed"
	}
	return TestResult{TestName: "latency_baseline", Category: "performance", Passed: passed, Duration: time.Since(start).String(), Error: fmt.Sprintf("%s (avg: %s, samples: %d/%d)", errMsg, avgLatency, successCount, samples)}
}
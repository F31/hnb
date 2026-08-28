package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteMappedUpstreamProblemPreservesStaleFields(t *testing.T) {
	upstream := map[string]any{
		"type": "https://hnb.cloud/problems/stale-confirmation-required", "title": "Conflict",
		"status": 409, "detail": "The target observation is stale and requires explicit confirmation.",
		"code": "STALE_CONFIRMATION_REQUIRED",
		"correlationId": "018f6c2a-4a64-7b58-9cc3-9f70462f36c1", "traceId": "ab12",
		"confirmation": "hmac-signature-with-at-least-16-chars",
		"targetId":       "7f1c2b9e-4a6d-4c1e-9f2b-3a5c7d8e9f01",
		"action":         "upgrade",
		"lastKnownStateAt": "2026-08-20T09:00:00Z",
		"lifecycleState":   "ACTIVE",
		"healthState":      "HEALTHY",
		"connectivityState": "CONNECTED",
		"policyOutcome":     "require_approval",
	}
	body, err := json.Marshal(upstream)
	if err != nil {
		t.Fatal(err)
	}

	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(string(body))))
	rec := httptest.NewRecorder()
	writeMappedUpstreamProblem(rec, req, http.StatusConflict, "application/problem+json", body)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type = %q", ct)
	}
	var out consoleProblem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Code != "STALE_CONFIRMATION_REQUIRED" {
		t.Fatalf("code = %q", out.Code)
	}
	if out.Confirmation != upstream["confirmation"] {
		t.Fatalf("confirmation not preserved: %q", out.Confirmation)
	}
	if out.TargetID != "7f1c2b9e-4a6d-4c1e-9f2b-3a5c7d8e9f01" || out.Action != "upgrade" {
		t.Fatalf("target/action not preserved: %q %q", out.TargetID, out.Action)
	}
	if out.PolicyOutcome != "require_approval" || out.LifecycleState != "ACTIVE" || out.HealthState != "HEALTHY" || out.ConnectivityState != "CONNECTED" {
		t.Fatalf("state fields not preserved: %+v", out)
	}
	if out.LastKnownStateAt != "2026-08-20T09:00:00Z" {
		t.Fatalf("lastKnownStateAt not preserved: %q", out.LastKnownStateAt)
	}
	if out.CorrelationID != "018f6c2a-4a64-7b58-9cc3-9f70462f36c1" {
		t.Fatalf("correlationId = %q", out.CorrelationID)
	}
}

func TestWriteMappedUpstreamProblemRejectsNonProblemContentType(t *testing.T) {
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader("{}")))
	rec := httptest.NewRecorder()
	writeMappedUpstreamProblem(rec, req, http.StatusConflict, "application/json", []byte(`{"code":"STALE_CONFIRMATION_REQUIRED"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (expected 503 upstream-unavailable for non-problem body), body %s", rec.Code, rec.Body)
	}
}

func TestWriteMappedUpstreamProblemDropsConfirmationWhenCodeMismatch(t *testing.T) {
	// A 409 with a TARGET_VERSION_CONFLICT code must not leak a confirmation token.
	upstream := map[string]any{
		"type": "https://hnb.cloud/problems/target-version-conflict", "title": "Conflict",
		"status": 409, "detail": "The runtime target version changed.",
		"code": "TARGET_VERSION_CONFLICT",
		"confirmation": "hmac-signature-with-at-least-16-chars",
		"correlationId": "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
	}
	body, _ := json.Marshal(upstream)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(string(body))))
	rec := httptest.NewRecorder()
	writeMappedUpstreamProblem(rec, req, http.StatusConflict, "application/problem+json", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	var out consoleProblem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != "TARGET_VERSION_CONFLICT" {
		t.Fatalf("code = %q", out.Code)
	}
	if out.Confirmation != "" {
		t.Fatalf("confirmation leaked on non-STALE 409: %q", out.Confirmation)
	}
}

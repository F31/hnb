package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func operationPlatformResponse(operationID string) string {
	return `{"id":"` + operationID + `","intentId":"11111111-1111-4111-8111-111111111111","tenantId":"tenant-a","planId":"plan-1","operationType":"upgrade","status":"queued","initiatedBy":"subject-a","totalSteps":2,"completedSteps":0,"failedSteps":0,"targetClusterIds":["515eba09-0a41-5b92-b972-69af1f0f655c"],"createdAt":"2026-08-01T00:00:00Z","lastObservedAt":"2026-08-01T00:00:00Z","steps":[{"id":"step-1","name":"register","status":"queued"},{"id":"step-2","name":"verify","status":"pending"}]}`
}

func operationPlatformListResponse() string {
	return `{"operations":[{"id":"356886f1-1b73-49a8-9275-1a85773cf973","tenantId":"tenant-a","operationType":"upgrade","status":"queued","totalSteps":2,"completedSteps":0,"failedSteps":0,"initiatedBy":"subject-a","targetClusterIds":["515eba09-0a41-5b92-b972-69af1f0f655c"],"createdAt":"2026-08-01T00:00:00Z","lastObservedAt":"2026-08-01T00:00:00Z"}],"total":1,"limit":20,"offset":0}`
}

func TestResourceClusterHandlerForwardsOperationList(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/operations" || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		payload, err := base64.RawURLEncoding.DecodeString(strings.Split(strings.TrimPrefix(auth, "Bearer "), ".")[1])
		if err != nil {
			t.Fatal(err)
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			t.Fatal(err)
		}
		scope, _ := claims["scope"].(map[string]any)
		if scope["resourceKind"] != "operation" || claims["action"] != "list" || scope["resourceId"] != nil {
			t.Fatalf("operation list delegation claims = %v", claims)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(operationPlatformListResponse()))
	}))
	defer platform.Close()

	h := newDelegatingResourceHandler(t, platform.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/operations?page=1&pageSize=20", nil)
	req = withTrusted(req)
	rec := httptest.NewRecorder()
	h.ListOperations(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp consoleOperationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.APIVersion != "ui.hnb.io/v1" || len(resp.Items) != 1 {
		t.Fatalf("list response = %+v", resp)
	}
	item := resp.Items[0]
	if item.OperationID != "356886f1-1b73-49a8-9275-1a85773cf973" || item.Status != "queued" ||
		item.TargetID != "515eba09-0a41-5b92-b972-69af1f0f655c" || item.TargetKind != "KubernetesTarget" {
		t.Fatalf("item = %+v", item)
	}
	if !resp.Pagination.ExactTotal || resp.Pagination.Total != 1 {
		t.Fatalf("pagination = %+v", resp.Pagination)
	}
}

func TestResourceClusterHandlerForwardsOperationDetail(t *testing.T) {
	const opID = "356886f1-1b73-49a8-9275-1a85773cf973"
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/operations/"+opID || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(operationPlatformResponse(opID)))
	}))
	defer platform.Close()

	h := newDelegatingResourceHandler(t, platform.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+opID, nil)
	req = withTrusted(req)
	rec := httptest.NewRecorder()
	h.GetOperation(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp consoleOperationDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	d := resp.Data
	if d.ExecutionPlanID != "plan-1" || d.OperationID != opID || len(d.Steps) != 2 || len(d.AllowedActions) != 1 {
		t.Fatalf("detail = %+v", d)
	}
	if d.AllowedActions[0] != "cancel" {
		t.Fatalf("allowed actions = %v", d.AllowedActions)
	}
	if d.Links.Operation != "/operations/"+opID || d.Links.Target != "/resource/clusters/515eba09-0a41-5b92-b972-69af1f0f655c" {
		t.Fatalf("links = %+v", d.Links)
	}
}

func TestResourceClusterHandlerForwardsOperationApprove(t *testing.T) {
	const opID = "356886f1-1b73-49a8-9275-1a85773cf973"
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/operations/"+opID+"/approve" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(operationPlatformResponse(opID)))
	}))
	defer platform.Close()

	h := newDelegatingResourceHandler(t, platform.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/"+opID+"/actions/approve", strings.NewReader(`{"reason":"ok"}`))
	req = withTrusted(req)
	rec := httptest.NewRecorder()
	h.OperationApprove(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResourceClusterHandlerOperationRejectsBadID(t *testing.T) {
	h := newDelegatingResourceHandler(t, "http://unused")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/not-a-uuid", nil)
	req = withTrusted(req)
	rec := httptest.NewRecorder()
	h.GetOperation(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

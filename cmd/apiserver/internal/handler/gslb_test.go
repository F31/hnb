package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gslbapp "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	"github.com/F31/hnb/pkg/gslb"
	"github.com/F31/hnb/pkg/iam"
)

// handlerStubRepo 是 handler 测试用的最小仓库。
type handlerStubRepo struct {
	service gslbapp.Service
	created []gslbapp.SwitchRequest
}

func (s *handlerStubRepo) GetService(_ context.Context, id, tenantID string) (gslbapp.Service, bool, error) {
	if s.service.ID != id || s.service.TenantID != tenantID {
		return gslbapp.Service{}, false, nil
	}
	return s.service, true, nil
}

func (s *handlerStubRepo) GetPoolMemberClusterIDs(_ context.Context, _ string) ([]string, error) {
	return []string{"cluster-a", "cluster-b"}, nil
}

func (s *handlerStubRepo) GetSwitchRequestByKey(context.Context, string, string, string) (gslbapp.SwitchRequest, bool, error) {
	return gslbapp.SwitchRequest{}, false, nil
}

func (s *handlerStubRepo) GetSwitchRequest(context.Context, string, string) (gslbapp.SwitchRequest, bool, error) {
	return gslbapp.SwitchRequest{}, false, nil
}

func (s *handlerStubRepo) CreateSwitchRequest(_ context.Context, req gslbapp.SwitchRequest, _ *gslbapp.DrillReport, _ []gslbapp.OutboxEvent) error {
	s.created = append(s.created, req)
	return nil
}

func (s *handlerStubRepo) UpdateSwitchRequestStatus(context.Context, string, string, map[string]any, []gslbapp.OutboxEvent) error {
	return nil
}

func (s *handlerStubRepo) ListDrillReports(context.Context, string, string) ([]gslbapp.DrillReport, error) {
	return nil, nil
}

func (s *handlerStubRepo) ListReadModels(_ context.Context, tenantID string) ([]gslbapp.ReadModel, error) {
	return []gslbapp.ReadModel{{ServiceID: s.service.ID, TenantID: tenantID, Domain: s.service.Domain, LifecycleState: "Active"}}, nil
}

func (s *handlerStubRepo) GetReadModel(_ context.Context, id, tenantID string) (gslbapp.ReadModel, bool, error) {
	if id != s.service.ID || tenantID != s.service.TenantID {
		return gslbapp.ReadModel{}, false, nil
	}
	return gslbapp.ReadModel{ServiceID: id, TenantID: tenantID, Domain: s.service.Domain, LifecycleState: "Active"}, true, nil
}

func newGSLBHandlerForTest() (*GSLBHandler, *handlerStubRepo) {
	repo := &handlerStubRepo{service: gslbapp.Service{
		ID: "00000000-0000-4000-8000-0000000000a1", TenantID: "tenant-a",
		Name: "api-gateway", Domain: "api.hnb.cloud", RoutingMode: "dns",
		LifecycleState: "Active", RequireApproval: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}}
	return NewGSLBHandler(gslbapp.NewService(repo)), repo
}

func gslbTrusted(permissions ...iam.ScopedPermission) iam.TrustedContext {
	return iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", ScopedPermissions: permissions}
}

func gslbIntentBody() string {
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1", "kind": "gslb.failover",
		"serviceId": "00000000-0000-4000-8000-0000000000a1", "tenantId": "tenant-a",
		"targetPoolId": "00000000-0000-4000-8000-0000000000b2",
		"metadata": map[string]any{
			"idempotencyKey": "k-handler-1", "correlationId": "00000000-0000-4000-8000-0000000000c1",
		},
	})
	return string(body)
}

func TestGSLBSubmitIntentUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000a1/intents", strings.NewReader(gslbIntentBody()))
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000a1")
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.SubmitIntent(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBSubmitIntentForbiddenWithoutExecute(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000a1/intents", strings.NewReader(gslbIntentBody()))
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000a1")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted()))
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.SubmitIntent(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBSubmitIntentOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000a1/intents", strings.NewReader(gslbIntentBody()))
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000a1")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(iam.ScopedPermission{
		TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionExecute,
	})))
	recorder := httptest.NewRecorder()

	h, repo := newGSLBHandlerForTest()
	h.SubmitIntent(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var created gslbapp.SwitchRequest
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Status != gslbapp.StatusPendingApproval {
		t.Fatalf("status = %s", created.Status)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d", len(repo.created))
	}
	// 计划快照必须是合法 gslb 计划
	var plan gslb.Plan
	if err := json.Unmarshal(created.PlanSnapshot, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("plan has no steps")
	}
}

func TestGSLBSubmitIntentRejectsServiceMismatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000ff/intents", strings.NewReader(gslbIntentBody()))
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000ff")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(iam.ScopedPermission{
		TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionExecute,
	})))
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.SubmitIntent(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBListServices(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gslb/services", nil)
	req = req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(iam.ScopedPermission{
		TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionList,
	})))
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.ListServices(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body struct {
		Items []gslbapp.ReadModel `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("items=%d total=%d", len(body.Items), body.Total)
	}
}

func TestGSLBGetServiceNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000ff", nil)
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000ff")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(iam.ScopedPermission{
		TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionRead,
	})))
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.GetService(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

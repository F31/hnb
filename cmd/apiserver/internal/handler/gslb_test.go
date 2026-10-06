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
	service        gslbapp.Service
	pools          []gslbapp.Pool
	healthyPools   []string
	switchRequests []gslbapp.SwitchRequest
	created        []gslbapp.SwitchRequest
	knownClusters  map[string]bool
	createdService bool
	updatedCalls   int
	replacedPools  []gslbapp.Pool
	replaceCalls   int
	inflight       bool
	deleted        bool
}

func (s *handlerStubRepo) GetService(_ context.Context, id, tenantID string) (gslbapp.Service, bool, error) {
	if s.service.ID != id || s.service.TenantID != tenantID {
		return gslbapp.Service{}, false, nil
	}
	return s.service, true, nil
}

func (s *handlerStubRepo) GetServiceByDomain(_ context.Context, tenantID, domain string) (gslbapp.Service, bool, error) {
	if s.service.TenantID != tenantID || s.service.Domain != domain {
		return gslbapp.Service{}, false, nil
	}
	return s.service, true, nil
}

func (s *handlerStubRepo) ExistingTenantClusters(_ context.Context, tenantID string, clusterIDs []string) (map[string]bool, error) {
	existing := map[string]bool{}
	for _, id := range clusterIDs {
		if s.knownClusters == nil || s.knownClusters[id] {
			existing[id] = true
		}
	}
	return existing, nil
}

func (s *handlerStubRepo) CreateService(_ context.Context, svc gslbapp.Service, _ gslbapp.HealthCheck, pools []gslbapp.Pool) error {
	s.service = svc
	s.pools = pools
	s.createdService = true
	return nil
}

func (s *handlerStubRepo) UpdateService(_ context.Context, svc gslbapp.Service, expectedUpdatedAt time.Time) (gslbapp.Service, bool, error) {
	if !expectedUpdatedAt.Equal(s.service.UpdatedAt) {
		return gslbapp.Service{}, false, nil
	}
	svc.UpdatedAt = svc.UpdatedAt.Add(time.Second)
	s.service = svc
	s.updatedCalls++
	return svc, true, nil
}

func (s *handlerStubRepo) ReplaceServicePools(_ context.Context, serviceID, tenantID string, pools []gslbapp.Pool, newActivePoolID string, expectedUpdatedAt time.Time) (bool, error) {
	if serviceID != s.service.ID || tenantID != s.service.TenantID || !expectedUpdatedAt.Equal(s.service.UpdatedAt) {
		return false, nil
	}
	s.pools = pools
	s.service.ActivePoolID = newActivePoolID
	s.replacedPools = pools
	s.replaceCalls++
	return true, nil
}

func (s *handlerStubRepo) DeleteService(_ context.Context, id, tenantID string) (bool, error) {
	if id != s.service.ID || tenantID != s.service.TenantID {
		return false, nil
	}
	s.deleted = true
	return true, nil
}

func (s *handlerStubRepo) HasInflightSwitchRequests(context.Context, string, string) (bool, error) {
	return s.inflight, nil
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

func (s *handlerStubRepo) ListPools(_ context.Context, serviceID, tenantID string) ([]gslbapp.Pool, error) {
	if serviceID != s.service.ID || tenantID != s.service.TenantID {
		return nil, nil
	}
	return s.pools, nil
}

func (s *handlerStubRepo) ListSwitchRequests(_ context.Context, tenantID, serviceID, status string) ([]gslbapp.SwitchRequest, error) {
	if serviceID != s.service.ID || tenantID != s.service.TenantID {
		return nil, nil
	}
	var out []gslbapp.SwitchRequest
	for _, req := range s.switchRequests {
		if status == "" || req.Status == status {
			out = append(out, req)
		}
	}
	return out, nil
}

func (s *handlerStubRepo) CreateSwitchRequest(_ context.Context, req gslbapp.SwitchRequest, _ *gslbapp.DrillReport, _ []gslbapp.OutboxEvent) error {
	s.created = append(s.created, req)
	return nil
}

func (s *handlerStubRepo) UpdateSwitchRequestStatus(context.Context, string, string, map[string]any, []gslbapp.OutboxEvent) error {
	return nil
}

func (s *handlerStubRepo) CompleteSwitchRequest(context.Context, string) error {
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
	return gslbapp.ReadModel{ServiceID: id, TenantID: tenantID, Domain: s.service.Domain,
		LifecycleState: "Active", HealthyPools: s.healthyPools}, true, nil
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

const (
	gslbTestPoolActive = "00000000-0000-4000-8000-0000000000b1"
	gslbTestPoolBackup = "00000000-0000-4000-8000-0000000000b2"
)

func newGSLBHandlerWithPools() (*GSLBHandler, *handlerStubRepo) {
	h, repo := newGSLBHandlerForTest()
	repo.service.ActivePoolID = gslbTestPoolActive
	repo.healthyPools = []string{gslbTestPoolActive}
	repo.pools = []gslbapp.Pool{
		{ID: gslbTestPoolActive, Name: "active", Priority: 0, Members: []gslbapp.PoolMember{{ClusterID: "test-cluster-a", Weight: 100}}},
		{ID: gslbTestPoolBackup, Name: "backup", Priority: 1, Members: []gslbapp.PoolMember{{ClusterID: "test-cluster-b", Weight: 100}}},
	}
	return h, repo
}

func gslbPoolsRequest(serviceID string, permissions ...iam.ScopedPermission) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gslb/services/"+serviceID+"/pools", nil)
	req.SetPathValue("id", serviceID)
	return req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(permissions...)))
}

func TestGSLBListPools(t *testing.T) {
	h, _ := newGSLBHandlerWithPools()
	recorder := httptest.NewRecorder()
	h.ListPools(recorder, gslbPoolsRequest("00000000-0000-4000-8000-0000000000a1", iam.ScopedPermission{
		TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionRead,
	}))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body []gslbapp.Pool
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 {
		t.Fatalf("items=%d", len(body))
	}
	active, backup := body[0], body[1]
	if active.ID != gslbTestPoolActive || !active.IsActive || !active.Healthy {
		t.Fatalf("active pool = %+v", active)
	}
	if backup.IsActive || backup.Healthy {
		t.Fatalf("backup pool = %+v", backup)
	}
	if len(active.Members) != 1 || active.Members[0].ClusterID != "test-cluster-a" || active.Members[0].Weight != 100 {
		t.Fatalf("members = %+v", active.Members)
	}
}

func TestGSLBListPoolsNotFoundCrossTenant(t *testing.T) {
	h, _ := newGSLBHandlerWithPools()
	recorder := httptest.NewRecorder()
	h.ListPools(recorder, gslbPoolsRequest("00000000-0000-4000-8000-0000000000ff", iam.ScopedPermission{
		TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionRead,
	}))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBListPoolsUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000a1/pools", nil)
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000a1")
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.ListPools(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func newGSLBHandlerWithSwitchRequests() *GSLBHandler {
	h, repo := newGSLBHandlerForTest()
	repo.switchRequests = []gslbapp.SwitchRequest{{
		ID: "00000000-0000-4000-8000-0000000000c1", TenantID: "tenant-a",
		ServiceID: repo.service.ID, IntentKind: "gslb.failover",
		Status: gslbapp.StatusPendingApproval, RequireApproval: true,
		ActorID: "subject-a", Reason: "dc outage",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}}
	return h
}

func gslbSwitchListRequest(rawURL string, permissions ...iam.ScopedPermission) *http.Request {
	req := httptest.NewRequest(http.MethodGet, rawURL, nil)
	return req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(permissions...)))
}

func TestGSLBListSwitchRequests(t *testing.T) {
	h := newGSLBHandlerWithSwitchRequests()
	recorder := httptest.NewRecorder()
	h.ListSwitchRequests(recorder, gslbSwitchListRequest(
		"/api/v1/gslb/switch-requests?service_id=00000000-0000-4000-8000-0000000000a1", iam.ScopedPermission{
			TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionList,
		}))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []gslbapp.SwitchRequest `json:"items"`
		Total int                     `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || body.Items[0].Status != gslbapp.StatusPendingApproval {
		t.Fatalf("body = %+v", body)
	}
}

func TestGSLBListSwitchRequestsStatusFilter(t *testing.T) {
	h := newGSLBHandlerWithSwitchRequests()
	recorder := httptest.NewRecorder()
	h.ListSwitchRequests(recorder, gslbSwitchListRequest(
		"/api/v1/gslb/switch-requests?service_id=00000000-0000-4000-8000-0000000000a1&status=Succeeded", iam.ScopedPermission{
			TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionRead,
		}))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []gslbapp.SwitchRequest `json:"items"`
		Total int                     `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 || len(body.Items) != 0 {
		t.Fatalf("filtered body = %+v", body)
	}
}

func TestGSLBListSwitchRequestsInvalidServiceID(t *testing.T) {
	h := newGSLBHandlerWithSwitchRequests()
	recorder := httptest.NewRecorder()
	h.ListSwitchRequests(recorder, gslbSwitchListRequest(
		"/api/v1/gslb/switch-requests?service_id=not-a-uuid", iam.ScopedPermission{
			TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionList,
		}))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBListSwitchRequestsInvalidStatus(t *testing.T) {
	h := newGSLBHandlerWithSwitchRequests()
	recorder := httptest.NewRecorder()
	h.ListSwitchRequests(recorder, gslbSwitchListRequest(
		"/api/v1/gslb/switch-requests?service_id=00000000-0000-4000-8000-0000000000a1&status=Bogus", iam.ScopedPermission{
			TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionList,
		}))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBListSwitchRequestsCrossTenantService(t *testing.T) {
	h := newGSLBHandlerWithSwitchRequests()
	recorder := httptest.NewRecorder()
	h.ListSwitchRequests(recorder, gslbSwitchListRequest(
		"/api/v1/gslb/switch-requests?service_id=00000000-0000-4000-8000-0000000000ff", iam.ScopedPermission{
			TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionList,
		}))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBListSwitchRequestsForbidden(t *testing.T) {
	h := newGSLBHandlerWithSwitchRequests()
	recorder := httptest.NewRecorder()
	h.ListSwitchRequests(recorder, gslbSwitchListRequest(
		"/api/v1/gslb/switch-requests?service_id=00000000-0000-4000-8000-0000000000a1"))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func gslbCreateBody(domain string) string {
	body, _ := json.Marshal(map[string]any{
		"name": "api-gateway", "domain": domain, "requireApproval": true,
		"pools": []map[string]any{
			{"name": "active", "priority": 0, "members": []map[string]any{{"clusterId": "test-cluster-a", "weight": 100}}},
			{"name": "backup", "priority": 1, "members": []map[string]any{{"clusterId": "test-cluster-b", "weight": 100}}},
		},
		"healthCheck": map[string]any{
			"protocol": "http", "path": "/healthz",
			"intervalSeconds": 10, "timeoutSeconds": 3, "failureThreshold": 3,
		},
	})
	return string(body)
}

func gslbCreateRequest(body string, permissions ...iam.ScopedPermission) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gslb/services", strings.NewReader(body))
	return req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(permissions...)))
}

func gslbCreatePermission() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionCreate}
}

func TestGSLBCreateServiceOK(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.CreateService(recorder, gslbCreateRequest(gslbCreateBody("new-api.hnb.cloud"), gslbCreatePermission()))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var detail gslbapp.ServiceDetail
	if err := json.NewDecoder(recorder.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID == "" || detail.Domain != "new-api.hnb.cloud" || !detail.RequireApproval {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.HealthCheck.Path != "/healthz" || detail.HealthCheck.IntervalSeconds != 10 {
		t.Fatalf("healthCheck = %+v", detail.HealthCheck)
	}
	if len(detail.Pools) != 2 {
		t.Fatalf("pools = %+v", detail.Pools)
	}
	// 最小 priority 池为 active 池
	active := detail.Pools[0]
	if !active.IsActive || detail.ActivePoolID != active.ID || active.Name != "active" {
		t.Fatalf("active pool = %+v activePoolId=%s", active, detail.ActivePoolID)
	}
	if len(active.Members) != 1 || active.Members[0].Weight != 100 {
		t.Fatalf("members = %+v", active.Members)
	}
	if !repo.createdService {
		t.Fatal("store CreateService not called")
	}
}

func TestGSLBCreateServiceIdempotentByDomain(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	repo.pools = []gslbapp.Pool{{ID: gslbTestPoolActive, Name: "active", Priority: 0,
		Members: []gslbapp.PoolMember{{ClusterID: "test-cluster-a", Weight: 100}}}}
	recorder := httptest.NewRecorder()
	// 同租户同 domain（桩仓库已有 api.hnb.cloud）→ 返回既有服务，200
	h.CreateService(recorder, gslbCreateRequest(gslbCreateBody("api.hnb.cloud"), gslbCreatePermission()))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var detail gslbapp.ServiceDetail
	if err := json.NewDecoder(recorder.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != repo.service.ID {
		t.Fatalf("expected existing service %s, got %s", repo.service.ID, detail.ID)
	}
	if repo.createdService {
		t.Fatal("store CreateService must not be called for duplicate domain")
	}
}

func TestGSLBCreateServiceValidation(t *testing.T) {
	cases := map[string]string{
		"empty pools":      `{"name":"svc","domain":"svc.hnb.cloud"}`,
		"bad domain":       `{"name":"svc","domain":"not a domain","pools":[{"name":"p","priority":0,"members":[{"clusterId":"c","weight":100}]}]}`,
		"bad name":         `{"name":"Bad_Name","domain":"svc.hnb.cloud","pools":[{"name":"p","priority":0,"members":[{"clusterId":"c","weight":100}]}]}`,
		"dup priority":     `{"name":"svc","domain":"svc.hnb.cloud","pools":[{"name":"p1","priority":0,"members":[{"clusterId":"c","weight":100}]},{"name":"p2","priority":0,"members":[{"clusterId":"c","weight":100}]}]}`,
		"empty members":    `{"name":"svc","domain":"svc.hnb.cloud","pools":[{"name":"p","priority":0,"members":[]}]}`,
		"weight over 100":  `{"name":"svc","domain":"svc.hnb.cloud","pools":[{"name":"p","priority":0,"members":[{"clusterId":"c","weight":101}]}]}`,
		"bad protocol":     `{"name":"svc","domain":"svc.hnb.cloud","pools":[{"name":"p","priority":0,"members":[{"clusterId":"c","weight":100}]}],"healthCheck":{"protocol":"tcp","path":"/healthz","intervalSeconds":10,"timeoutSeconds":3,"failureThreshold":3}}`,
		"bad path":         `{"name":"svc","domain":"svc.hnb.cloud","pools":[{"name":"p","priority":0,"members":[{"clusterId":"c","weight":100}]}],"healthCheck":{"protocol":"http","path":"healthz","intervalSeconds":10,"timeoutSeconds":3,"failureThreshold":3}}`,
		"interval too low": `{"name":"svc","domain":"svc.hnb.cloud","pools":[{"name":"p","priority":0,"members":[{"clusterId":"c","weight":100}]}],"healthCheck":{"protocol":"http","path":"/healthz","intervalSeconds":1,"timeoutSeconds":3,"failureThreshold":3}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := newGSLBHandlerForTest()
			recorder := httptest.NewRecorder()
			h.CreateService(recorder, gslbCreateRequest(body, gslbCreatePermission()))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestGSLBCreateServiceRejectsCrossTenantCluster(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	repo.knownClusters = map[string]bool{"test-cluster-a": true} // test-cluster-b 不属于本租户
	recorder := httptest.NewRecorder()
	h.CreateService(recorder, gslbCreateRequest(gslbCreateBody("new-api.hnb.cloud"), gslbCreatePermission()))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.createdService {
		t.Fatal("store CreateService must not be called for unknown cluster")
	}
}

func TestGSLBCreateServiceForbidden(t *testing.T) {
	h, _ := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.CreateService(recorder, gslbCreateRequest(gslbCreateBody("new-api.hnb.cloud")))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBCreateServiceUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gslb/services", strings.NewReader(gslbCreateBody("new-api.hnb.cloud")))
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.CreateService(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func gslbUpdatePermission() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionUpdate}
}

func gslbDeletePermission() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionDelete}
}

func gslbWriteRequest(method, serviceID, body, ifMatch string, permissions ...iam.ScopedPermission) *http.Request {
	suffix := ""
	if method == http.MethodPut {
		suffix = "/pools"
	}
	req := httptest.NewRequest(method, "/api/v1/gslb/services/"+serviceID+suffix, strings.NewReader(body))
	req.SetPathValue("id", serviceID)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	return req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(permissions...)))
}

func TestGSLBUpdateServiceOK(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, repo.service.ID,
		`{"name":"api-gw-v2","requireApproval":false}`, "", gslbUpdatePermission()))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var detail gslbapp.ServiceDetail
	if err := json.NewDecoder(recorder.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Name != "api-gw-v2" || detail.RequireApproval {
		t.Fatalf("detail = %+v", detail)
	}
	if repo.updatedCalls != 1 {
		t.Fatalf("updatedCalls = %d", repo.updatedCalls)
	}
}

func TestGSLBUpdateServiceIdempotentNoChange(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	// 与现状一致（name 相同）→ 200 且不写库
	h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, repo.service.ID,
		`{"name":"api-gateway","requireApproval":true}`, "", gslbUpdatePermission()))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.updatedCalls != 0 {
		t.Fatalf("no-change patch must not write, updatedCalls = %d", repo.updatedCalls)
	}
	var detail gslbapp.ServiceDetail
	if err := json.NewDecoder(recorder.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Name != "api-gateway" {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestGSLBUpdateServiceValidation(t *testing.T) {
	cases := map[string]string{
		"bad name":         `{"name":"Bad_Name"}`,
		"bad protocol":     `{"healthCheck":{"protocol":"tcp","path":"/healthz","intervalSeconds":10,"timeoutSeconds":3,"failureThreshold":3}}`,
		"interval too low": `{"healthCheck":{"protocol":"http","path":"/healthz","intervalSeconds":1,"timeoutSeconds":3,"failureThreshold":3}}`,
		"bad path":         `{"healthCheck":{"protocol":"http","path":"healthz","intervalSeconds":10,"timeoutSeconds":3,"failureThreshold":3}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h, repo := newGSLBHandlerForTest()
			recorder := httptest.NewRecorder()
			h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, repo.service.ID, body, "", gslbUpdatePermission()))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if repo.updatedCalls != 0 {
				t.Fatal("invalid patch must not write")
			}
		})
	}
}

func TestGSLBUpdateServiceFailingOverConflict(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	repo.service.LifecycleState = "FailingOver"
	recorder := httptest.NewRecorder()
	h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, repo.service.ID,
		`{"name":"api-gw-v2"}`, "", gslbUpdatePermission()))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.updatedCalls != 0 {
		t.Fatal("failing-over service must not be edited")
	}
}

func TestGSLBUpdateServiceIfMatch(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	// If-Match 与当前 updatedAt 一致 → 放行
	recorder := httptest.NewRecorder()
	h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, repo.service.ID,
		`{"name":"api-gw-v2"}`, repo.service.UpdatedAt.Format(time.RFC3339Nano), gslbUpdatePermission()))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	// If-Match 过期 → 409
	h2, repo2 := newGSLBHandlerForTest()
	recorder2 := httptest.NewRecorder()
	h2.UpdateService(recorder2, gslbWriteRequest(http.MethodPatch, repo2.service.ID,
		`{"name":"api-gw-v2"}`, repo2.service.UpdatedAt.Add(-time.Hour).Format(time.RFC3339Nano), gslbUpdatePermission()))
	if recorder2.Code != http.StatusConflict {
		t.Fatalf("stale If-Match status = %d", recorder2.Code)
	}
	if repo2.updatedCalls != 0 {
		t.Fatal("stale If-Match must not write")
	}

	// If-Match 非法 → 400
	recorder3 := httptest.NewRecorder()
	h2.UpdateService(recorder3, gslbWriteRequest(http.MethodPatch, repo2.service.ID,
		`{"name":"api-gw-v2"}`, "not-a-time", gslbUpdatePermission()))
	if recorder3.Code != http.StatusBadRequest {
		t.Fatalf("bad If-Match status = %d", recorder3.Code)
	}
}

func TestGSLBUpdateServiceCrossTenantNotFound(t *testing.T) {
	h, _ := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, "00000000-0000-4000-8000-0000000000ff",
		`{"name":"api-gw-v2"}`, "", gslbUpdatePermission()))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBUpdateServiceForbidden(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.UpdateService(recorder, gslbWriteRequest(http.MethodPatch, repo.service.ID, `{"name":"api-gw-v2"}`, ""))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func gslbReplaceBody(pools ...map[string]any) string {
	body, _ := json.Marshal(map[string]any{"pools": pools})
	return string(body)
}

func gslbPoolInput(name string, priority int, clusterID string) map[string]any {
	return map[string]any{
		"name": name, "priority": priority,
		"members": []map[string]any{{"clusterId": clusterID, "weight": 100}},
	}
}

func TestGSLBReplacePoolsOK(t *testing.T) {
	h, repo := newGSLBHandlerWithPools()
	recorder := httptest.NewRecorder()
	// 保留 active 池名字，调整成员
	h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, repo.service.ID,
		gslbReplaceBody(gslbPoolInput("active", 0, "test-cluster-a"), gslbPoolInput("standby", 2, "test-cluster-b")),
		"", gslbUpdatePermission()))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.replaceCalls != 1 {
		t.Fatalf("replaceCalls = %d", repo.replaceCalls)
	}
	if repo.service.ActivePoolID == gslbTestPoolActive {
		t.Fatal("active pool must be re-pointed to the new pool id")
	}
	var body struct {
		Items []gslbapp.Pool `json:"items"`
		Total int            `json:"total"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 2 || len(body.Items) != 2 {
		t.Fatalf("body = %+v", body)
	}
	var active *gslbapp.Pool
	for i := range body.Items {
		if body.Items[i].IsActive {
			active = &body.Items[i]
		}
	}
	if active == nil || active.Name != "active" {
		t.Fatalf("active pool = %+v", active)
	}
}

func TestGSLBReplacePoolsIdempotent(t *testing.T) {
	h, repo := newGSLBHandlerWithPools()
	recorder := httptest.NewRecorder()
	// 与现状语义一致（同名同 priority 同成员权重）→ 200 不写库
	h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, repo.service.ID,
		gslbReplaceBody(gslbPoolInput("active", 0, "test-cluster-a"), gslbPoolInput("backup", 1, "test-cluster-b")),
		"", gslbUpdatePermission()))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.replaceCalls != 0 {
		t.Fatalf("equivalent pools must not be rewritten, replaceCalls = %d", repo.replaceCalls)
	}
}

func TestGSLBReplacePoolsValidation(t *testing.T) {
	cases := map[string]string{
		"empty pools":   `{"pools":[]}`,
		"dup priority":  gslbReplaceBody(gslbPoolInput("a", 0, "test-cluster-a"), gslbPoolInput("b", 0, "test-cluster-b")),
		"empty members": `{"pools":[{"name":"active","priority":0,"members":[]}]}`,
		"weight over":   `{"pools":[{"name":"active","priority":0,"members":[{"clusterId":"test-cluster-a","weight":101}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h, repo := newGSLBHandlerWithPools()
			recorder := httptest.NewRecorder()
			h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, repo.service.ID, body, "", gslbUpdatePermission()))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			if repo.replaceCalls != 0 {
				t.Fatal("invalid pools must not write")
			}
		})
	}
}

func TestGSLBReplacePoolsRejectsCrossTenantCluster(t *testing.T) {
	h, repo := newGSLBHandlerWithPools()
	repo.knownClusters = map[string]bool{"test-cluster-a": true}
	recorder := httptest.NewRecorder()
	h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, repo.service.ID,
		gslbReplaceBody(gslbPoolInput("active", 0, "test-cluster-a"), gslbPoolInput("backup", 1, "test-cluster-b")),
		"", gslbUpdatePermission()))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.replaceCalls != 0 {
		t.Fatal("unknown cluster must not write")
	}
}

func TestGSLBReplacePoolsActiveRemovedConflict(t *testing.T) {
	h, repo := newGSLBHandlerWithPools()
	recorder := httptest.NewRecorder()
	// 新配置不含当前 active 池（名字 "active"）→ 409，提示先 failover
	h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, repo.service.ID,
		gslbReplaceBody(gslbPoolInput("backup", 1, "test-cluster-b")),
		"", gslbUpdatePermission()))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.replaceCalls != 0 {
		t.Fatal("active pool removal must not write")
	}
}

func TestGSLBReplacePoolsFailingOverConflict(t *testing.T) {
	h, repo := newGSLBHandlerWithPools()
	repo.service.LifecycleState = "FailingOver"
	recorder := httptest.NewRecorder()
	h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, repo.service.ID,
		gslbReplaceBody(gslbPoolInput("active", 0, "test-cluster-a")), "", gslbUpdatePermission()))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBReplacePoolsCrossTenantNotFound(t *testing.T) {
	h, _ := newGSLBHandlerWithPools()
	recorder := httptest.NewRecorder()
	h.ReplacePools(recorder, gslbWriteRequest(http.MethodPut, "00000000-0000-4000-8000-0000000000ff",
		gslbReplaceBody(gslbPoolInput("active", 0, "test-cluster-a")), "", gslbUpdatePermission()))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func gslbDeleteRequest(serviceID, rawQuery string, permissions ...iam.ScopedPermission) *http.Request {
	url := "/api/v1/gslb/services/" + serviceID
	if rawQuery != "" {
		url += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("id", serviceID)
	return req.WithContext(iam.WithTrustedContext(req.Context(), gslbTrusted(permissions...)))
}

func TestGSLBDeleteServiceActiveRequiresForce(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.DeleteService(recorder, gslbDeleteRequest(repo.service.ID, "", gslbDeletePermission()))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.deleted {
		t.Fatal("active service must not be deleted without force")
	}
}

func TestGSLBDeleteServiceForceBlockedByInflight(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	repo.inflight = true
	recorder := httptest.NewRecorder()
	h.DeleteService(recorder, gslbDeleteRequest(repo.service.ID, "force=true", gslbDeletePermission()))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.deleted {
		t.Fatal("service with in-flight switch requests must not be deleted")
	}
}

func TestGSLBDeleteServiceForceOK(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.DeleteService(recorder, gslbDeleteRequest(repo.service.ID, "force=true", gslbDeletePermission()))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !repo.deleted {
		t.Fatal("store DeleteService not called")
	}
}

func TestGSLBDeleteServiceInactiveOK(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	repo.service.LifecycleState = "Inactive"
	recorder := httptest.NewRecorder()
	h.DeleteService(recorder, gslbDeleteRequest(repo.service.ID, "", gslbDeletePermission()))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !repo.deleted {
		t.Fatal("store DeleteService not called")
	}
}

func TestGSLBDeleteServiceCrossTenantNotFound(t *testing.T) {
	h, _ := newGSLBHandlerForTest()
	recorder := httptest.NewRecorder()
	h.DeleteService(recorder, gslbDeleteRequest("00000000-0000-4000-8000-0000000000ff", "force=true", gslbDeletePermission()))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBDeleteServiceForbidden(t *testing.T) {
	h, repo := newGSLBHandlerForTest()
	repo.service.LifecycleState = "Inactive"
	recorder := httptest.NewRecorder()
	h.DeleteService(recorder, gslbDeleteRequest(repo.service.ID, ""))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGSLBDeleteServiceUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gslb/services/00000000-0000-4000-8000-0000000000a1", nil)
	req.SetPathValue("id", "00000000-0000-4000-8000-0000000000a1")
	recorder := httptest.NewRecorder()

	h, _ := newGSLBHandlerForTest()
	h.DeleteService(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

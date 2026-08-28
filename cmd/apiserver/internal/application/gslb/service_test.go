package gslb

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/F31/hnb/pkg/gslb"
	"github.com/F31/hnb/pkg/iam"
)

const (
	testServiceID = "00000000-0000-4000-8000-0000000000a1"
	testPoolA     = "00000000-0000-4000-8000-0000000000b1"
	testPoolB     = "00000000-0000-4000-8000-0000000000b2"
	testClusterA  = "cluster-a"
	testClusterB  = "cluster-b"
)

type stubRepo struct {
	lastDrill    *DrillReport
	drillReports []DrillReport
	services     map[string]Service
	members      map[string][]string
	requests     map[string]SwitchRequest
	byKey        map[string]SwitchRequest
	created      []SwitchRequest
	events       [][]OutboxEvent
	transitions  []string
}

func newStubRepo() *stubRepo {
	now := time.Now().UTC()
	return &stubRepo{
		services: map[string]Service{
			testServiceID: {
				ID: testServiceID, TenantID: "tenant-a", Name: "api-gateway",
				Domain: "api.hnb.cloud", RoutingMode: "dns", ActivePoolID: testPoolA,
				LifecycleState: "Active", RequireApproval: true, CreatedAt: now, UpdatedAt: now,
			},
		},
		members: map[string][]string{
			testPoolA: {testClusterA},
			testPoolB: {testClusterB},
		},
		requests: map[string]SwitchRequest{},
		byKey:    map[string]SwitchRequest{},
	}
}

func (s *stubRepo) GetService(_ context.Context, id, tenantID string) (Service, bool, error) {
	svc, ok := s.services[id]
	if !ok || svc.TenantID != tenantID {
		return Service{}, false, nil
	}
	return svc, true, nil
}

func (s *stubRepo) GetPoolMemberClusterIDs(_ context.Context, poolID string) ([]string, error) {
	return s.members[poolID], nil
}

func (s *stubRepo) GetSwitchRequestByKey(_ context.Context, tenantID, serviceID, idempotencyKey string) (SwitchRequest, bool, error) {
	req, ok := s.byKey[tenantID+"|"+serviceID+"|"+idempotencyKey]
	return req, ok, nil
}

func (s *stubRepo) GetSwitchRequest(_ context.Context, id, _ string) (SwitchRequest, bool, error) {
	req, ok := s.requests[id]
	return req, ok, nil
}

func (s *stubRepo) CreateSwitchRequest(_ context.Context, req SwitchRequest, drill *DrillReport, events []OutboxEvent) error {
	s.lastDrill = drill
	s.requests[req.ID] = req
	s.byKey[req.TenantID+"|"+req.ServiceID+"|"+req.IdempotencyKey] = req
	s.created = append(s.created, req)
	s.events = append(s.events, events)
	return nil
}

func (s *stubRepo) ListDrillReports(_ context.Context, serviceID, tenantID string) ([]DrillReport, error) {
	return s.drillReports, nil
}

func (s *stubRepo) ListReadModels(_ context.Context, tenantID string) ([]ReadModel, error) {
	return []ReadModel{{ServiceID: testServiceID, TenantID: tenantID, Domain: "api.hnb.cloud", LifecycleState: "Active"}}, nil
}

func (s *stubRepo) GetReadModel(_ context.Context, id, tenantID string) (ReadModel, bool, error) {
	if id != testServiceID {
		return ReadModel{}, false, nil
	}
	return ReadModel{ServiceID: id, TenantID: tenantID, Domain: "api.hnb.cloud", LifecycleState: "Active"}, true, nil
}

func (s *stubRepo) UpdateSwitchRequestStatus(_ context.Context, id, status string, fields map[string]any, events []OutboxEvent) error {
	req := s.requests[id]
	req.Status = status
	if v, ok := fields["approved_by"]; ok {
		req.ApprovedBy = v.(string)
	}
	s.requests[id] = req
	s.transitions = append(s.transitions, id+":"+status)
	s.events = append(s.events, events)
	return nil
}

func trustedWith(permissions ...iam.ScopedPermission) iam.TrustedContext {
	return iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", ScopedPermissions: permissions}
}

func gslbExecute() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionExecute}
}

func gslbUpdate() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionUpdate}
}

func intentBody(kind gslb.IntentKind, targetPool string, key string) []byte {
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1",
		"kind":       string(kind),
		"serviceId":  testServiceID,
		"tenantId":   "tenant-a",
		"targetPoolId": targetPool,
		"metadata": map[string]any{
			"idempotencyKey": key,
			"correlationId":  "00000000-0000-4000-8000-0000000000c1",
		},
	})
	return body
}

func subjects(events []OutboxEvent) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Subject)
	}
	return out
}

func TestSubmitFailoverPendingApproval(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	req, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentFailover, testPoolB, "k-failover-1"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != StatusPendingApproval {
		t.Fatalf("status = %s", req.Status)
	}
	if len(repo.events) != 1 || repo.events[0][0].Subject != EventIntentSubmitted {
		t.Fatalf("submission events: %v", subjects(repo.events[0]))
	}
	// 待审批：不得派发执行命令
	for _, e := range repo.events[0] {
		if e.Subject == CommandStepRequested {
			t.Fatal("pending request must not dispatch")
		}
	}
	// 计划快照包含 apply/verify/revert
	var plan gslb.Plan
	if err := json.Unmarshal(req.PlanSnapshot, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[2].StepType != gslb.StepDNSRevert {
		t.Fatalf("plan steps: %+v", plan.Steps)
	}
}

func TestSubmitWeightUpdateDispatchesImmediately(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1",
		"kind":       "gslb.weight-update",
		"serviceId":  testServiceID,
		"tenantId":   "tenant-a",
		"weights":    map[string]int{testClusterA: 50, testClusterB: 50},
		"metadata": map[string]any{
			"idempotencyKey": "k-weight-1",
			"correlationId":  "00000000-0000-4000-8000-0000000000c1",
		},
	})
	req, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != StatusApproved {
		t.Fatalf("status = %s", req.Status)
	}
	got := subjects(repo.events[0])
	found := false
	for _, s := range got {
		if s == CommandStepRequested {
			found = true
		}
	}
	if !found {
		t.Fatalf("approved executable intent must dispatch, events=%v", got)
	}
}

func TestSubmitDrillCompletesWithoutDispatch(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	req, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentDrill, testPoolB, "k-drill-1"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != StatusDrillCompleted {
		t.Fatalf("status = %s", req.Status)
	}
	for _, e := range repo.events[0] {
		if e.Subject == CommandStepRequested {
			t.Fatal("drill must not dispatch DNS execution")
		}
	}
}

func TestSubmitIdempotentReplay(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	body := intentBody(gslb.IntentFailover, testPoolB, "k-idem-1")
	if _, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute())); err != nil {
		t.Fatal(err)
	}
	second, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d, want idempotent replay", len(repo.created))
	}
	if second.Status != StatusPendingApproval {
		t.Fatalf("replayed status = %s", second.Status)
	}
}

func TestSubmitRejectsExecutableFields(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	body := []byte(`{
		"apiVersion":"gslb.hnb.io/v1","kind":"gslb.failover",
		"serviceId":"` + testServiceID + `","tenantId":"tenant-a",
		"targetPoolId":"` + testPoolB + `",
		"providerId":"evil-provider","command":"apply",
		"metadata":{"idempotencyKey":"k-evil","correlationId":"00000000-0000-4000-8000-0000000000c1"}
	}`)
	if _, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute())); err == nil {
		t.Fatal("intent with execution fields must be rejected")
	}
}

func TestSubmitRejectsTenantMismatch(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1", "kind": "gslb.failover",
		"serviceId": testServiceID, "tenantId": "tenant-other",
		"targetPoolId": testPoolB,
		"metadata": map[string]any{
			"idempotencyKey": "k-tenant", "correlationId": "00000000-0000-4000-8000-0000000000c1",
		},
	})
	if _, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute())); err == nil {
		t.Fatal("tenant mismatch must be rejected")
	}
}

func TestSubmitRequiresExecutePermission(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	if _, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentFailover, testPoolB, "k-noperm"), testServiceID, trustedWith()); err != ErrForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestSubmitServiceNotFound(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	otherService := "00000000-0000-4000-8000-0000000000f1"
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1", "kind": "gslb.failover",
		"serviceId": otherService, "tenantId": "tenant-a",
		"targetPoolId": testPoolB,
		"metadata": map[string]any{
			"idempotencyKey": "k-missing", "correlationId": "00000000-0000-4000-8000-0000000000c1",
		},
	})
	if _, err := app.SubmitIntent(context.Background(), body, otherService, trustedWith(gslbExecute())); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestApproveDispatchesAndRejectBlocks(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	req, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentFailover, testPoolB, "k-approve-1"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	// 审批通过 → 派发执行命令
	approved, err := app.Approve(context.Background(), req.ID, trustedWith(gslbUpdate()))
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != StatusApproved {
		t.Fatalf("status = %s", approved.Status)
	}
	last := repo.events[len(repo.events)-1]
	found := false
	for _, e := range last {
		if e.Subject == CommandStepRequested {
			found = true
		}
	}
	if !found {
		t.Fatalf("approval must dispatch, events=%v", subjects(last))
	}

	// 再提交一个并拒绝
	req2, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentFailover, testPoolB, "k-reject-1"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := app.Reject(context.Background(), req2.ID, trustedWith(gslbUpdate()))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != StatusRejected {
		t.Fatalf("status = %s", rejected.Status)
	}
	// 已终态请求不可再次审批
	if _, err := app.Approve(context.Background(), req2.ID, trustedWith(gslbUpdate())); err == nil {
		t.Fatal("rejected request must not be approvable")
	}
}

func TestSwitchRequestIDsAreUUIDs(t *testing.T) {
	if _, err := uuid.Parse(testServiceID); err != nil {
		t.Fatal("test fixture ids must be uuids")
	}
}

func TestDRSwitchbackForcesApproval(t *testing.T) {
	repo := newStubRepo()
	// 服务级关闭审批降级：DR 编排的回切仍必须显式人工确认（GSLB-009）。
	svc := repo.services[testServiceID]
	svc.RequireApproval = false
	repo.services[testServiceID] = svc
	app := NewService(repo)
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1", "kind": "gslb.switchback",
		"serviceId": testServiceID, "tenantId": "tenant-a",
		"targetPoolId": testPoolA, "drGroupRef": "dr-group-region-east",
		"metadata": map[string]any{
			"idempotencyKey": "k-dr-switchback", "correlationId": "00000000-0000-4000-8000-0000000000c1",
		},
	})
	req, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != StatusPendingApproval {
		t.Fatalf("DR switchback status = %s, want PendingApproval", req.Status)
	}
	if req.DRGroupRef != "dr-group-region-east" {
		t.Fatalf("drGroupRef = %q", req.DRGroupRef)
	}
	for _, e := range repo.events[0] {
		if e.Subject == CommandStepRequested {
			t.Fatal("DR switchback must not dispatch before approval")
		}
	}
}

func TestSubmitRejectsInvalidDRGroupRef(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "gslb.hnb.io/v1", "kind": "gslb.failover",
		"serviceId": testServiceID, "tenantId": "tenant-a",
		"targetPoolId": testPoolB, "drGroupRef": "bad ref with spaces!",
		"metadata": map[string]any{
			"idempotencyKey": "k-dr-bad", "correlationId": "00000000-0000-4000-8000-0000000000c1",
		},
	})
	if _, err := app.SubmitIntent(context.Background(), body, testServiceID, trustedWith(gslbExecute())); err == nil {
		t.Fatal("invalid drGroupRef must be rejected")
	}
}

func TestSubmitDrillPersistsStructuredReport(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	req, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentDrill, testPoolB, "k-drill-report"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	drill := repo.lastDrill
	if drill == nil {
		t.Fatal("drill report must be persisted")
	}
	if drill.RequestID != req.ID || drill.ServiceID != testServiceID || drill.TenantID != "tenant-a" {
		t.Fatalf("drill linkage: %+v", drill)
	}
	if drill.Verdict != DrillVerdictReady {
		t.Fatalf("verdict = %s, want Ready", drill.Verdict)
	}
	var payload map[string]any
	if err := json.Unmarshal(drill.Report, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["targetPoolId"] != testPoolB {
		t.Fatalf("report targetPoolId = %v", payload["targetPoolId"])
	}
	targets, ok := payload["projectedTargets"].([]any)
	if !ok || len(targets) != 1 || targets[0] != testClusterB {
		t.Fatalf("projectedTargets = %v", payload["projectedTargets"])
	}
	checks, ok := payload["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("checks = %v", payload["checks"])
	}
}

func TestSubmitDrillWithoutTargetPoolBlocked(t *testing.T) {
	repo := newStubRepo()
	repo.members[testPoolB] = nil // 目标池无启用成员 → Blocked
	app := NewService(repo)
	_, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentDrill, testPoolB, "k-drill-blocked"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastDrill == nil || repo.lastDrill.Verdict != DrillVerdictBlocked {
		t.Fatalf("verdict = %+v, want Blocked", repo.lastDrill)
	}
}

func TestSubmitAssignsOperationID(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo)
	req, err := app.SubmitIntent(context.Background(), intentBody(gslb.IntentFailover, testPoolB, "k-op-id"), testServiceID, trustedWith(gslbExecute()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(req.OperationID); err != nil {
		t.Fatalf("operationId must be a uuid, got %q", req.OperationID)
	}
}

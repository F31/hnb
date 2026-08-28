package dr

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	gslbapp "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	"github.com/F31/hnb/pkg/gslb"
	"github.com/F31/hnb/pkg/iam"
)

const (
	testGroupID     = "00000000-0000-4000-8000-0000000000d1"
	testGSLBService = "00000000-0000-4000-8000-0000000000e1"
	testBackupPool  = "00000000-0000-4000-8000-0000000000f1"
	testPrimaryPool = "00000000-0000-4000-8000-0000000000f2"
)

type stubRepo struct {
	groups        map[string]Group
	members       map[string][]Member
	runs          map[string]SwitchRun
	byKey         map[string]SwitchRun
	backupPoolOK  bool
	primaryPoolOK bool
	requestStatus map[string]string
	createdRuns   []SwitchRun
	updatedStatus []string
	storedEvents  [][]OutboxEvent
}

func newStubRepo() *stubRepo {
	now := time.Now().UTC()
	return &stubRepo{
		groups: map[string]Group{
			testGroupID: {
				ID: testGroupID, TenantID: "tenant-a", Name: "region-east",
				PrimaryRegion: "cn-east", StandbyRegion: "cn-north",
				LifecycleState: "Ready", CreatedAt: now, UpdatedAt: now,
			},
		},
		members:       map[string][]Member{},
		runs:          map[string]SwitchRun{},
		byKey:         map[string]SwitchRun{},
		backupPoolOK:  true,
		primaryPoolOK: true,
		requestStatus: map[string]string{},
	}
}

func (s *stubRepo) CreateGroup(_ context.Context, group Group) error {
	s.groups[group.ID] = group
	return nil
}

func (s *stubRepo) GetGroup(_ context.Context, id, tenantID string) (Group, bool, error) {
	group, ok := s.groups[id]
	if !ok || group.TenantID != tenantID {
		return Group{}, false, nil
	}
	return group, true, nil
}

func (s *stubRepo) ListGroups(_ context.Context, tenantID string) ([]Group, error) {
	var out []Group
	for _, group := range s.groups {
		if group.TenantID == tenantID {
			out = append(out, group)
		}
	}
	return out, nil
}

func (s *stubRepo) AddMember(_ context.Context, member Member) error {
	s.members[member.GroupID] = append(s.members[member.GroupID], member)
	return nil
}

func (s *stubRepo) ListMembers(_ context.Context, groupID string) ([]Member, error) {
	return s.members[groupID], nil
}

func (s *stubRepo) GetGSLBBackupPool(_ context.Context, serviceID, _ string) (string, bool, error) {
	if !s.backupPoolOK {
		return "", false, nil
	}
	return testBackupPool, true, nil
}

func (s *stubRepo) GetGSLBPrimaryPool(_ context.Context, serviceID string) (string, bool, error) {
	if !s.primaryPoolOK {
		return "", false, nil
	}
	return testPrimaryPool, true, nil
}

func (s *stubRepo) GetRunByKey(_ context.Context, groupID, idempotencyKey string) (SwitchRun, bool, error) {
	run, ok := s.byKey[groupID+"|"+idempotencyKey]
	return run, ok, nil
}

func (s *stubRepo) GetRun(_ context.Context, id, tenantID string) (SwitchRun, bool, error) {
	run, ok := s.runs[id]
	if !ok || run.TenantID != tenantID {
		return SwitchRun{}, false, nil
	}
	return run, true, nil
}

func (s *stubRepo) CreateRun(_ context.Context, run SwitchRun, events []OutboxEvent) error {
	s.runs[run.ID] = run
	s.byKey[run.GroupID+"|"+run.IdempotencyKey] = run
	s.createdRuns = append(s.createdRuns, run)
	s.storedEvents = append(s.storedEvents, events)
	return nil
}

func (s *stubRepo) UpdateRun(_ context.Context, id, status string, fields map[string]any, events []OutboxEvent) error {
	run := s.runs[id]
	run.Status = status
	if v, ok := fields["traffic_request_ids"]; ok {
		run.TrafficRequestIDs = v.([]string)
	}
	if v, ok := fields["error"]; ok {
		run.Error = v.(string)
	}
	s.runs[id] = run
	s.updatedStatus = append(s.updatedStatus, id+":"+status)
	s.storedEvents = append(s.storedEvents, events)
	return nil
}

func (s *stubRepo) ListRuns(_ context.Context, groupID, tenantID string) ([]SwitchRun, error) {
	var out []SwitchRun
	for _, run := range s.runs {
		if run.GroupID == groupID && run.TenantID == tenantID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (s *stubRepo) TrafficRequestStatuses(_ context.Context, requestIDs []string) ([]string, error) {
	var out []string
	for _, id := range requestIDs {
		status, ok := s.requestStatus[id]
		if !ok {
			continue
		}
		out = append(out, status)
	}
	return out, nil
}

type stubSubmitter struct {
	calls    int
	bodies   [][]byte
	services []string
	failErr  error
	nextID   int
}

func (s *stubSubmitter) SubmitIntent(_ context.Context, body []byte, pathServiceID string, _ iam.TrustedContext) (gslbapp.SwitchRequest, error) {
	if s.failErr != nil {
		return gslbapp.SwitchRequest{}, s.failErr
	}
	s.calls++
	s.bodies = append(s.bodies, body)
	s.services = append(s.services, pathServiceID)
	s.nextID++
	return gslbapp.SwitchRequest{ID: uuid.NewString(), Status: gslbapp.StatusPendingApproval}, nil
}

func trustedWith(permissions ...iam.ScopedPermission) iam.TrustedContext {
	return iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", ScopedPermissions: permissions}
}

func drPermission(action iam.AuthorizationAction) iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceDR), Action: action}
}

func addGSLBMember(t *testing.T, app *App, refID string) {
	t.Helper()
	if _, err := app.AddMember(context.Background(), testGroupID, MemberGSLBService, refID, "traffic", trustedWith(drPermission(iam.ActionUpdate))); err != nil {
		t.Fatalf("add gslb member: %v", err)
	}
}

func addDataMember(t *testing.T, app *App, refID string) {
	t.Helper()
	if _, err := app.AddMember(context.Background(), testGroupID, MemberDataLayer, refID, "data", trustedWith(drPermission(iam.ActionUpdate))); err != nil {
		t.Fatalf("add data member: %v", err)
	}
}

func lastIntent(t *testing.T, submitter *stubSubmitter) gslb.Intent {
	t.Helper()
	if len(submitter.bodies) == 0 {
		t.Fatal("no intent submitted")
	}
	var intent gslb.Intent
	if err := json.Unmarshal(submitter.bodies[len(submitter.bodies)-1], &intent); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	return intent
}

// TestInitiateSwitchDataLayerGate 是 OBS-008 的顺序保证：组内有数据层成员时，
// InitiateSwitch 不得触达流量层，必须停在 DataLayerPending 等待显式确认。
func TestInitiateSwitchDataLayerGate(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)
	addDataMember(t, app, "postgres-main")

	run, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "drill", "key-1", trustedWith(drPermission(iam.ActionExecute)))
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunDataLayerPending {
		t.Fatalf("status = %s, want DataLayerPending", run.Status)
	}
	if submitter.calls != 0 {
		t.Fatalf("traffic layer must not be touched before data-layer confirmation (calls=%d)", submitter.calls)
	}
}

// TestConfirmDataLayerDispatchesTraffic 确认后才推进流量层：每个 gslb_service
// 成员各提交一次携带 drGroupRef=组 ID 的受控意图。
func TestConfirmDataLayerDispatchesTraffic(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)
	addDataMember(t, app, "postgres-main")

	run, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "region outage", "key-2", trustedWith(drPermission(iam.ActionExecute)))
	if err != nil {
		t.Fatal(err)
	}
	run, err = app.ConfirmDataLayer(context.Background(), run.ID, trustedWith(drPermission(iam.ActionUpdate)))
	if err != nil {
		t.Fatal(err)
	}
	if submitter.calls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitter.calls)
	}
	if run.Status != RunAwaitingApproval {
		t.Fatalf("status = %s, want AwaitingApproval", run.Status)
	}
	if len(run.TrafficRequestIDs) != 1 {
		t.Fatalf("trafficRequestIds = %v", run.TrafficRequestIDs)
	}
	intent := lastIntent(t, submitter)
	if intent.DRGroupRef != testGroupID {
		t.Fatalf("drGroupRef = %q, want group id %s", intent.DRGroupRef, testGroupID)
	}
	if intent.Metadata.IdempotencyKey != "key-2-"+testGSLBService {
		t.Fatalf("idempotencyKey = %q", intent.Metadata.IdempotencyKey)
	}
	if intent.Kind != gslb.IntentFailover {
		t.Fatalf("kind = %s, want failover", intent.Kind)
	}
	if intent.TargetPoolID != testBackupPool {
		t.Fatalf("targetPoolId = %s, want backup pool", intent.TargetPoolID)
	}
}

// TestInitiateSwitchNoDataMemberDispatchesImmediately 无数据层成员时直接进入流量层。
func TestInitiateSwitchNoDataMemberDispatchesImmediately(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)

	run, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-3", trustedWith(drPermission(iam.ActionExecute)))
	if err != nil {
		t.Fatal(err)
	}
	if submitter.calls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitter.calls)
	}
	if run.Status != RunAwaitingApproval {
		t.Fatalf("status = %s, want AwaitingApproval", run.Status)
	}
}

// TestSwitchbackUsesPrimaryPool 回切方向提交 gslb.switchback 意图，目标为主池。
func TestSwitchbackUsesPrimaryPool(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)

	if _, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionSwitchback, "", "key-4", trustedWith(drPermission(iam.ActionExecute))); err != nil {
		t.Fatal(err)
	}
	intent := lastIntent(t, submitter)
	if intent.Kind != gslb.IntentSwitchback {
		t.Fatalf("kind = %s, want switchback", intent.Kind)
	}
	if intent.TargetPoolID != testPrimaryPool {
		t.Fatalf("targetPoolId = %s, want primary pool", intent.TargetPoolID)
	}
}

// TestInitiateSwitchIdempotentReplay 同幂等键重放返回既有运行，不重复触达流量层。
func TestInitiateSwitchIdempotentReplay(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)

	trusted := trustedWith(drPermission(iam.ActionExecute))
	first, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-5", trusted)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-5", trusted)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotent replay returned different run: %s vs %s", second.ID, first.ID)
	}
	if submitter.calls != 1 {
		t.Fatalf("submitter calls = %d, want 1 (replay must not re-dispatch)", submitter.calls)
	}
}

// TestInitiateSwitchActiveRunConflict 存在非终态运行时拒绝发起新运行。
func TestInitiateSwitchActiveRunConflict(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)

	trusted := trustedWith(drPermission(iam.ActionExecute))
	if _, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-6", trusted); err != nil {
		t.Fatal(err)
	}
	if _, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-6b", trusted); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestPermissionDenied(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo, &stubSubmitter{})
	if _, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-7", trustedWith()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("InitiateSwitch err = %v, want ErrForbidden", err)
	}
	if _, err := app.CreateGroup(context.Background(), "g", "r1", "r2", trustedWith()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateGroup err = %v, want ErrForbidden", err)
	}
}

// TestAggregateRunsTerminalStates 运行终态由子 gslb 请求聚合推导。
func TestAggregateRunsTerminalStates(t *testing.T) {
	repo := newStubRepo()
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)

	trusted := trustedWith(drPermission(iam.ActionExecute), drPermission(iam.ActionRead))
	run, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-8", trusted)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range run.TrafficRequestIDs {
		repo.requestStatus[id] = gslbapp.StatusSucceeded
	}
	runs, err := app.ListRuns(context.Background(), testGroupID, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != RunCompleted {
		t.Fatalf("runs = %+v, want single Completed run", runs)
	}

	for _, id := range run.TrafficRequestIDs {
		repo.requestStatus[id] = gslbapp.StatusFailed
	}
	runs, err = app.ListRuns(context.Background(), testGroupID, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Status != RunFailed {
		t.Fatalf("status = %s, want Failed", runs[0].Status)
	}
}

// TestDispatchFailureFailsRun 目标池不可解析时运行置 Failed 并记录原因。
func TestDispatchFailureFailsRun(t *testing.T) {
	repo := newStubRepo()
	repo.backupPoolOK = false
	submitter := &stubSubmitter{}
	app := NewService(repo, submitter)
	addGSLBMember(t, app, testGSLBService)

	run, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-9", trustedWith(drPermission(iam.ActionExecute)))
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunFailed {
		t.Fatalf("status = %s, want Failed", run.Status)
	}
	if run.Error == "" {
		t.Fatal("error reason must be recorded")
	}
	if submitter.calls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitter.calls)
	}
}

// TestConfirmDataLayerRejectsWrongState 非 DataLayerPending 状态不得确认。
func TestConfirmDataLayerRejectsWrongState(t *testing.T) {
	repo := newStubRepo()
	app := NewService(repo, &stubSubmitter{})
	addGSLBMember(t, app, testGSLBService)

	run, err := app.InitiateSwitch(context.Background(), testGroupID, DirectionFailover, "", "key-10", trustedWith(drPermission(iam.ActionExecute)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ConfirmDataLayer(context.Background(), run.ID, trustedWith(drPermission(iam.ActionUpdate))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

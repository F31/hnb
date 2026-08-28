package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
	"github.com/F31/hnb/pkg/gslb"
)

type fakeRequestStore struct {
	request      store.SwitchRequest
	found        bool
	transitions  []string
	statusEvents []string
	errors       map[string]error
}

func (f *fakeRequestStore) GetRequest(_ context.Context, id string) (store.SwitchRequest, bool, error) {
	if f.errors != nil && f.errors["get"] != nil {
		return store.SwitchRequest{}, false, f.errors["get"]
	}
	if !f.found || f.request.ID != id {
		return store.SwitchRequest{}, false, nil
	}
	return f.request, true, nil
}

func (f *fakeRequestStore) Transition(_ context.Context, id string, from []string, to string, errorMsg string) error {
	if f.errors != nil && f.errors["transition"] != nil {
		return f.errors["transition"]
	}
	f.transitions = append(f.transitions, id+":"+to)
	return nil
}

func (f *fakeRequestStore) EmitStatusChanged(_ context.Context, _ store.SwitchRequest, status, errorMsg string) error {
	f.statusEvents = append(f.statusEvents, status+":"+errorMsg)
	return nil
}

type fakeExecutor struct {
	ran     int
	last    *gslb.Plan
	failErr error
}

func (f *fakeExecutor) ExecutePlan(_ context.Context, plan *gslb.Plan) error {
	f.ran++
	f.last = plan
	return f.failErr
}

func commandBody(requestID string, plan *gslb.Plan) []byte {
	planJSON, _ := json.Marshal(plan)
	body, _ := json.Marshal(map[string]any{
		"requestId": requestID,
		"serviceId": "00000000-0000-4000-8000-0000000000a1",
		"plan":      json.RawMessage(planJSON),
	})
	return body
}

func planForTest() *gslb.Plan {
	intent := &gslb.Intent{
		APIVersion: gslb.APIVersion, Kind: gslb.IntentFailover,
		ServiceID: "00000000-0000-4000-8000-0000000000a1", TenantID: "tenant-a",
		TargetPoolID: "00000000-0000-4000-8000-0000000000b2",
		Metadata:     gslb.IntentMetadata{IdempotencyKey: "k1", CorrelationID: "00000000-0000-4000-8000-0000000000c1"},
	}
	plan, err := intent.BuildPlan(gslb.PlanInput{
		ServiceID: intent.ServiceID, TenantID: intent.TenantID, Domain: "api.hnb.cloud",
		TargetPoolID: intent.TargetPoolID, Targets: []string{"10.0.1.10"}, PreviousTargets: []string{"10.0.0.10"},
	})
	if err != nil {
		panic(err)
	}
	return plan
}

func TestHandleCommandExecutesApprovedRequest(t *testing.T) {
	plan := planForTest()
	reqStore := &fakeRequestStore{
		found: true,
		request: store.SwitchRequest{
			ID: "req-1", TenantID: "tenant-a", ServiceID: "00000000-0000-4000-8000-0000000000a1",
			Status: store.RequestStatusApproved,
		},
	}
	exec := &fakeExecutor{}
	c := New(nil, reqStore, exec)

	if err := c.HandleCommand(context.Background(), commandBody("req-1", plan)); err != nil {
		t.Fatal(err)
	}
	if exec.ran != 1 {
		t.Fatalf("executor ran = %d", exec.ran)
	}
	// Approved → Dispatched → Succeeded
	got := reqStore.transitions
	if len(got) != 2 || got[0] != "req-1:"+store.RequestStatusDispatched || got[1] != "req-1:"+store.RequestStatusSucceeded {
		t.Fatalf("transitions = %v", got)
	}
	if len(reqStore.statusEvents) != 1 || reqStore.statusEvents[0] != store.RequestStatusSucceeded+":" {
		t.Fatalf("status events = %v", reqStore.statusEvents)
	}
}

func TestHandleCommandRejectsNonApproved(t *testing.T) {
	plan := planForTest()
	reqStore := &fakeRequestStore{
		found: true,
		request: store.SwitchRequest{
			ID: "req-1", TenantID: "tenant-a", ServiceID: "s1",
			Status: store.RequestStatusPendingApproval,
		},
	}
	exec := &fakeExecutor{}
	c := New(nil, reqStore, exec)

	if err := c.HandleCommand(context.Background(), commandBody("req-1", plan)); err == nil {
		t.Fatal("pending request must be rejected")
	}
	if exec.ran != 0 {
		t.Fatal("executor must not run for pending request")
	}
	if len(reqStore.transitions) != 0 {
		t.Fatalf("transitions = %v", reqStore.transitions)
	}
}

func TestHandleCommandRejectsUnknownRequest(t *testing.T) {
	plan := planForTest()
	c := New(nil, &fakeRequestStore{found: false}, &fakeExecutor{})
	if err := c.HandleCommand(context.Background(), commandBody("nope", plan)); err == nil {
		t.Fatal("unknown request must be rejected")
	}
}

func TestHandleCommandFailsAndRecordsFailure(t *testing.T) {
	plan := planForTest()
	reqStore := &fakeRequestStore{
		found: true,
		request: store.SwitchRequest{
			ID: "req-1", TenantID: "tenant-a", ServiceID: "s1",
			Status: store.RequestStatusApproved,
		},
	}
	exec := &fakeExecutor{failErr: errors.New("dns provider unavailable")}
	c := New(nil, reqStore, exec)

	if err := c.HandleCommand(context.Background(), commandBody("req-1", plan)); err == nil {
		t.Fatal("execution failure must surface")
	}
	if reqStore.transitions[len(reqStore.transitions)-1] != "req-1:"+store.RequestStatusFailed {
		t.Fatalf("transitions = %v", reqStore.transitions)
	}
	if len(reqStore.statusEvents) != 1 || reqStore.statusEvents[0] != store.RequestStatusFailed+":dns provider unavailable" {
		t.Fatalf("status events = %v", reqStore.statusEvents)
	}
}

func TestHandleCommandRejectsMalformedPayload(t *testing.T) {
	c := New(nil, &fakeRequestStore{}, &fakeExecutor{})
	if err := c.HandleCommand(context.Background(), []byte(`{not-json`)); err == nil {
		t.Fatal("malformed payload must be rejected")
	}
}

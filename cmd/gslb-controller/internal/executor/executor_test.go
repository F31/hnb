package executor

import (
	"context"
	"testing"

	"github.com/F31/hnb/cmd/gslb-controller/internal/dns"
	"github.com/F31/hnb/pkg/gslb"
)

type dnsWrite struct {
	name    string
	records []dns.DNSRecord
}

type fakeDNS struct {
	calls    []string
	writes   []dnsWrite
	failOn   string // target 触发失败的哨兵
	records  map[string][]dns.DNSRecord
}

func (f *fakeDNS) ApplyRecords(_ context.Context, name string, records []dns.DNSRecord) (string, error) {
	f.calls = append(f.calls, name)
	f.writes = append(f.writes, dnsWrite{name: name, records: records})
	f.records[name] = records
	if f.failOn != "" && len(records) > 0 {
		for _, r := range records {
			for _, t := range r.Targets {
				if t == f.failOn {
					return "", errDNSApply
				}
			}
		}
	}
	return "fake://" + name, nil
}

func (f *fakeDNS) VerifyTargets(_ context.Context, name string, expected []string) error {
	f.calls = append(f.calls, name)
	return nil
}

func (f *fakeDNS) DeleteRecords(_ context.Context, name string) error {
	f.calls = append(f.calls, name)
	return nil
}

var errDNSApply = &applyError{}

type applyError struct{}

func (*applyError) Error() string { return "dns apply failed" }

func planWith(serviceID, domain string, targets, previous []string) *gslb.Plan {
	intent := &gslb.Intent{
		APIVersion: gslb.APIVersion,
		Kind:       gslb.IntentFailover,
		ServiceID:  serviceID,
		TenantID:   "tenant-a",
		TargetPoolID: "00000000-0000-4000-8000-0000000000b2",
		Metadata: gslb.IntentMetadata{
			IdempotencyKey: "k-exec",
			CorrelationID:  "00000000-0000-4000-8000-0000000000c1",
		},
	}
	plan, err := intent.BuildPlan(gslb.PlanInput{
		ServiceID: serviceID, TenantID: "tenant-a", Domain: domain,
		TargetPoolID: intent.TargetPoolID, Targets: targets, PreviousTargets: previous,
	})
	if err != nil {
		panic(err)
	}
	return plan
}

func TestExecutePlanAppliesVerifyRevertOrder(t *testing.T) {
	fake := &fakeDNS{records: map[string][]dns.DNSRecord{}}
	exec := NewExecutor(fake, 300)

	plan := planWith("00000000-0000-4000-8000-0000000000a1", "api.hnb.cloud", []string{"10.0.1.10"}, []string{"10.0.0.10"})
	if err := exec.ExecutePlan(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	// apply → verify → revert 顺序；revert 目标为上一已知目标。
	// 数据面写入仅 apply 与 revert（verify 走 VerifyTargets，不写）。
	if len(fake.calls) != 3 {
		t.Fatalf("calls = %v", fake.calls)
	}
	if len(fake.writes) != 2 {
		t.Fatalf("writes = %d", len(fake.writes))
	}
	applyRecords := fake.writes[0].records
	if len(applyRecords) != 1 || len(applyRecords[0].Targets) != 1 || applyRecords[0].Targets[0] != "10.0.1.10" {
		t.Fatalf("apply records: %+v", applyRecords)
	}
	revertRecords := fake.writes[1].records
	if len(revertRecords) != 1 || len(revertRecords[0].Targets) != 1 || revertRecords[0].Targets[0] != "10.0.0.10" {
		t.Fatalf("revert records: %+v", revertRecords)
	}
}

func TestExecutePlanRevertsOnApplyFailure(t *testing.T) {
	fake := &fakeDNS{failOn: "10.0.9.9", records: map[string][]dns.DNSRecord{}}
	exec := NewExecutor(fake, 300)

	plan := planWith("00000000-0000-4000-8000-0000000000a1", "api.hnb.cloud", []string{"10.0.9.9"}, []string{"10.0.0.10"})
	if err := exec.ExecutePlan(context.Background(), plan); err == nil {
		t.Fatal("apply failure must surface")
	}
	// 补偿：revert 确保上一目标
	revertRecords := fake.records["api-hnb-cloud"]
	if len(revertRecords) != 1 || len(revertRecords[0].Targets) != 1 || revertRecords[0].Targets[0] != "10.0.0.10" {
		t.Fatalf("revert records: %+v", revertRecords)
	}
}

func TestExecuteStepDrillIsReadOnly(t *testing.T) {
	fake := &fakeDNS{records: map[string][]dns.DNSRecord{}}
	exec := NewExecutor(fake, 300)
	if err := exec.ExecuteStep(context.Background(), gslb.Step{
		StepID: "compute", StepType: gslb.StepDrillCompute,
		Inputs: map[string]any{"serviceId": "s1", "targetPoolId": "p2"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("drill must not touch DNS, calls=%v", fake.calls)
	}
}

func TestExecuteStepRejectsUnknownType(t *testing.T) {
	fake := &fakeDNS{records: map[string][]dns.DNSRecord{}}
	exec := NewExecutor(fake, 300)
	if err := exec.ExecuteStep(context.Background(), gslb.Step{
		StepID: "x", StepType: "evil_step",
	}); err == nil {
		t.Fatal("unknown step type must be rejected")
	}
}

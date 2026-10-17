package provision

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type recordedProgress struct{ events []StageEvent }

func (r *recordedProgress) Observe(event StageEvent) { r.events = append(r.events, event) }

func TestSharedStagesReportProgressWithoutSecretOutputs(t *testing.T) {
	observer := &recordedProgress{}
	p := New(WithBootstrapper(&NodeBootstrapper{SSH: &fakeRunner{}, Key: fakeKeyResolver{}, Store: &fakeStore{}}), WithProgressObserver(observer))
	_, err := p.Provision(context.Background(), ProvisionSpec{TenantID: "t", TargetID: "target", DesiredVersion: "1.36.5", Nodes: []NodeSpec{{ID: "cp-1", Host: "10.0.0.10", Role: RoleControlPlane}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"prereq", "containerd-config", "kubeadm-init", "cni", "cluster-acceptance", "read-kubeconfig"}
	if len(observer.events) != len(want)*2 {
		t.Fatalf("missing start/finish observations: %+v", observer.events)
	}
	for i, stage := range want {
		start, finish := observer.events[i*2], observer.events[i*2+1]
		if start.Stage != stage || start.NodeID != "cp-1" || start.State != "running" || start.StartedAt.IsZero() || !start.FinishedAt.IsZero() || finish.State != "succeeded" || finish.FinishedAt.Before(start.StartedAt) {
			t.Fatalf("incorrect observation pair: %+v %+v", start, finish)
		}
	}
	data, err := json.Marshal(observer.events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"abcdef.0123456789abcdef", "PRIVATE KEY", "current-context", "certificate-authority-data"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("progress leaked remote outputs or credentials")
		}
	}
}

type failedStageRunner struct{}

func (failedStageRunner) Run(context.Context, NodeSpec, []byte, string) (string, error) {
	return "token=secret", errors.New("token=secret")
}

func TestSharedStageFailureRemainsFailure(t *testing.T) {
	observer := &recordedProgress{}
	p := New(WithBootstrapper(&NodeBootstrapper{SSH: failedStageRunner{}, Key: fakeKeyResolver{}, Store: &fakeStore{}}), WithProgressObserver(observer))
	_, err := p.Provision(context.Background(), ProvisionSpec{DesiredVersion: "1.36.5", Nodes: []NodeSpec{{ID: "cp-1", Host: "10.0.0.10", Role: RoleControlPlane}}})
	if err == nil || len(observer.events) != 2 || observer.events[1].State != "failed" {
		t.Fatalf("failure became stage success: %+v %v", observer.events, err)
	}
	data, _ := json.Marshal(observer.events)
	if strings.Contains(string(data), "secret") {
		t.Fatal("failed progress leaked remote error")
	}
}

type blockedEvidence struct{ fakeKeyResolver }

func (blockedEvidence) ValidateVersion(context.Context, string) error {
	return errors.New("version evidence missing")
}

func TestSharedExecutorKeepsCallerEvidenceGateBeforeSSH(t *testing.T) {
	observer, runner := &recordedProgress{}, &fakeRunner{}
	p := New(WithBootstrapper(&NodeBootstrapper{SSH: runner, Key: blockedEvidence{}, Store: &fakeStore{}}), WithProgressObserver(observer))
	_, err := p.Provision(context.Background(), ProvisionSpec{DesiredVersion: "1.36.5", Nodes: []NodeSpec{{ID: "cp-1", Host: "10.0.0.10", Role: RoleControlPlane}}})
	if err == nil || !strings.Contains(err.Error(), "version-evidence") || len(runner.scripts) != 0 || len(observer.events) != 0 {
		t.Fatal("shared executor bypassed caller qualification before SSH")
	}
}

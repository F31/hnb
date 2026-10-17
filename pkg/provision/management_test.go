package provision

import (
	"context"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// stubManagementClient records Apply calls and simulates control-plane
// readiness so the orchestrator's CAPI wiring is testable offline.
type stubManagementClient struct {
	applied       []unstructured.Unstructured
	controlReady  bool
	kubeconfig    []byte
	errKubeconfig error
}

func (s *stubManagementClient) Apply(_ context.Context, objects []unstructured.Unstructured) error {
	s.applied = append(s.applied, objects...)
	return nil
}

func (s *stubManagementClient) WaitControlPlaneReady(ctx context.Context, _, _ string, _ time.Duration) error {
	if !s.controlReady {
		return fmt.Errorf("control plane not ready")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (s *stubManagementClient) Kubeconfig(_ context.Context, _, _ string) ([]byte, error) {
	if s.errKubeconfig != nil {
		return nil, s.errKubeconfig
	}
	return s.kubeconfig, nil
}

func TestApplyAndWaitReady(t *testing.T) {
	mgmt := &stubManagementClient{controlReady: true}
	spec := ProvisionSpec{
		Nodes: []NodeSpec{{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane}},
	}
	objects := CAPIManifests(spec, "prod", "")
	if err := mgmt.Apply(context.Background(), objects); err != nil {
		t.Fatal(err)
	}
	if len(mgmt.applied) != len(objects) {
		t.Fatalf("applied %d, want %d", len(mgmt.applied), len(objects))
	}
	if err := mgmt.WaitControlPlaneReady(context.Background(), "default", "prod-control-plane", time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitControlPlaneReadyBlocks(t *testing.T) {
	mgmt := &stubManagementClient{controlReady: false}
	if err := mgmt.WaitControlPlaneReady(context.Background(), "default", "prod-control-plane", time.Millisecond); err == nil {
		t.Fatal("expected error when control plane not ready")
	}
}

func TestKubeconfigFromManagement(t *testing.T) {
	mgmt := &stubManagementClient{controlReady: true, kubeconfig: []byte("managed-kubeconfig")}
	got, err := mgmt.Kubeconfig(context.Background(), "default", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "managed-kubeconfig" {
		t.Fatalf("kubeconfig = %q", got)
	}
}

func TestManagementClusterIntegration(t *testing.T) {
	mgmt := &stubManagementClient{controlReady: true, kubeconfig: []byte("managed-kubeconfig")}
	spec := ProvisionSpec{
		TenantID: "t1", TargetID: "00000000-0000-4000-8000-0000000000aa",
		DesiredVersion: "v1.31.4",
		Nodes: []NodeSpec{
			{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane},
			{ID: "w1", Host: "10.0.0.11", Role: RoleWorker},
		},
	}
	runner := &fakeRunner{}
	store := &fakeStore{}
	p := New(
		WithBootstrapper(&NodeBootstrapper{SSH: runner, Key: fakeKeyResolver{}, Store: store}),
		WithManagementClient(mgmt),
	)
	p.clusterName = "cluster-1"
	_, err := p.Provision(context.Background(), spec)
	if err == nil {
		t.Fatal("mixed lifecycle ownership must fail before SSH execution")
	}
	if len(runner.scripts) != 0 || len(mgmt.applied) != 0 {
		t.Fatal("unsupported CAPI strategy performed side effects")
	}
}

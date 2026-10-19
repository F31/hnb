package deliveryplan

import (
	"context"
	"errors"
	"testing"
)

type fakeValidator struct {
	namespaceErr error
	secretErr    error
	quota        Quota
	snapshot     Snapshot
	snapshotErr  error
}

func (f fakeValidator) NamespaceOwned(context.Context, string, string) error { return f.namespaceErr }
func (f fakeValidator) SecretReferenceOwned(context.Context, string, SecretRef) error {
	return f.secretErr
}
func (f fakeValidator) Quota(context.Context, string, string) (Quota, error) { return f.quota, nil }
func (f fakeValidator) Snapshot(context.Context, string, string, string) (Snapshot, error) {
	return f.snapshot, f.snapshotErr
}

func validComposition() Composition {
	return Composition{
		SchemaVersion: CompositionSchema, Kind: "ApplicationComposition", Namespace: "app", TargetID: "target-a",
		ReleaseID: "release-1", ReleaseVersion: "1.0.0", SnapshotDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Resources: Resources{CPU: 2, Memory: 1024, Storage: 1024},
		Components: []Component{
			{Name: "api", Kind: "Deployment", Image: "registry.example/app@sha256:" + "b" + "000000000000000000000000000000000000000000000000000000000000000", DependsOn: []string{"db"}},
			{Name: "db", Kind: "StatefulSet", Image: "registry.example/db@sha256:" + "c" + "000000000000000000000000000000000000000000000000000000000000000"},
			{Name: "data", Kind: "PVC", StorageClassName: "fast", StorageBytes: 100},
			{Name: "route", Kind: "Ingress", Host: "app.example.test"},
		},
	}
}

func TestBuildOrdersDependenciesAndChecksQuota(t *testing.T) {
	validator := fakeValidator{quota: Quota{CPU: 4, Memory: 2048, Storage: 2048}, snapshot: Snapshot{StorageClasses: []string{"fast"}, CSIDrivers: []string{"csi.example"}, GatewayAPIVersion: []string{"v1"}}}
	plan, err := Build(context.Background(), "tenant-a", validComposition(), validator)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Name != "db" || plan.Steps[1].Name != "api" {
		t.Fatalf("unexpected order: %+v", plan.Steps)
	}
}

func TestBuildRejectsCrossTenantSecretSnapshotQuotaAndCycles(t *testing.T) {
	base := validComposition()
	base.Components[0].SecretRefs = []SecretRef{{Provider: "aes", Scope: "tenant", Name: "db", Version: "1"}}
	cases := []struct {
		name  string
		edit  func(*Composition)
		valid fakeValidator
		code  string
	}{
		{"namespace", func(c *Composition) {}, fakeValidator{namespaceErr: errors.New("not owned")}, "NAMESPACE_NOT_OWNED"},
		{"secret", func(c *Composition) {}, fakeValidator{secretErr: errors.New("not owned"), quota: Quota{}, snapshot: Snapshot{StorageClasses: []string{"fast"}, CSIDrivers: []string{"csi"}}}, "SECRET_REFERENCE_NOT_OWNED"},
		{"quota", func(c *Composition) {}, fakeValidator{quota: Quota{CPU: 1}, snapshot: Snapshot{StorageClasses: []string{"fast"}, CSIDrivers: []string{"csi"}}}, "QUOTA_EXCEEDED"},
		{"snapshot", func(c *Composition) {}, fakeValidator{snapshotErr: errors.New("missing")}, "SNAPSHOT_INVALID"},
		{"capability", func(c *Composition) {}, fakeValidator{snapshot: Snapshot{}}, "CAPABILITY_MISSING"},
		{"cycle", func(c *Composition) { c.Components[1].DependsOn = []string{"api"} }, fakeValidator{snapshot: Snapshot{StorageClasses: []string{"fast"}, CSIDrivers: []string{"csi"}, GatewayAPIVersion: []string{"v1"}}}, "DEPENDENCY_CYCLE"},
		{"unpinned", func(c *Composition) { c.Components[0].Image = "registry.example/app:latest" }, fakeValidator{snapshot: Snapshot{StorageClasses: []string{"fast"}, CSIDrivers: []string{"csi"}, GatewayAPIVersion: []string{"v1"}}}, "COMPONENT_IMAGE_UNPINNED"},
		{"kind", func(c *Composition) { c.Components[0].Kind = "Manifest" }, fakeValidator{snapshot: Snapshot{StorageClasses: []string{"fast"}, CSIDrivers: []string{"csi"}, GatewayAPIVersion: []string{"v1"}}}, "COMPONENT_KIND_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composition := base
			composition.Components = append([]Component(nil), base.Components...)
			tc.edit(&composition)
			_, err := Build(context.Background(), "tenant-a", composition, tc.valid)
			var problem *Problem
			if !errors.As(err, &problem) || problem.Code != tc.code {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
		})
	}
}

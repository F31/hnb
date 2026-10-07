package provision

import (
	"testing"
)

func TestCAPIManifestsRender(t *testing.T) {
	spec := ProvisionSpec{
		DesiredVersion: "v1.31.4",
		Nodes: []NodeSpec{
			{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane},
			{ID: "w1", Host: "10.0.0.11", Role: RoleWorker},
		},
	}
	objects := CAPIManifests(spec, "prod", "")
	if len(objects) == 0 {
		t.Fatal("no manifests rendered")
	}
	got := map[string]string{}
	for _, u := range objects {
		got[u.GetKind()] = u.GetName()
	}
	for _, want := range []string{"Cluster", "KubeadmControlPlane", "SSHCluster", "SSHMachineTemplate", "MachineDeployment", "KubeadmConfigTemplate", "SSHMachineDeployment"} {
		if got[want] == "" {
			t.Fatalf("missing CAPI object kind %s; got %v", want, got)
		}
	}
	if got["Cluster"] != "prod" {
		t.Fatalf("cluster name = %q, want prod", got["Cluster"])
	}
}

func TestCAPIManifestsSingleControlPlane(t *testing.T) {
	spec := ProvisionSpec{
		Nodes: []NodeSpec{{ID: "cp1", Host: "10.0.0.10", Role: RoleControlPlane}},
	}
	objects := CAPIManifests(spec, "small", "")
	hasDeployment := false
	for _, u := range objects {
		if u.GetKind() == "MachineDeployment" {
			hasDeployment = true
		}
	}
	if hasDeployment {
		t.Fatal("MachineDeployment should not render with zero workers")
	}
}
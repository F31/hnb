package toolkit

import (
	"strings"
	"testing"
	"time"
)

// existingManifestFixture mirrors the 4.3 existing-cluster ownership boundary:
// this install owns only its release, and the cluster/CNI/backings are foreign.
func existingManifestFixture() *OwnershipManifest {
	return &OwnershipManifest{
		SchemaVersion: "ownership-manifest/v1", ReleaseID: "release.lock.yaml",
		Namespace: "hnb-system", ReleaseName: "hnb-existing-minimal",
		Claimed: []string{
			"namespace/hnb-system",
			"release/hnb-existing-minimal (hnb-system)",
			"serviceaccount/observer+executor (hnb-system)",
			"clusterrole/hnb-existing-minimal-*-observer",
			"clusterrolebinding/hnb-existing-minimal-*-observer",
			"persistentvolumeclaim/postgresql data (hnb-system)",
			"persistentvolumeclaim/jetstream data (hnb-system)",
		},
		RequiredExisting: []string{
			"secret/fixture-tls", "storageclass/fixture-storage",
			"secret/tenant/platform-pg", "secret/tenant/platform-nats",
		},
		PreservedExternal: []string{
			"external kubernetes cluster lifecycle owner (kubeconfig is the entry point)",
			"external CNI daemonsets/deployments (never modified or replaced)",
			"external namespaces/pvs/business workloads outside hnb-system",
			"external postgresql backing (never reconfigured or deleted)",
		},
		NoNodeInitAction: true, FitsBoundary: true,
	}
}

func deletionContains(list []string, want string) bool {
	for _, entry := range list {
		if strings.Contains(entry, want) {
			return true
		}
	}
	return false
}

func removalNow() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }

// 卸载 HNB 只删除本次拥有的 release 资源.
func TestUninstallHNBDeletesOnlyOwnedRelease(t *testing.T) {
	plan, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Mode: RemovalUninstallHNB,
		DataLossAcknowledged: true, Now: removalNow()})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized {
		t.Fatalf("authorized uninstall refused: %v", plan.Blocked)
	}
	// Owned release resources are deleted.
	for _, want := range []string{"release/hnb-existing-minimal", "serviceaccount/observer+executor", "clusterrole/hnb-existing-minimal-*-observer"} {
		if !deletionContains(plan.Delete, want) {
			t.Fatalf("owned resource not scheduled for removal: %s (%v)", want, plan.Delete)
		}
	}
	// Foreign owner resources are never in the delete set.
	for _, forbidden := range []string{"external kubernetes", "external CNI", "external postgresql", "business workloads"} {
		if deletionContains(plan.Delete, forbidden) {
			t.Fatalf("uninstall scheduled external resource for deletion: %s", forbidden)
		}
		if !deletionContains(plan.Retain, forbidden) {
			t.Fatalf("external resource not retained: %s (%v)", forbidden, plan.Retain)
		}
	}
	// Operator-provided secrets/shared backings are retained.
	for _, forbidden := range []string{"secret/fixture-tls", "storageclass/fixture-storage", "secret/tenant/platform-pg", "secret/tenant/platform-nats"} {
		if deletionContains(plan.Delete, forbidden) {
			t.Fatalf("operator-provided resource scheduled for deletion: %s", forbidden)
		}
		if !deletionContains(plan.Retain, forbidden) {
			t.Fatalf("operator-provided resource not retained: %s (%v)", forbidden, plan.Retain)
		}
	}
}

// 自有持久存储删除必须显式确认.
func TestUninstallRequiresAcknowledgementForOwnedStorage(t *testing.T) {
	plan, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Mode: RemovalUninstallHNB, Now: removalNow()})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized {
		t.Fatal("storage removal authorized without acknowledgement")
	}
	if !deletionContains(plan.Blocked, "PERSISTENT_STORAGE_REMOVAL_UNACKNOWLEDGED") {
		t.Fatalf("missing storage acknowledgement block: %v", plan.Blocked)
	}
}

// 解除纳管不删除任何本地资源，只经正式平台契约释放管理关系.
func TestUnmanageDeletesNothingLocally(t *testing.T) {
	plan, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Mode: RemovalUnmanage, Now: removalNow()})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Delete) != 0 {
		t.Fatalf("unmanage scheduled deletions: %v", plan.Delete)
	}
	if plan.Authorized {
		t.Fatal("unmanage reported as locally authorized")
	}
	if !deletionContains(plan.Blocked, "UNMANAGE_REQUIRES_PLATFORM_OPERATION") {
		t.Fatalf("unmanage must require a formal platform operation: %v", plan.Blocked)
	}
	// The platform release is explicitly retained.
	if !deletionContains(plan.Retain, "release/hnb-existing-minimal") {
		t.Fatalf("unmanage did not retain the release: %v", plan.Retain)
	}
}

// 删除自建管理基座：外部集群一律拒绝.
func TestDeleteManagementBaseRefusesExternallyOwnedCluster(t *testing.T) {
	plan, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Mode: RemovalDeleteBase,
		InstallCreatedBase: false, Now: removalNow()})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized {
		t.Fatal("delete-management-base authorized against an external cluster")
	}
	for _, want := range []string{"BASE_NOT_OWNED_BY_THIS_INSTALL", "NODE_OWNERSHIP_NOT_ESTABLISHED"} {
		if !deletionContains(plan.Blocked, want) {
			t.Fatalf("missing block %s: %v", want, plan.Blocked)
		}
	}
	if !deletionContains(plan.Retain, "external kubernetes cluster lifecycle owner") {
		t.Fatalf("external cluster not retained: %v", plan.Retain)
	}
}

// 自建基座：需要安装创建的证据与已核验的基座 UID.
func TestDeleteManagementBaseRequiresOwnedBaseUID(t *testing.T) {
	manifest := existingManifestFixture()
	manifest.NoNodeInitAction = false // a self-bootstrapped base did init nodes

	// Install-created but UID unverified -> refused.
	plan, _ := PlanRemoval(RemovalInput{Manifest: manifest, Mode: RemovalDeleteBase,
		InstallCreatedBase: true, Now: removalNow()})
	if plan.Authorized || !deletionContains(plan.Blocked, "BASE_UID_UNVERIFIED") {
		t.Fatalf("unverified base UID not refused: %+v", plan)
	}

	// Install-created with a verified UID -> authorized.
	plan, _ = PlanRemoval(RemovalInput{Manifest: manifest, Mode: RemovalDeleteBase,
		InstallCreatedBase: true, OwnedBaseUID: "uid-self-created", Now: removalNow()})
	if !plan.Authorized {
		t.Fatalf("self-created base removal refused: %v", plan.Blocked)
	}
	if !deletionContains(plan.Delete, "management-base/uid=uid-self-created") {
		t.Fatalf("base not scheduled for removal: %v", plan.Delete)
	}
	// Even then, foreign resources stay retained.
	for _, forbidden := range []string{"external kubernetes", "external CNI", "business workloads"} {
		if deletionContains(plan.Delete, forbidden) {
			t.Fatalf("base removal destroyed external owner resource: %s", forbidden)
		}
	}
}

// 无论输入如何，ownership 清单中的保留项都不得进入删除集合（安全不变量）.
func TestRemovalNeverDeletesPreservedResources(t *testing.T) {
	manifest := existingManifestFixture()
	// Simulate a corrupted manifest where a preserved entry is also claimed.
	manifest.Claimed = append(manifest.Claimed, "external CNI daemonsets/deployments (never modified or replaced)")
	plan, err := PlanRemoval(RemovalInput{Manifest: manifest, Mode: RemovalUninstallHNB,
		DataLossAcknowledged: true, Now: removalNow()})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized {
		t.Fatal("ownership violation did not block the plan")
	}
	if !deletionContains(plan.Blocked, "OWNERSHIP_VIOLATION") {
		t.Fatalf("ownership violation not reported: %v", plan.Blocked)
	}
}

func TestUnsupportedRemovalModeRefused(t *testing.T) {
	if _, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Mode: RemovalMode("destroy-everything"), Now: removalNow()}); err == nil {
		t.Fatal("unsupported removal mode accepted")
	}
	if _, err := PlanRemoval(RemovalInput{Mode: RemovalUninstallHNB, Now: removalNow()}); err == nil {
		t.Fatal("removal plan accepted without an ownership manifest")
	}
}

// 客户 PVC 与业务工作负载必须作为 retained data 明确保留.
func TestCustomerDataIsAlwaysRetained(t *testing.T) {
	for _, mode := range []RemovalMode{RemovalUninstallHNB, RemovalUnmanage, RemovalDeleteBase} {
		plan, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Mode: mode,
			DataLossAcknowledged: true, InstallCreatedBase: true, OwnedBaseUID: "uid-x", Now: removalNow()})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"business workloads", "external postgresql backing"} {
			if !deletionContains(plan.RetainedData, want) {
				t.Fatalf("mode %s dropped retained customer data %q: %v", mode, want, plan.RetainedData)
			}
			if deletionContains(plan.Delete, want) {
				t.Fatalf("mode %s deleted customer data %q", mode, want)
			}
		}
	}
}

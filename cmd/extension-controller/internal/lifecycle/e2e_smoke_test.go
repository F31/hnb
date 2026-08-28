package lifecycle

import (
	"context"
	"testing"
)

const digest2 = "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

type smokeVerifier struct {
	manifests map[string]Manifest
}

func (v smokeVerifier) VerifyManifest(_ context.Context, cmd Command) (Manifest, error) {
	return v.manifests[cmd.ProviderVersion], nil
}

func TestE2ESmokeProviderLifecycleInstallUpgradeRollback(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	manifestV1 := lifecycleManifest()
	manifestV2 := manifestV1
	manifestV2.ProviderVersion = "2.0.0"
	manifestV2.BundleDigest = digest2
	manifestV2.Capabilities = []string{"compute.instance", "compute.autoscale"}
	manifestV2.Routes = append(manifestV2.Routes, NavigationRoute{Path: "/providers/aws/autoscale", Permission: "provider.aws:autoscale", MenuTitle: "Autoscale"})
	svc := NewService(store, smokeVerifier{manifests: map[string]Manifest{
		"1.0.0": manifestV1,
		"2.0.0": manifestV2,
	}}, fakeCompat{}, fakeHealth(true))

	install := lifecycleCommand(ActionInstall)
	event, err := svc.Reconcile(ctx, install)
	if err != nil || event.Phase != "enabled" {
		t.Fatalf("install event=%+v err=%v", event, err)
	}

	upgrade := lifecycleCommand(ActionUpgrade)
	upgrade.ProviderVersion = "2.0.0"
	upgrade.BundleDigest = digest2
	upgrade.IdempotencyKey = "upgrade-2.0.0"
	event, err = svc.Reconcile(ctx, upgrade)
	if err != nil || event.Phase != "enabled" {
		t.Fatalf("upgrade event=%+v err=%v", event, err)
	}
	if got := store.snapshots[len(store.snapshots)-1]; got.ProviderVersion != "2.0.0" || !got.Active || len(got.Capabilities) != 2 || len(got.Routes) != 2 {
		t.Fatalf("upgrade snapshot not promoted with metadata: %+v", got)
	}

	rollback := lifecycleCommand(ActionRollback)
	rollback.IdempotencyKey = "rollback-1.0.0"
	event, err = svc.Reconcile(ctx, rollback)
	if err != nil || event.Phase != "enabled" {
		t.Fatalf("rollback event=%+v err=%v", event, err)
	}
	if got := store.snapshots[len(store.snapshots)-1]; got.ProviderVersion != "1.0.0" || !got.Active || len(got.Capabilities) != 1 || len(got.Routes) != 1 {
		t.Fatalf("rollback snapshot not restored: %+v", got)
	}
}

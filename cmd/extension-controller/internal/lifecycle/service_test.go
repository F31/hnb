package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	seen       map[string]bool
	states     []string
	snapshots  []Snapshot
	operations int
	report     DependencyReport
	deleted    bool
}

func (s *fakeStore) AcquireIdempotency(_ context.Context, cmd Command) (bool, error) {
	if s.seen == nil {
		s.seen = make(map[string]bool)
	}
	key := cmd.ProviderID + ":" + cmd.IdempotencyKey
	if s.seen[key] {
		return false, nil
	}
	s.seen[key] = true
	return true, nil
}

func (s *fakeStore) CreateOperation(context.Context, Command) error {
	s.operations++
	return nil
}

func (s *fakeStore) SaveLifecycleState(_ context.Context, _ Command, phase string) error {
	s.states = append(s.states, phase)
	return nil
}

func (s *fakeStore) SaveSnapshots(_ context.Context, snapshot Snapshot) error {
	s.snapshots = append(s.snapshots, snapshot)
	return nil
}

func (s *fakeStore) DependencyReport(context.Context, string) (DependencyReport, error) {
	return s.report, nil
}

func (s *fakeStore) DeleteSnapshots(context.Context, string, string) error {
	s.deleted = true
	return nil
}

type fakeVerifier struct{ manifest Manifest }

func (v fakeVerifier) VerifyManifest(context.Context, Command) (Manifest, error) {
	return v.manifest, nil
}

type fakeCompat struct{ err error }

func (c fakeCompat) Check(context.Context, Manifest) error { return c.err }

type fakeHealth bool

func (h fakeHealth) Healthy(context.Context, Snapshot) bool { return bool(h) }

func lifecycleCommand(action Action) Command {
	return Command{ProviderID: "provider.aws", ProviderVersion: "1.0.0", Action: action, BundleDigest: digest, OperationID: "op1", IdempotencyKey: string(action) + "-1"}
}

func lifecycleManifest() Manifest {
	expires := time.Now().Add(time.Hour)
	return Manifest{ProviderID: "provider.aws", ProviderVersion: "1.0.0", BundleDigest: digest, Capabilities: []string{"compute.instance"}, Routes: []NavigationRoute{{Path: "/providers/aws", Permission: "provider.aws:read", MenuTitle: "AWS"}}, Permissions: []string{"provider.aws:read"}, ConformanceUntil: &expires}
}

func TestInstallCreatesOperationAndMetadataSnapshots(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, fakeVerifier{manifest: lifecycleManifest()}, fakeCompat{}, fakeHealth(true))
	event, err := svc.Reconcile(context.Background(), lifecycleCommand(ActionInstall))
	if err != nil {
		t.Fatal(err)
	}
	if event.Phase != "enabled" || store.operations != 1 || len(store.snapshots) != 1 || !store.snapshots[0].Active {
		t.Fatalf("unexpected install result event=%+v store=%+v", event, store)
	}
	if len(store.snapshots[0].Capabilities) != 1 || len(store.snapshots[0].Routes) != 1 {
		t.Fatalf("metadata snapshot missing: %+v", store.snapshots[0])
	}
}

func TestInstallRejectsCompatibilityFailure(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, fakeVerifier{manifest: lifecycleManifest()}, fakeCompat{err: errors.New("incompatible")}, fakeHealth(true))
	event, err := svc.Reconcile(context.Background(), lifecycleCommand(ActionInstall))
	if err == nil || event.Phase != "rejected" || len(store.snapshots) != 0 {
		t.Fatalf("expected rejection before snapshot, event=%+v snapshots=%d err=%v", event, len(store.snapshots), err)
	}
}

func TestDuplicateLifecycleCommandIsIdempotent(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, fakeVerifier{manifest: lifecycleManifest()}, nil, fakeHealth(true))
	cmd := lifecycleCommand(ActionInstall)
	if _, err := svc.Reconcile(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	event, err := svc.Reconcile(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if event.Phase != "duplicate" || store.operations != 1 {
		t.Fatalf("expected duplicate no-op, event=%+v operations=%d", event, store.operations)
	}
}

func TestUpgradeRollsBackWhenHealthFails(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, fakeVerifier{manifest: lifecycleManifest()}, nil, fakeHealth(false))
	event, err := svc.Reconcile(context.Background(), lifecycleCommand(ActionUpgrade))
	if err == nil || event.Phase != "rolling_back" || len(store.snapshots) != 1 || store.snapshots[0].Active {
		t.Fatalf("expected inactive candidate rollback, event=%+v snapshots=%+v err=%v", event, store.snapshots, err)
	}
}

func TestUpgradePromotesHealthyCandidate(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, fakeVerifier{manifest: lifecycleManifest()}, nil, fakeHealth(true))
	event, err := svc.Reconcile(context.Background(), lifecycleCommand(ActionUpgrade))
	if err != nil {
		t.Fatal(err)
	}
	if event.Phase != "enabled" || len(store.snapshots) != 2 || !store.snapshots[1].Active {
		t.Fatalf("expected promoted candidate, event=%+v snapshots=%+v", event, store.snapshots)
	}
}

func TestUninstallRefusesDependenciesAndDeletesCleanProvider(t *testing.T) {
	store := &fakeStore{report: DependencyReport{RuntimeTargets: 1}}
	svc := NewService(store, nil, nil, nil)
	event, err := svc.Reconcile(context.Background(), lifecycleCommand(ActionUninstall))
	if !errors.Is(err, ErrUninstallBlocked) || event.Phase != "blocked" || store.deleted {
		t.Fatalf("expected blocked uninstall, event=%+v deleted=%v err=%v", event, store.deleted, err)
	}
	store.report = DependencyReport{}
	cmd := lifecycleCommand(ActionUninstall)
	cmd.IdempotencyKey = "uninstall-2"
	event, err = svc.Reconcile(context.Background(), cmd)
	if err != nil || event.Phase != "disabled" || !store.deleted {
		t.Fatalf("expected clean uninstall, event=%+v deleted=%v err=%v", event, store.deleted, err)
	}
}

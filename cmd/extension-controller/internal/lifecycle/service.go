package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	lifecycleDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hnb_provider_lifecycle_duration_seconds",
		Help:    "Provider lifecycle reconciliation duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"action", "phase"})
	lifecycleTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_provider_lifecycle_transitions_total",
		Help: "Provider lifecycle phase transitions.",
	}, []string{"action", "phase"})
	lifecycleRollbacks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_provider_lifecycle_rollbacks_total",
		Help: "Provider lifecycle rollbacks.",
	}, []string{"reason"})
	lifecycleHealthTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_provider_lifecycle_health_transitions_total",
		Help: "Provider lifecycle health transitions.",
	}, []string{"health"})
	lifecycleConformanceExpiry = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hnb_provider_conformance_expiry_seconds",
		Help: "Provider conformance expiry as Unix seconds.",
	}, []string{"provider_id", "provider_version"})
)

type Manifest struct {
	ProviderID       string
	ProviderVersion  string
	BundleDigest     string
	Capabilities     []string
	Routes           []NavigationRoute
	Permissions      []string
	ConformanceUntil *time.Time
}

type Store interface {
	AcquireIdempotency(context.Context, Command) (bool, error)
	CreateOperation(context.Context, Command) error
	SaveLifecycleState(context.Context, Command, string) error
	SaveSnapshots(context.Context, Snapshot) error
	DependencyReport(context.Context, string) (DependencyReport, error)
	DeleteSnapshots(context.Context, string, string) error
}

type Verifier interface {
	VerifyManifest(context.Context, Command) (Manifest, error)
}

type CompatibilityChecker interface {
	Check(context.Context, Manifest) error
}

type HealthChecker interface {
	Healthy(context.Context, Snapshot) bool
}

type Service struct {
	store         Store
	verifier      Verifier
	compatibility CompatibilityChecker
	health        HealthChecker
}

func NewService(store Store, verifier Verifier, compatibility CompatibilityChecker, health HealthChecker) *Service {
	return &Service{store: store, verifier: verifier, compatibility: compatibility, health: health}
}

func (s *Service) Reconcile(ctx context.Context, cmd Command) (Event, error) {
	started := time.Now()
	event := Event{ProviderID: cmd.ProviderID, ProviderVersion: cmd.ProviderVersion, Action: cmd.Action, BundleDigest: cmd.BundleDigest, OperationID: cmd.OperationID, OccurredAt: started}
	phase := "failed"
	defer func() {
		lifecycleDuration.WithLabelValues(string(cmd.Action), phase).Observe(time.Since(started).Seconds())
		lifecycleTransitions.WithLabelValues(string(cmd.Action), phase).Inc()
	}()

	if err := ValidateCommand(cmd); err != nil {
		return event, err
	}
	if s.store == nil {
		return event, errors.New("lifecycle store is required")
	}
	acquired, err := s.store.AcquireIdempotency(ctx, cmd)
	if err != nil {
		return event, err
	}
	if !acquired {
		phase = "duplicate"
		event.Phase = phase
		return event, nil
	}

	switch cmd.Action {
	case ActionInstall, ActionEnable:
		phase, err = s.installOrEnable(ctx, cmd)
	case ActionUpgrade:
		phase, err = s.upgrade(ctx, cmd)
	case ActionRollback:
		phase, err = s.rollback(ctx, cmd)
	case ActionUninstall:
		phase, err = s.uninstall(ctx, cmd)
	default:
		err = fmt.Errorf("%w: unsupported action %q", ErrInvalidCommand, cmd.Action)
	}
	event.Phase = phase
	return event, err
}

func (s *Service) installOrEnable(ctx context.Context, cmd Command) (string, error) {
	manifest, err := s.verify(ctx, cmd)
	if err != nil {
		return "rejected", err
	}
	if err := s.store.CreateOperation(ctx, cmd); err != nil {
		return "operation_failed", err
	}
	snapshot := snapshotFromManifest(manifest, true)
	if err := s.store.SaveSnapshots(ctx, snapshot); err != nil {
		return "snapshot_failed", err
	}
	if err := s.store.SaveLifecycleState(ctx, cmd, "enabled"); err != nil {
		return "state_failed", err
	}
	recordConformance(manifest)
	lifecycleHealthTransitions.WithLabelValues("healthy").Inc()
	return "enabled", nil
}

func (s *Service) upgrade(ctx context.Context, cmd Command) (string, error) {
	manifest, err := s.verify(ctx, cmd)
	if err != nil {
		return "rejected", err
	}
	candidate := snapshotFromManifest(manifest, false)
	if err := s.store.SaveLifecycleState(ctx, cmd, "candidate"); err != nil {
		return "state_failed", err
	}
	if err := s.store.SaveSnapshots(ctx, candidate); err != nil {
		return "snapshot_failed", err
	}
	if s.health != nil && !s.health.Healthy(ctx, candidate) {
		lifecycleRollbacks.WithLabelValues("health").Inc()
		return "rolling_back", errors.New("candidate health check failed")
	}
	candidate.Active = true
	if err := s.store.SaveSnapshots(ctx, candidate); err != nil {
		return "promote_failed", err
	}
	if err := s.store.SaveLifecycleState(ctx, cmd, "enabled"); err != nil {
		return "state_failed", err
	}
	recordConformance(manifest)
	lifecycleHealthTransitions.WithLabelValues("healthy").Inc()
	return "enabled", nil
}

func (s *Service) rollback(ctx context.Context, cmd Command) (string, error) {
	manifest, err := s.verify(ctx, cmd)
	if err != nil {
		return "rejected", err
	}
	snapshot := snapshotFromManifest(manifest, true)
	if err := s.store.SaveSnapshots(ctx, snapshot); err != nil {
		return "rollback_failed", err
	}
	lifecycleRollbacks.WithLabelValues("manual").Inc()
	if err := s.store.SaveLifecycleState(ctx, cmd, "enabled"); err != nil {
		return "state_failed", err
	}
	return "enabled", nil
}

func (s *Service) uninstall(ctx context.Context, cmd Command) (string, error) {
	report, err := s.store.DependencyReport(ctx, cmd.ProviderID)
	if err != nil {
		return "dependency_check_failed", err
	}
	if err := EnsureUninstallAllowed(report); err != nil {
		return "blocked", err
	}
	if err := s.store.DeleteSnapshots(ctx, cmd.ProviderID, cmd.ProviderVersion); err != nil {
		return "delete_failed", err
	}
	if err := s.store.SaveLifecycleState(ctx, cmd, "disabled"); err != nil {
		return "state_failed", err
	}
	return "disabled", nil
}

func (s *Service) verify(ctx context.Context, cmd Command) (Manifest, error) {
	if s.verifier == nil {
		return Manifest{}, errors.New("manifest verifier is required")
	}
	manifest, err := s.verifier.VerifyManifest(ctx, cmd)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.ProviderID != cmd.ProviderID || manifest.ProviderVersion != cmd.ProviderVersion || manifest.BundleDigest != cmd.BundleDigest {
		return Manifest{}, fmt.Errorf("%w: manifest identity mismatch", ErrInvalidCommand)
	}
	if s.compatibility != nil {
		if err := s.compatibility.Check(ctx, manifest); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func snapshotFromManifest(manifest Manifest, active bool) Snapshot {
	return Snapshot{ProviderID: manifest.ProviderID, ProviderVersion: manifest.ProviderVersion, Capabilities: append([]string(nil), manifest.Capabilities...), Routes: append([]NavigationRoute(nil), manifest.Routes...), Active: active}
}

func recordConformance(manifest Manifest) {
	if manifest.ConformanceUntil != nil {
		lifecycleConformanceExpiry.WithLabelValues(manifest.ProviderID, manifest.ProviderVersion).Set(float64(manifest.ConformanceUntil.Unix()))
	}
}

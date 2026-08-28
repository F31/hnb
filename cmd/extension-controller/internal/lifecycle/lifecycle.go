package lifecycle

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	SubjectLifecycleRequested = "hnb.provider.lifecycle.requested.v1"
	SubjectLifecycleChanged   = "hnb.provider.lifecycle.changed.v1"
)

var (
	ErrInvalidCommand    = errors.New("invalid provider lifecycle command")
	ErrUninstallBlocked  = errors.New("provider uninstall is blocked")
	digestPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	secretLikeFieldNames = []string{"secret", "token", "password", "kubeconfig", "credential"}
)

type Action string

const (
	ActionInstall   Action = "install"
	ActionEnable    Action = "enable"
	ActionUpgrade   Action = "upgrade"
	ActionRollback  Action = "rollback"
	ActionUninstall Action = "uninstall"
)

type Command struct {
	ProviderID       string   `json:"provider_id"`
	ProviderVersion  string   `json:"provider_version"`
	Action           Action   `json:"action"`
	BundleDigest     string   `json:"bundle_digest"`
	OperationID      string   `json:"operation_id"`
	IdempotencyKey   string   `json:"idempotency_key"`
	CapabilityIDs    []string `json:"capability_ids,omitempty"`
	SecretReferences []string `json:"secret_references,omitempty"`
}

type Event struct {
	ProviderID      string    `json:"provider_id"`
	ProviderVersion string    `json:"provider_version"`
	Action          Action    `json:"action"`
	Phase           string    `json:"phase"`
	BundleDigest    string    `json:"bundle_digest"`
	OperationID     string    `json:"operation_id"`
	OccurredAt      time.Time `json:"occurred_at"`
}

type Snapshot struct {
	ProviderID      string
	ProviderVersion string
	Capabilities    []string
	Routes          []NavigationRoute
	Active          bool
}

type NavigationRoute struct {
	Path       string
	Permission string
	MenuTitle  string
}

type DependencyReport struct {
	ActiveOperations   int
	RuntimeTargets     int
	Capabilities       int
	ReleasePlans       int
	NavigationRoutes   int
	ProtectedResources int
}

func (r DependencyReport) Blockers() []string {
	blockers := make([]string, 0)
	if r.ActiveOperations > 0 {
		blockers = append(blockers, "active_operations")
	}
	if r.RuntimeTargets > 0 {
		blockers = append(blockers, "runtime_targets")
	}
	if r.Capabilities > 0 {
		blockers = append(blockers, "capabilities")
	}
	if r.ReleasePlans > 0 {
		blockers = append(blockers, "release_plans")
	}
	if r.NavigationRoutes > 0 {
		blockers = append(blockers, "navigation_routes")
	}
	if r.ProtectedResources > 0 {
		blockers = append(blockers, "protected_resources")
	}
	return blockers
}

func ValidateCommand(cmd Command) error {
	if cmd.ProviderID == "" || cmd.ProviderVersion == "" || cmd.OperationID == "" || cmd.IdempotencyKey == "" {
		return fmt.Errorf("%w: provider_id, provider_version, operation_id and idempotency_key are required", ErrInvalidCommand)
	}
	if !digestPattern.MatchString(cmd.BundleDigest) {
		return fmt.Errorf("%w: bundle_digest must be sha256", ErrInvalidCommand)
	}
	switch cmd.Action {
	case ActionInstall, ActionEnable, ActionUpgrade, ActionRollback, ActionUninstall:
	default:
		return fmt.Errorf("%w: unsupported action %q", ErrInvalidCommand, cmd.Action)
	}
	for _, ref := range cmd.SecretReferences {
		if strings.Contains(ref, "=") || strings.HasPrefix(strings.TrimSpace(ref), "{") {
			return fmt.Errorf("%w: inline secret values are not allowed", ErrInvalidCommand)
		}
	}
	return nil
}

func ValidateEventFields(fields map[string]any) error {
	for key := range fields {
		lower := strings.ToLower(key)
		for _, word := range secretLikeFieldNames {
			if strings.Contains(lower, word) && word != "secret" {
				return fmt.Errorf("%w: secret-like field %s", ErrInvalidCommand, key)
			}
			if strings.Contains(lower, word) && word == "secret" && key != "secret_references" {
				return fmt.Errorf("%w: secret-like field %s", ErrInvalidCommand, key)
			}
		}
	}
	return nil
}

func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(attempt*attempt) * time.Second
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

func PromoteCandidate(active, candidate Snapshot, healthy bool) (Snapshot, Snapshot, error) {
	if !healthy {
		return active, candidate, errors.New("candidate health check failed")
	}
	active.Active = false
	candidate.Active = true
	return candidate, active, nil
}

func EnsureUninstallAllowed(report DependencyReport) error {
	if blockers := report.Blockers(); len(blockers) > 0 {
		return fmt.Errorf("%w: %s", ErrUninstallBlocked, strings.Join(blockers, ","))
	}
	return nil
}

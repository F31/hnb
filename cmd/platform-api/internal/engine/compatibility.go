package engine

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	CodeTargetActionUnsupported = "TARGET_ACTION_UNSUPPORTED"
	CodeProviderRouteNotFound   = "PROVIDER_ROUTE_NOT_FOUND"
	CodeProviderIncompatible    = "PROVIDER_INCOMPATIBLE"
)

//go:embed runtime-target-compatibility-matrix.json
var defaultCompatibilityMatrixJSON []byte

type compatibilityMatrixDocument struct {
	SchemaVersion           string                   `json:"schemaVersion"`
	MatrixVersion           string                   `json:"matrixVersion"`
	ProviderProtocolVersion string                   `json:"providerProtocolVersion"`
	EffectiveAt             time.Time                `json:"effectiveAt"`
	ExpiresAt               time.Time                `json:"expiresAt"`
	Rows                    []compatibilityMatrixRow `json:"rows"`
}

type compatibilityMatrixRow struct {
	TargetKind        string            `json:"targetKind"`
	ProviderID        string            `json:"providerId"`
	ObservationSource string            `json:"observationSource"`
	Actions           map[string]string `json:"actions"`
}

type CompatibilityDecision struct {
	MatrixVersion           string    `json:"matrixVersion"`
	ProviderProtocolVersion string    `json:"providerProtocolVersion"`
	TargetKind              string    `json:"targetKind"`
	Action                  string    `json:"action"`
	Status                  string    `json:"status"`
	ProviderID              string    `json:"providerId"`
	ObservationSource       string    `json:"observationSource"`
	EffectiveAt             time.Time `json:"effectiveAt"`
	ExpiresAt               time.Time `json:"expiresAt"`
}

type ProviderResolution struct {
	ProviderID       string
	ProviderVersion  string
	ProviderDigest   string
	EvidenceRef      string
	PackageID        string
	PackageVersion   string
	PackageDigest    string
	Provisioners     []string
	CapabilityClaims map[string]any
	RollbackVersion  string
}

type LifecycleProviderResolver interface {
	ResolveLifecycleProvider(context.Context, CompatibilityDecision) (ProviderResolution, error)
}

type StorageProviderResolver interface {
	ResolveStorageProvider(context.Context, string) (ProviderResolution, error)
	ResolveStorageDriverProvider(context.Context, StorageDriverRequest) (ProviderResolution, error)
	ResolveRetainedVolumeProvider(context.Context, RetainedVolumeProviderRequest) (ProviderResolution, error)
}

type StorageDriverRequest struct {
	Action, PackageID, PackageVersion, CurrentVersion, KubernetesVersion string
}

type RetainedVolumeProviderRequest struct {
	Action, ProviderID string
}

type CompatibilityError struct {
	Code   string
	Reason string
}

func (e *CompatibilityError) Error() string { return e.Reason }

func CompatibilityErrorCode(err error) (string, bool) {
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) {
		return "", false
	}
	return compatibility.Code, true
}

type CompatibilityMatrix struct {
	document compatibilityMatrixDocument
	now      func() time.Time
}

func NewCompatibilityMatrix(data []byte, now func() time.Time) (*CompatibilityMatrix, error) {
	var document compatibilityMatrixDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode compatibility matrix: %w", err)
	}
	if now == nil {
		now = time.Now
	}
	if document.SchemaVersion == "" || document.MatrixVersion == "" || document.ProviderProtocolVersion == "" ||
		document.EffectiveAt.IsZero() || !document.ExpiresAt.After(document.EffectiveAt) || len(document.Rows) != 2 {
		return nil, errors.New("compatibility matrix metadata is invalid")
	}
	wantedActions := []string{"create", "import", "upgrade", "unmanage"}
	seen := make(map[string]struct{}, len(document.Rows))
	for _, row := range document.Rows {
		if (row.TargetKind != "KubernetesTarget" && row.TargetKind != "EdgeRuntimeTarget") || row.ProviderID == "" ||
			(row.ObservationSource != "Agent" && row.ObservationSource != "CloudCore") || len(row.Actions) != len(wantedActions) {
			return nil, errors.New("compatibility matrix row is invalid")
		}
		if _, duplicate := seen[row.TargetKind]; duplicate {
			return nil, errors.New("compatibility matrix target kind is duplicated")
		}
		seen[row.TargetKind] = struct{}{}
		for _, action := range wantedActions {
			if row.Actions[action] != "REQUIRED" && row.Actions[action] != "UNSUPPORTED" {
				return nil, errors.New("compatibility matrix action cell is invalid")
			}
		}
	}
	return &CompatibilityMatrix{document: document, now: now}, nil
}

func DefaultCompatibilityMatrix() *CompatibilityMatrix {
	matrix, err := NewCompatibilityMatrix(defaultCompatibilityMatrixJSON, time.Now)
	if err != nil {
		panic(err)
	}
	return matrix
}

func (m *CompatibilityMatrix) Evaluate(intentKind IntentKind, targetKind string) (CompatibilityDecision, error) {
	action, ok := lifecycleAction(intentKind)
	if !ok {
		return CompatibilityDecision{}, &CompatibilityError{Code: CodeTargetActionUnsupported, Reason: "intent kind has no lifecycle compatibility action"}
	}
	now := m.now().UTC()
	if now.Before(m.document.EffectiveAt) || !now.Before(m.document.ExpiresAt) {
		return CompatibilityDecision{}, &CompatibilityError{Code: CodeProviderRouteNotFound, Reason: "runtime target compatibility matrix is not current"}
	}
	for _, row := range m.document.Rows {
		if row.TargetKind != targetKind {
			continue
		}
		decision := CompatibilityDecision{
			MatrixVersion: m.document.MatrixVersion, ProviderProtocolVersion: m.document.ProviderProtocolVersion,
			TargetKind: row.TargetKind, Action: action, Status: row.Actions[action], ProviderID: row.ProviderID,
			ObservationSource: row.ObservationSource, EffectiveAt: m.document.EffectiveAt, ExpiresAt: m.document.ExpiresAt,
		}
		if decision.Status == "UNSUPPORTED" {
			return decision, &CompatibilityError{Code: CodeTargetActionUnsupported, Reason: "target kind does not support the requested lifecycle action"}
		}
		return decision, nil
	}
	return CompatibilityDecision{}, &CompatibilityError{Code: CodeTargetActionUnsupported, Reason: "target kind is not present in the compatibility matrix"}
}

func (m *CompatibilityMatrix) Decisions() []CompatibilityDecision {
	now := m.now().UTC()
	if now.Before(m.document.EffectiveAt) || !now.Before(m.document.ExpiresAt) {
		return nil
	}
	decisions := make([]CompatibilityDecision, 0, len(m.document.Rows)*4)
	for _, row := range m.document.Rows {
		for _, action := range []string{"create", "import", "upgrade", "unmanage"} {
			decisions = append(decisions, CompatibilityDecision{
				MatrixVersion: m.document.MatrixVersion, ProviderProtocolVersion: m.document.ProviderProtocolVersion,
				TargetKind: row.TargetKind, Action: action, Status: row.Actions[action], ProviderID: row.ProviderID,
				ObservationSource: row.ObservationSource, EffectiveAt: m.document.EffectiveAt, ExpiresAt: m.document.ExpiresAt,
			})
		}
	}
	return decisions
}

func lifecycleAction(kind IntentKind) (string, bool) {
	switch kind {
	case IntentCreateKubernetesTarget:
		return "create", true
	case IntentImportRuntimeTarget:
		return "import", true
	case IntentUpgradeRuntimeTarget:
		return "upgrade", true
	case IntentDeleteRuntimeTarget:
		return "unmanage", true
	default:
		return "", false
	}
}

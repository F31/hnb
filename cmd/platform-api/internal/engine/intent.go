package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/F31/hnb/pkg/core"
	"github.com/google/uuid"
)

// IntentKind enum values matching the runtime-intent.schema.json.
type IntentKind string

const (
	IntentInstallRelease      IntentKind = "InstallRelease"
	IntentUninstallRelease    IntentKind = "UninstallRelease"
	IntentUpgradeRelease      IntentKind = "UpgradeRelease"
	IntentRollbackRelease     IntentKind = "RollbackRelease"
	IntentChangeConfiguration IntentKind = "ChangeConfiguration"

	// Cluster-management kinds (RT-001 / RT-006): typed runtime mutations for
	// KubernetesTarget / EdgeRuntimeTarget lifecycle. These do not reference a
	// Release; planning uses kind-specific steps instead.
	IntentCreateKubernetesTarget       IntentKind = "CreateKubernetesTarget"
	IntentImportRuntimeTarget          IntentKind = "ImportRuntimeTarget"
	IntentUpgradeRuntimeTarget         IntentKind = "UpgradeRuntimeTarget"
	IntentDeleteRuntimeTarget          IntentKind = "DeleteRuntimeTarget"
	IntentImportStorageClassBinding    IntentKind = "ImportStorageClassBinding"
	IntentReconcileStorageClassBinding IntentKind = "ReconcileStorageClassBinding"
	IntentInstallStorageDriver         IntentKind = "InstallStorageDriver"
	IntentUpgradeStorageDriver         IntentKind = "UpgradeStorageDriver"
	IntentUninstallStorageDriver       IntentKind = "UninstallStorageDriver"
	IntentReleaseRetainedVolume        IntentKind = "ReleaseRetainedVolume"
	IntentSanitizeRetainedVolume       IntentKind = "SanitizeRetainedVolume"
)

var intentKindValid = map[IntentKind]bool{
	IntentInstallRelease:               true,
	IntentUninstallRelease:             true,
	IntentUpgradeRelease:               true,
	IntentRollbackRelease:              true,
	IntentChangeConfiguration:          true,
	IntentCreateKubernetesTarget:       true,
	IntentImportRuntimeTarget:          true,
	IntentUpgradeRuntimeTarget:         true,
	IntentDeleteRuntimeTarget:          true,
	IntentImportStorageClassBinding:    true,
	IntentReconcileStorageClassBinding: true,
	IntentInstallStorageDriver:         true,
	IntentUpgradeStorageDriver:         true,
	IntentUninstallStorageDriver:       true,
	IntentReleaseRetainedVolume:        true,
	IntentSanitizeRetainedVolume:       true,
}

// requiresReleaseID reports whether the intent kind is a Release-scoped
// operation that must carry a releaseId. Cluster-management kinds are
// target-scoped and must not require one.
func requiresReleaseID(kind IntentKind) bool {
	switch kind {
	case IntentCreateKubernetesTarget, IntentImportRuntimeTarget,
		IntentUpgradeRuntimeTarget, IntentDeleteRuntimeTarget,
		IntentImportStorageClassBinding, IntentReconcileStorageClassBinding:
		return false
	case IntentInstallStorageDriver, IntentUpgradeStorageDriver, IntentUninstallStorageDriver:
		return false
	case IntentReleaseRetainedVolume, IntentSanitizeRetainedVolume:
		return false
	default:
		return true
	}
}

var apiVersion = regexp.MustCompile(`^hnb\.io/v1$`)

var forbiddenIntentParamKeys = map[string]bool{
	"steps":             true,
	"command":           true,
	"commands":          true,
	"providerid":        true,
	"providercommand":   true,
	"providercommands":  true,
	"credential":        true,
	"credentials":       true,
	"targetcredential":  true,
	"targetcredentials": true,
	"artifactbytes":     true,
	"fencing":           true,
	"fencingtoken":      true,
	"policyresult":      true,
	"policyresults":     true,
	"approvalresult":    true,
	"approvalresults":   true,
}

// RuntimeIntent is the parsed client request matching the runtime-intent schema.
type RuntimeIntent struct {
	APIVersion string         `json:"apiVersion"`
	Kind       IntentKind     `json:"kind"`
	Metadata   IntentMetadata `json:"metadata"`
	Spec       IntentSpec     `json:"spec"`
	rawBody    []byte
}

type IntentMetadata struct {
	IdempotencyKey string `json:"idempotencyKey"`
	CorrelationID  string `json:"correlationId"`
}

type IntentSpec struct {
	ReleaseID                   string                     `json:"releaseId"`
	TargetRef                   string                     `json:"targetRef"`
	ScopeRef                    string                     `json:"scopeRef"`
	TargetID                    string                     `json:"targetId,omitempty"`
	TargetKind                  string                     `json:"targetKind,omitempty"`
	ExpectedVersion             int64                      `json:"expectedVersion,omitempty"`
	BindingID                   string                     `json:"bindingId,omitempty"`
	BindingVersion              int64                      `json:"bindingVersion,omitempty"`
	OfferingID                  string                     `json:"offeringId,omitempty"`
	OfferingVersion             int64                      `json:"offeringVersion,omitempty"`
	StorageClassName            string                     `json:"storageClassName,omitempty"`
	StorageClassUID             string                     `json:"storageClassUid,omitempty"`
	StorageClassResourceVersion string                     `json:"storageClassResourceVersion,omitempty"`
	InstallationID              string                     `json:"installationId,omitempty"`
	PackageID                   string                     `json:"packageId,omitempty"`
	PackageVersion              string                     `json:"packageVersion,omitempty"`
	CurrentVersion              string                     `json:"currentVersion,omitempty"`
	DesiredVersion              string                     `json:"desiredVersion,omitempty"`
	DisplayName                 string                     `json:"displayName,omitempty"`
	KubernetesVersion           string                     `json:"kubernetesVersion,omitempty"`
	CloudCoreEndpoint           string                     `json:"cloudCoreEndpoint,omitempty"`
	CredentialSecretRef         *SecretReferenceEntry      `json:"credentialSecretRef,omitempty"`
	NodeGroupMappings           map[string]string          `json:"nodeGroupMappings,omitempty"`
	RiskConfirmation            *RiskConfirmation          `json:"riskConfirmation,omitempty"`
	Parameters                  map[string]any             `json:"parameters,omitempty"`
	SecretReferences            []SecretReferenceEntry     `json:"secretReferences,omitempty"`
	VolumeID                    string                     `json:"volumeId,omitempty"`
	WorkflowProviderRef         string                     `json:"workflowProviderRef,omitempty"`
	PersistentVolume            RetainedVolumeResource     `json:"persistentVolume,omitempty"`
	PersistentVolumeClaim       RetainedVolumeResource     `json:"persistentVolumeClaim,omitempty"`
	PodDependencies             []RetainedVolumeDependency `json:"podDependencies,omitempty"`
	StatefulSetDependencies     []RetainedVolumeDependency `json:"statefulSetDependencies,omitempty"`
}

type RetainedVolumeResource struct {
	Namespace, Name, UID, ResourceVersion, Phase, ReclaimPolicy string
	DeletionObserved                                            bool `json:"deletionObserved,omitempty"`
}

func (r *RetainedVolumeResource) UnmarshalJSON(data []byte) error {
	type resource RetainedVolumeResource
	var value struct {
		Namespace        string `json:"namespace"`
		Name             string `json:"name"`
		UID              string `json:"uid"`
		ResourceVersion  string `json:"resourceVersion"`
		Phase            string `json:"phase"`
		ReclaimPolicy    string `json:"reclaimPolicy"`
		DeletionObserved bool   `json:"deletionObserved"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = RetainedVolumeResource{Namespace: value.Namespace, Name: value.Name, UID: value.UID, ResourceVersion: value.ResourceVersion, Phase: value.Phase, ReclaimPolicy: value.ReclaimPolicy, DeletionObserved: value.DeletionObserved}
	return nil
}

func (r RetainedVolumeResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Namespace        string `json:"namespace,omitempty"`
		Name             string `json:"name"`
		UID              string `json:"uid"`
		ResourceVersion  string `json:"resourceVersion"`
		Phase            string `json:"phase,omitempty"`
		ReclaimPolicy    string `json:"reclaimPolicy,omitempty"`
		DeletionObserved bool   `json:"deletionObserved,omitempty"`
	}{r.Namespace, r.Name, r.UID, r.ResourceVersion, r.Phase, r.ReclaimPolicy, r.DeletionObserved})
}

type RetainedVolumeDependency struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
}

type RiskConfirmation struct {
	Acknowledged bool   `json:"acknowledged"`
	Confirmation string `json:"confirmation"`
	Reason       string `json:"reason,omitempty"`
}

type SecretReferenceEntry struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

func IsClusterIntentKind(kind IntentKind) bool {
	return kind == IntentCreateKubernetesTarget || kind == IntentImportRuntimeTarget ||
		kind == IntentUpgradeRuntimeTarget || kind == IntentDeleteRuntimeTarget
}

func IsStorageIntentKind(kind IntentKind) bool {
	return kind == IntentImportStorageClassBinding || kind == IntentReconcileStorageClassBinding || IsStorageDriverIntentKind(kind) || IsRetainedVolumeIntentKind(kind)
}

func IsRetainedVolumeIntentKind(kind IntentKind) bool {
	return kind == IntentReleaseRetainedVolume || kind == IntentSanitizeRetainedVolume
}

func IsStorageDriverIntentKind(kind IntentKind) bool {
	return kind == IntentInstallStorageDriver || kind == IntentUpgradeStorageDriver || kind == IntentUninstallStorageDriver
}

// ParseRuntimeIntent decodes JSON into a RuntimeIntent and validates the
// structural contract. Returns intent validation errors only — no planning.
func ParseRuntimeIntent(body []byte) (*RuntimeIntent, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse body: %w", err)
	}

	intent := &RuntimeIntent{rawBody: body}

	// apiVersion required.
	if av, ok := raw["apiVersion"]; ok {
		if err := json.Unmarshal(av, &intent.APIVersion); err != nil {
			return nil, fmt.Errorf("apiVersion: %w", err)
		}
	} else {
		return nil, &ValidationError{Field: "apiVersion", Reason: "required"}
	}
	if !apiVersion.MatchString(intent.APIVersion) {
		return nil, &ValidationError{Field: "apiVersion", Reason: "must be hnb.io/v1"}
	}

	// kind required and must be in enum.
	if k, ok := raw["kind"]; ok {
		if err := json.Unmarshal(k, &intent.Kind); err != nil {
			return nil, fmt.Errorf("kind: %w", err)
		}
	} else {
		return nil, &ValidationError{Field: "kind", Reason: "required"}
	}
	if !intentKindValid[intent.Kind] {
		return nil, &ValidationError{Field: "kind", Reason: fmt.Sprintf("unsupported kind %q", intent.Kind)}
	}

	// metadata required.
	if md, ok := raw["metadata"]; ok {
		if err := json.Unmarshal(md, &intent.Metadata); err != nil {
			return nil, fmt.Errorf("metadata: %w", err)
		}
	} else {
		return nil, &ValidationError{Field: "metadata", Reason: "required"}
	}
	if strings.TrimSpace(intent.Metadata.IdempotencyKey) == "" {
		return nil, &ValidationError{Field: "metadata.idempotencyKey", Reason: "required, non-empty"}
	}
	if len(intent.Metadata.IdempotencyKey) > 128 {
		return nil, &ValidationError{Field: "metadata.idempotencyKey", Reason: "max 128 characters"}
	}
	if intent.Metadata.CorrelationID != "" {
		if _, err := uuid.Parse(intent.Metadata.CorrelationID); err != nil {
			return nil, &ValidationError{Field: "metadata.correlationId", Reason: "must be a valid UUID"}
		}
	}

	// spec required.
	if sp, ok := raw["spec"]; ok {
		var specRaw map[string]json.RawMessage
		if err := json.Unmarshal(sp, &specRaw); err != nil {
			return nil, fmt.Errorf("spec: %w", err)
		}
		specFields := map[string]bool{
			"releaseId": true, "targetRef": true, "scopeRef": true,
			"targetId": true, "targetKind": true, "expectedVersion": true, "desiredVersion": true,
			"bindingId": true, "bindingVersion": true, "offeringId": true, "offeringVersion": true,
			"storageClassName": true, "storageClassUid": true, "storageClassResourceVersion": true,
			"installationId": true, "packageId": true, "packageVersion": true, "currentVersion": true,
			"displayName": true, "kubernetesVersion": true, "cloudCoreEndpoint": true,
			"credentialSecretRef": true, "nodeGroupMappings": true,
			"riskConfirmation": true,
			"parameters":       true, "secretReferences": true,
		}
		for k := range specRaw {
			if !specFields[k] {
				return nil, &ValidationError{Field: "spec", Reason: fmt.Sprintf("unknown field %q is not permitted", k)}
			}
		}
		if err := json.Unmarshal(sp, &intent.Spec); err != nil {
			return nil, fmt.Errorf("spec: %w", err)
		}
	} else {
		return nil, &ValidationError{Field: "spec", Reason: "required"}
	}
	if requiresReleaseID(intent.Kind) && strings.TrimSpace(intent.Spec.ReleaseID) == "" {
		return nil, &ValidationError{Field: "spec.releaseId", Reason: "required for release-scoped intents"}
	}
	if requiresReleaseID(intent.Kind) {
		if strings.TrimSpace(intent.Spec.TargetRef) == "" {
			return nil, &ValidationError{Field: "spec.targetRef", Reason: "required"}
		}
		if strings.TrimSpace(intent.Spec.ScopeRef) == "" {
			return nil, &ValidationError{Field: "spec.scopeRef", Reason: "required"}
		}
	} else if IsStorageDriverIntentKind(intent.Kind) {
		if err := validateStorageDriverIntentSpec(intent); err != nil {
			return nil, err
		}
	} else if IsRetainedVolumeIntentKind(intent.Kind) {
		if err := validateRetainedVolumeIntentSpec(intent); err != nil {
			return nil, err
		}
	} else if IsStorageIntentKind(intent.Kind) {
		if err := validateStorageIntentSpec(intent); err != nil {
			return nil, err
		}
	} else if err := validateClusterIntentSpec(intent); err != nil {
		return nil, err
	}
	if len(intent.Spec.ReleaseID) > 256 || len(intent.Spec.TargetRef) > 256 || len(intent.Spec.ScopeRef) > 256 {
		return nil, &ValidationError{Field: "spec", Reason: "fields exceed max length 256"}
	}
	if intent.Spec.RiskConfirmation != nil {
		if !intent.Spec.RiskConfirmation.Acknowledged || len(intent.Spec.RiskConfirmation.Confirmation) < 16 || len(intent.Spec.RiskConfirmation.Confirmation) > 2048 || len(intent.Spec.RiskConfirmation.Reason) > 512 {
			return nil, &ValidationError{Field: "spec.riskConfirmation", Reason: "invalid confirmation"}
		}
	}

	// parameters forbidden key check.
	for key := range intent.Spec.Parameters {
		lower := strings.ToLower(key)
		if forbiddenIntentParamKeys[lower] {
			return nil, &ValidationError{Field: "spec.parameters", Reason: fmt.Sprintf("forbidden key %q is not allowed", key)}
		}
	}
	if len(intent.Spec.Parameters) > 64 {
		return nil, &ValidationError{Field: "spec.parameters", Reason: "max 64 properties"}
	}

	// secretReferences validated.
	if len(intent.Spec.SecretReferences) > 32 {
		return nil, &ValidationError{Field: "spec.secretReferences", Reason: "max 32 items"}
	}

	return intent, nil
}

// ValidateNoExtraFields checks that the raw JSON contains only the four known
// top-level keys to catch unknown-field injection attacks.
func (i *RuntimeIntent) ValidateNoExtraFields(rawBody []byte) error {
	if len(rawBody) == 0 {
		return nil
	}
	var keys []string
	dec := json.NewDecoder(strings.NewReader(string(rawBody)))
	t, err := dec.Token()
	if err != nil {
		return fmt.Errorf("parse top-level object: %w", err)
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected object")
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return fmt.Errorf("decode key: %w", err)
		}
		keys = append(keys, key.(string))
		if dec.More() {
			var v any
			if err := dec.Decode(&v); err != nil {
				break
			}
		} else {
			break
		}
	}
	expected := map[string]bool{"apiVersion": true, "kind": true, "metadata": true, "spec": true}
	for _, k := range keys {
		if !expected[k] {
			return fmt.Errorf("unknown field %q is not permitted", k)
		}
	}
	return nil
}

// ComputeIntentDigest returns a sha256 digest over the canonical intent fields
// used for audit linkage (SEM-001).
func (i *RuntimeIntent) ComputeIntentDigest() string {
	document := core.IntentSemanticDocument{
		APIVersion: i.APIVersion, Kind: string(i.Kind), ReleaseID: i.Spec.ReleaseID,
		TargetRef: i.Spec.TargetRef, ScopeRef: i.Spec.ScopeRef, TargetID: i.Spec.TargetID,
		TargetKind: i.Spec.TargetKind, ExpectedVersion: i.Spec.ExpectedVersion,
		BindingID: i.Spec.BindingID, BindingVersion: i.Spec.BindingVersion,
		OfferingID: i.Spec.OfferingID, OfferingVersion: i.Spec.OfferingVersion,
		StorageClassName: i.Spec.StorageClassName, StorageClassUID: i.Spec.StorageClassUID,
		StorageClassVersion: i.Spec.StorageClassResourceVersion,
		InstallationID:      i.Spec.InstallationID, PackageID: i.Spec.PackageID,
		PackageVersion: i.Spec.PackageVersion, CurrentVersion: i.Spec.CurrentVersion,
		DesiredVersion: i.Spec.DesiredVersion, DisplayName: i.Spec.DisplayName,
		KubernetesVersion: i.Spec.KubernetesVersion, CloudCoreEndpoint: i.Spec.CloudCoreEndpoint,
		NodeGroupMappings: i.Spec.NodeGroupMappings, Parameters: i.Spec.Parameters,
	}
	if IsRetainedVolumeIntentKind(i.Kind) {
		document.VolumeID, document.WorkflowProviderRef = i.Spec.VolumeID, i.Spec.WorkflowProviderRef
		document.PersistentVolume, document.PersistentVolumeClaim = i.Spec.PersistentVolume, i.Spec.PersistentVolumeClaim
		document.PodDependencies, document.StatefulSetDependencies = i.Spec.PodDependencies, i.Spec.StatefulSetDependencies
	}
	if i.Spec.CredentialSecretRef != nil {
		ref := canonicalSecretReference(*i.Spec.CredentialSecretRef)
		document.CredentialSecretRef = &ref
	}
	for _, ref := range i.Spec.SecretReferences {
		document.SecretReferences = append(document.SecretReferences, canonicalSecretReference(ref))
	}
	return core.IntentSemanticDigest(document)
}

func canonicalSecretReference(ref SecretReferenceEntry) core.IntentSecretReference {
	return core.IntentSecretReference{Provider: ref.Provider, Scope: ref.Scope, Name: ref.Name, Version: ref.Version}
}

func sortedParams(params map[string]any) []map[string]any {
	if params == nil {
		return nil
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(params))
	for _, k := range keys {
		out = append(out, map[string]any{"k": k, "v": params[k]})
	}
	return out
}

// ValidationError represents a structural / contract validation failure.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

// IsValidation checks whether err is a ValidationError.
func IsValidation(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

// IntentValidator enforces business rules beyond the JSON schema for RuntimeIntents.
type IntentValidator struct{}

func NewIntentValidator() *IntentValidator {
	return &IntentValidator{}
}

// Validate runs all structural and business-rule checks on the intent.
func (v *IntentValidator) Validate(intent *RuntimeIntent) error {
	if intent.Kind == "" {
		return &ValidationError{Field: "kind", Reason: "required"}
	}
	if requiresReleaseID(intent.Kind) && strings.TrimSpace(intent.Spec.ReleaseID) == "" {
		return &ValidationError{Field: "spec.releaseId", Reason: "required for release-scoped intents"}
	}
	if requiresReleaseID(intent.Kind) {
		if strings.TrimSpace(intent.Spec.TargetRef) == "" {
			return &ValidationError{Field: "spec.targetRef", Reason: "required"}
		}
		if strings.TrimSpace(intent.Spec.ScopeRef) == "" {
			return &ValidationError{Field: "spec.scopeRef", Reason: "required"}
		}
	} else if IsStorageDriverIntentKind(intent.Kind) {
		if err := validateStorageDriverIntentSpec(intent); err != nil {
			return err
		}
	} else if IsRetainedVolumeIntentKind(intent.Kind) {
		if err := validateRetainedVolumeIntentSpec(intent); err != nil {
			return err
		}
	} else if IsStorageIntentKind(intent.Kind) {
		if err := validateStorageIntentSpec(intent); err != nil {
			return err
		}
	} else if err := validateClusterIntentSpec(intent); err != nil {
		return err
	}
	if len(intent.Spec.ReleaseID) > 256 || len(intent.Spec.TargetRef) > 256 || len(intent.Spec.ScopeRef) > 256 {
		return &ValidationError{Field: "spec", Reason: "field exceeds max length 256"}
	}
	for key := range intent.Spec.Parameters {
		if forbiddenIntentParamKeys[strings.ToLower(key)] {
			return &ValidationError{Field: "spec.parameters", Reason: fmt.Sprintf("forbidden key %q is not allowed", key)}
		}
	}
	if len(intent.Spec.Parameters) > 64 {
		return &ValidationError{Field: "spec.parameters", Reason: "max 64 properties"}
	}
	if len(intent.Spec.SecretReferences) > 32 {
		return &ValidationError{Field: "spec.secretReferences", Reason: "max 32 items"}
	}
	for i, ref := range intent.Spec.SecretReferences {
		if strings.TrimSpace(ref.Name) == "" {
			return &ValidationError{Field: fmt.Sprintf("spec.secretReferences[%d].name", i), Reason: "required"}
		}
	}
	return nil
}

func validateRetainedVolumeIntentSpec(intent *RuntimeIntent) error {
	if _, err := uuid.Parse(intent.Spec.TargetID); err != nil || intent.Spec.TargetKind != "KubernetesTarget" || intent.Spec.ExpectedVersion < 1 {
		return &ValidationError{Field: "spec.targetId", Reason: "a fenced KubernetesTarget is required"}
	}
	if !boundedRetainedIdentity(intent.Spec.VolumeID, 128) || !boundedRetainedIdentity(intent.Spec.WorkflowProviderRef, 256) {
		return &ValidationError{Field: "spec.volumeId", Reason: "volume and provider identities are required"}
	}
	if !validRetainedResource(intent.Spec.PersistentVolume, false) || intent.Spec.PersistentVolume.ReclaimPolicy != "Retain" || intent.Spec.PersistentVolume.Phase != "Released" {
		return &ValidationError{Field: "spec.persistentVolume", Reason: "a Released PV with Retain reclaim policy is required"}
	}
	if !validRetainedResource(intent.Spec.PersistentVolumeClaim, true) || !intent.Spec.PersistentVolumeClaim.DeletionObserved {
		return &ValidationError{Field: "spec.persistentVolumeClaim", Reason: "deleted PVC evidence is required"}
	}
	if len(intent.Spec.PodDependencies) != 0 || len(intent.Spec.StatefulSetDependencies) != 0 {
		return &ValidationError{Field: "spec", Reason: "active Pod or StatefulSet dependencies prohibit retained-volume release"}
	}
	if len(intent.Spec.Parameters) != 0 || len(intent.Spec.SecretReferences) != 0 || intent.Spec.CredentialSecretRef != nil {
		return &ValidationError{Field: "spec", Reason: "retained-volume intents accept only typed observation evidence"}
	}
	intent.Spec.TargetRef = intent.Spec.TargetID
	intent.Spec.ScopeRef = "retainedVolume:" + intent.Spec.VolumeID
	return nil
}

func validRetainedResource(value RetainedVolumeResource, namespaceRequired bool) bool {
	return boundedRetainedIdentity(value.Name, 253) && boundedRetainedIdentity(value.UID, 128) && boundedRetainedIdentity(value.ResourceVersion, 128) && (!namespaceRequired || boundedRetainedIdentity(value.Namespace, 253))
}

func boundedRetainedIdentity(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

func validateStorageDriverIntentSpec(intent *RuntimeIntent) error {
	for field, value := range map[string]string{"spec.targetId": intent.Spec.TargetID, "spec.installationId": intent.Spec.InstallationID} {
		if _, err := uuid.Parse(value); err != nil {
			return &ValidationError{Field: field, Reason: "must be a valid UUID"}
		}
	}
	if intent.Spec.TargetKind != "KubernetesTarget" || intent.Spec.ExpectedVersion < 1 {
		return &ValidationError{Field: "spec", Reason: "KubernetesTarget and target version are required"}
	}
	for field, value := range map[string]string{"spec.packageId": intent.Spec.PackageID, "spec.packageVersion": intent.Spec.PackageVersion} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return &ValidationError{Field: field, Reason: "must be a bounded non-empty identity"}
		}
	}
	if strings.TrimSpace(intent.Spec.KubernetesVersion) == "" {
		return &ValidationError{Field: "spec.kubernetesVersion", Reason: "required"}
	}
	if intent.Kind == IntentUpgradeStorageDriver && strings.TrimSpace(intent.Spec.CurrentVersion) == "" {
		return &ValidationError{Field: "spec.currentVersion", Reason: "required for upgrade"}
	}
	if intent.Kind != IntentUpgradeStorageDriver && intent.Spec.CurrentVersion != "" {
		return &ValidationError{Field: "spec.currentVersion", Reason: "only valid for upgrade"}
	}
	if len(intent.Spec.Parameters) > 32 {
		return &ValidationError{Field: "spec.parameters", Reason: "max 32 properties"}
	}
	for key, value := range intent.Spec.Parameters {
		switch typed := value.(type) {
		case nil, bool, float64:
		case string:
			if len(typed) > 4096 || strings.ContainsAny(typed, "\x00\r\n") {
				return &ValidationError{Field: "spec.parameters." + key, Reason: "string value is invalid"}
			}
		default:
			return &ValidationError{Field: "spec.parameters." + key, Reason: "only scalar sanitized values are allowed"}
		}
	}
	intent.Spec.TargetRef = intent.Spec.TargetID
	intent.Spec.ScopeRef = "storageDriverInstallation:" + intent.Spec.InstallationID
	return nil
}

func validateStorageIntentSpec(intent *RuntimeIntent) error {
	for field, value := range map[string]string{
		"spec.targetId": intent.Spec.TargetID, "spec.bindingId": intent.Spec.BindingID,
		"spec.offeringId": intent.Spec.OfferingID,
	} {
		if _, err := uuid.Parse(value); err != nil {
			return &ValidationError{Field: field, Reason: "must be a valid UUID"}
		}
	}
	if intent.Spec.TargetKind != "KubernetesTarget" {
		return &ValidationError{Field: "spec.targetKind", Reason: "must be KubernetesTarget"}
	}
	if intent.Spec.ExpectedVersion < 1 || intent.Spec.OfferingVersion < 1 {
		return &ValidationError{Field: "spec", Reason: "target and offering versions must be at least 1"}
	}
	if intent.Kind == IntentReconcileStorageClassBinding && intent.Spec.BindingVersion < 1 {
		return &ValidationError{Field: "spec.bindingVersion", Reason: "must be at least 1"}
	}
	for field, value := range map[string]string{
		"spec.storageClassName":            intent.Spec.StorageClassName,
		"spec.storageClassUid":             intent.Spec.StorageClassUID,
		"spec.storageClassResourceVersion": intent.Spec.StorageClassResourceVersion,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 253 || strings.ContainsAny(value, "\x00\r\n") {
			return &ValidationError{Field: field, Reason: "must be a bounded non-empty identity"}
		}
	}
	if len(intent.Spec.Parameters) != 0 || len(intent.Spec.SecretReferences) != 0 || intent.Spec.CredentialSecretRef != nil {
		return &ValidationError{Field: "spec", Reason: "storage binding intents accept only typed identity fields and no secret values"}
	}
	intent.Spec.TargetRef = intent.Spec.TargetID
	intent.Spec.ScopeRef = "storageClassBinding:" + intent.Spec.BindingID
	return nil
}

func validateClusterIntentSpec(intent *RuntimeIntent) error {
	if intent.Spec.TargetKind != "KubernetesTarget" && intent.Spec.TargetKind != "EdgeRuntimeTarget" {
		return &ValidationError{Field: "spec.targetKind", Reason: "must be KubernetesTarget or EdgeRuntimeTarget"}
	}
	switch intent.Kind {
	case IntentCreateKubernetesTarget:
		// Edge create reaches the compatibility matrix so the stable
		// TARGET_ACTION_UNSUPPORTED reason remains authoritative.
		if intent.Spec.TargetKind == "KubernetesTarget" {
			if strings.TrimSpace(intent.Spec.DisplayName) == "" {
				return &ValidationError{Field: "spec.displayName", Reason: "required"}
			}
			if intent.Spec.CredentialSecretRef == nil {
				return &ValidationError{Field: "spec.credentialSecretRef", Reason: "required"}
			}
		}
	case IntentImportRuntimeTarget:
		if strings.TrimSpace(intent.Spec.DisplayName) == "" {
			return &ValidationError{Field: "spec.displayName", Reason: "required"}
		}
		if intent.Spec.CredentialSecretRef == nil {
			return &ValidationError{Field: "spec.credentialSecretRef", Reason: "required"}
		}
		if intent.Spec.TargetKind == "EdgeRuntimeTarget" {
			normalized, err := normalizeCloudCoreEndpoint(intent.Spec.CloudCoreEndpoint)
			if err != nil {
				return &ValidationError{Field: "spec.cloudCoreEndpoint", Reason: err.Error()}
			}
			intent.Spec.CloudCoreEndpoint = normalized
		}
	case IntentUpgradeRuntimeTarget, IntentDeleteRuntimeTarget:
		if _, err := uuid.Parse(intent.Spec.TargetID); err != nil {
			return &ValidationError{Field: "spec.targetId", Reason: "must be a valid UUID"}
		}
		if intent.Spec.ExpectedVersion < 1 {
			return &ValidationError{Field: "spec.expectedVersion", Reason: "must be at least 1"}
		}
		intent.Spec.TargetRef = intent.Spec.TargetID
		if intent.Kind == IntentUpgradeRuntimeTarget && strings.TrimSpace(intent.Spec.DesiredVersion) == "" {
			return &ValidationError{Field: "spec.desiredVersion", Reason: "required"}
		}
	}
	if intent.Spec.CredentialSecretRef != nil && strings.TrimSpace(intent.Spec.CredentialSecretRef.Name) == "" {
		return &ValidationError{Field: "spec.credentialSecretRef.name", Reason: "required"}
	}
	return nil
}

func normalizeCloudCoreEndpoint(raw string) (string, error) {
	if raw == "" || len(raw) > 512 || strings.ContainsAny(raw, "\x00\r\n\t") {
		return "", errors.New("must be a bounded absolute URL")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "wss") || u.Hostname() == "" {
		return "", errors.New("must use https or wss with a hostname")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("userinfo, query, and fragment are forbidden")
	}
	if port := u.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", errors.New("port is invalid")
		}
	}
	return u.String(), nil
}

// ValidateWithScope adds tenant-scope validation to standard checks.
func (v *IntentValidator) ValidateWithScope(intent *RuntimeIntent, tenantID string) error {
	if err := v.Validate(intent); err != nil {
		return err
	}
	if tenantID == "" {
		return &ValidationError{Field: "tenantId", Reason: "required for scope validation"}
	}
	return nil
}

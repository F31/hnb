package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type RetryPolicy struct {
	MaxAttempts int    `json:"maxAttempts"`
	Backoff     string `json:"backoff"`
}

type CompensationPolicy struct {
	Type           string `json:"type"`
	OwnershipScope string `json:"ownershipScope"`
}

// Step describes a single executable unit within an ExecutionPlan.
type Step struct {
	StepID                  string                 `json:"stepId"`
	StepType                string                 `json:"stepType"`
	ProviderID              string                 `json:"providerId,omitempty"`
	ProviderVersion         string                 `json:"providerVersion,omitempty"`
	ProviderDigest          string                 `json:"providerDigest,omitempty"`
	ProviderProtocolVersion string                 `json:"providerProtocolVersion,omitempty"`
	DependsOn               []string               `json:"dependsOn"`
	TargetRef               string                 `json:"targetRef,omitempty"`
	TargetKind              string                 `json:"targetKind,omitempty"`
	InputSchema             string                 `json:"inputSchema,omitempty"`
	Inputs                  map[string]any         `json:"inputs,omitempty"`
	SecretReferences        []SecretReferenceEntry `json:"secretReferences,omitempty"`
	IdempotencyKey          string                 `json:"idempotencyKey,omitempty"`
	FencingPolicy           string                 `json:"fencingPolicy,omitempty"`
	RetryPolicy             *RetryPolicy           `json:"retryPolicy,omitempty"`
	TimeoutSeconds          int                    `json:"timeoutSeconds,omitempty"`
	Compensation            *CompensationPolicy    `json:"compensation,omitempty"`
}

type TargetSnapshot struct {
	TargetID          string `json:"targetId"`
	TargetKind        string `json:"targetKind"`
	ProjectionVersion int64  `json:"projectionVersion"`
	ObservationSource string `json:"observationSource"`
}

// ExecutionPlan is the immutable, server-authored plan derived from a RuntimeIntent.
type ExecutionPlan struct {
	PlanID                   string                 `json:"planId"`
	IntentID                 string                 `json:"intentId"`
	SemanticDigest           string                 `json:"semanticDigest"`
	ReleaseRef               string                 `json:"releaseRef"`
	ArtifactDigests          []string               `json:"artifactDigests"`
	TargetRef                string                 `json:"targetRef"`
	CapabilitySnapshotDigest string                 `json:"capabilitySnapshotDigest"`
	ProviderVersions         map[string]string      `json:"providerVersions"`
	PolicyDecisionRefs       []string               `json:"policyDecisionRefs"`
	ApprovedParameters       map[string]any         `json:"approvedParameters"`
	SecretReferences         []SecretReferenceEntry `json:"secretReferences"`
	CompatibilityDecision    *CompatibilityDecision `json:"compatibilityDecision,omitempty"`
	TargetSnapshot           *TargetSnapshot        `json:"targetSnapshot,omitempty"`
	Steps                    []Step                 `json:"steps"`
	CreatedAt                time.Time              `json:"createdAt"`
}

// SemanticDigest returns sha256 digest over the canonical step list for plan integrity.
func (ep *ExecutionPlan) ComputeDigest() string {
	return ep.SemanticDigest
}

type Planner struct {
	matrix    *CompatibilityMatrix
	providers LifecycleProviderResolver
	now       func() time.Time
}

func NewPlanner(providers ...LifecycleProviderResolver) *Planner {
	planner := &Planner{matrix: DefaultCompatibilityMatrix(), now: time.Now}
	if len(providers) > 0 {
		planner.providers = providers[0]
	}
	return planner
}

func NewPlannerWithCompatibility(matrix *CompatibilityMatrix, providers LifecycleProviderResolver) *Planner {
	return &Planner{matrix: matrix, providers: providers, now: time.Now}
}

// Plan generates an ExecutionPlan from the given intent and tenant metadata.
func (p *Planner) Plan(intent *RuntimeIntent, tenantID, projectID, environmentID, namespaceID, subjectID string) (*ExecutionPlan, error) {
	return p.PlanContext(context.Background(), intent, tenantID, projectID, environmentID, namespaceID, subjectID)
}

func (p *Planner) PlanContext(ctx context.Context, intent *RuntimeIntent, tenantID, projectID, environmentID, namespaceID, subjectID string) (*ExecutionPlan, error) {
	steps := p.generateSteps(intent)
	var compatibility *CompatibilityDecision
	var targetSnapshot *TargetSnapshot
	providerVersions := map[string]string{"platform": "1.0"}
	artifactDigests := []string{}
	policyDecisionRefs := []string{}
	if IsClusterIntentKind(intent.Kind) {
		if p.matrix == nil {
			return nil, &CompatibilityError{Code: CodeProviderRouteNotFound, Reason: "runtime target compatibility matrix is unavailable"}
		}
		decision, err := p.matrix.Evaluate(intent.Kind, intent.Spec.TargetKind)
		if err != nil {
			return nil, err
		}
		if p.providers == nil {
			return nil, &CompatibilityError{Code: CodeProviderRouteNotFound, Reason: "lifecycle provider registry is unavailable"}
		}
		provider, err := p.providers.ResolveLifecycleProvider(ctx, decision)
		if err != nil {
			if _, ok := CompatibilityErrorCode(err); ok {
				return nil, err
			}
			return nil, &CompatibilityError{Code: CodeProviderRouteNotFound, Reason: "no eligible lifecycle provider is available"}
		}
		if provider.ProviderID != decision.ProviderID || provider.ProviderVersion == "" || provider.ProviderDigest == "" {
			return nil, &CompatibilityError{Code: CodeProviderIncompatible, Reason: "lifecycle provider resolution is incomplete"}
		}
		steps, targetSnapshot = lifecycleSteps(intent, tenantID, decision, provider)
		compatibility = &decision
		providerVersions = map[string]string{provider.ProviderID: provider.ProviderVersion}
		artifactDigests = []string{provider.ProviderDigest}
		policyDecisionRefs = []string{"matrix:" + decision.MatrixVersion, provider.EvidenceRef}
	} else if IsStorageIntentKind(intent.Kind) {
		resolver, ok := p.providers.(StorageProviderResolver)
		if !ok {
			return nil, &CompatibilityError{Code: CodeProviderRouteNotFound, Reason: "storage provider registry is unavailable"}
		}
		action := "import"
		if intent.Kind == IntentReconcileStorageClassBinding {
			action = "reconcile"
		}
		if IsStorageDriverIntentKind(intent.Kind) {
			action = map[IntentKind]string{IntentInstallStorageDriver: "install", IntentUpgradeStorageDriver: "upgrade", IntentUninstallStorageDriver: "uninstall"}[intent.Kind]
			provider, err := resolver.ResolveStorageDriverProvider(ctx, StorageDriverRequest{Action: action, PackageID: intent.Spec.PackageID, PackageVersion: intent.Spec.PackageVersion, CurrentVersion: intent.Spec.CurrentVersion, KubernetesVersion: intent.Spec.KubernetesVersion})
			if err != nil {
				return nil, err
			}
			steps, targetSnapshot = storageDriverSteps(intent, tenantID, action, provider)
			providerVersions = map[string]string{provider.ProviderID: provider.ProviderVersion}
			artifactDigests = []string{provider.ProviderDigest, provider.PackageDigest}
			policyDecisionRefs = []string{provider.EvidenceRef, "package:" + provider.PackageID + "@" + provider.PackageVersion}
		} else if IsRetainedVolumeIntentKind(intent.Kind) {
			action := "manual-release"
			if intent.Kind == IntentSanitizeRetainedVolume {
				action = "sanitize"
			}
			provider, err := resolver.ResolveRetainedVolumeProvider(ctx, RetainedVolumeProviderRequest{Action: action, ProviderID: intent.Spec.WorkflowProviderRef})
			if err != nil {
				return nil, err
			}
			steps, targetSnapshot = retainedVolumeSteps(intent, tenantID, action, provider)
			providerVersions = map[string]string{provider.ProviderID: provider.ProviderVersion}
			artifactDigests = []string{provider.ProviderDigest}
			policyDecisionRefs = []string{provider.EvidenceRef, "approval:explicit-operation-approval"}
		} else {
			provider, err := resolver.ResolveStorageProvider(ctx, action)
			if err != nil {
				return nil, err
			}
			if provider.ProviderID != "kubernetes-provider" || provider.ProviderVersion == "" || provider.ProviderDigest == "" {
				return nil, &CompatibilityError{Code: CodeProviderIncompatible, Reason: "storage provider resolution is incomplete"}
			}
			steps, targetSnapshot = storageBindingSteps(intent, tenantID, action, provider)
			providerVersions = map[string]string{provider.ProviderID: provider.ProviderVersion}
			artifactDigests = []string{provider.ProviderDigest}
			policyDecisionRefs = []string{provider.EvidenceRef}
		}
	}
	for i := range steps {
		if steps[i].DependsOn == nil {
			steps[i].DependsOn = []string{}
		}
	}
	if err := validateDAG(steps); err != nil {
		return nil, fmt.Errorf("invalid step DAG: %w", err)
	}
	if len(steps) == 0 || len(steps) > 256 {
		return nil, fmt.Errorf("step count %d out of range [1,256]", len(steps))
	}

	digest := computePlanDigest(intent, tenantID, steps, compatibility, targetSnapshot, providerVersions, artifactDigests)

	now := p.now().UTC()
	idemKey := intent.Metadata.IdempotencyKey
	idLen := len(idemKey)
	if idLen > 120 {
		idLen = 120
	}
	// Cluster-management intents do not reference a Release; keep a stable
	// release_ref placeholder so execution_plans.release_id NOT NULL holds.
	releaseRef := intent.Spec.ReleaseID
	targetRef := intent.Spec.TargetRef
	if IsStorageIntentKind(intent.Kind) {
		targetRef = targetSnapshot.TargetID
		if IsStorageDriverIntentKind(intent.Kind) {
			releaseRef = "storage-driver:" + intent.Spec.InstallationID
		} else if IsRetainedVolumeIntentKind(intent.Kind) {
			releaseRef = "retained-volume:" + intent.Spec.VolumeID
		} else {
			releaseRef = "storage-binding:" + intent.Spec.BindingID
		}
	} else if !requiresReleaseID(intent.Kind) {
		targetRef = targetSnapshot.TargetID
		releaseRef = "target:" + targetRef
	}
	approvedParameters := intent.Spec.Parameters
	if approvedParameters == nil {
		approvedParameters = map[string]any{}
	}
	return &ExecutionPlan{
		PlanID:                   fmt.Sprintf("plan-%s", idemKey[:idLen]),
		IntentID:                 subjectID,
		SemanticDigest:           digest,
		ReleaseRef:               releaseRef,
		ArtifactDigests:          artifactDigests,
		TargetRef:                targetRef,
		CapabilitySnapshotDigest: computeCapabilitySnapshotDigest(targetRef, compatibility, providerVersions, artifactDigests),
		ProviderVersions:         providerVersions,
		PolicyDecisionRefs:       policyDecisionRefs,
		ApprovedParameters:       approvedParameters,
		SecretReferences:         lifecycleSecretReferences(intent),
		CompatibilityDecision:    compatibility,
		TargetSnapshot:           targetSnapshot,
		Steps:                    steps,
		CreatedAt:                now,
	}, nil
}

func retainedVolumeSteps(intent *RuntimeIntent, tenantID, action string, provider ProviderResolution) ([]Step, *TargetSnapshot) {
	idempotencyKey := lifecycleStepIdempotencyKey(tenantID, intent)
	inputs := map[string]any{
		"schemaVersion": "1.0.0", "workflow": action, "volumeId": intent.Spec.VolumeID,
		"targetId": intent.Spec.TargetID, "targetProjectionVersion": intent.Spec.ExpectedVersion,
		"persistentVolume": intent.Spec.PersistentVolume, "persistentVolumeClaim": intent.Spec.PersistentVolumeClaim,
		"podDependencies": intent.Spec.PodDependencies, "statefulSetDependencies": intent.Spec.StatefulSetDependencies,
		"approvalPolicy": "explicit-operation-approval", "providerConformanceEvidenceRef": provider.EvidenceRef,
		"idempotencyKey": idempotencyKey, "fencingGeneration": intent.Spec.ExpectedVersion,
	}
	stepType := "storage.retained-volume." + action
	step := Step{StepID: stepType, StepType: stepType, ProviderID: provider.ProviderID, ProviderVersion: provider.ProviderVersion,
		ProviderDigest: provider.ProviderDigest, ProviderProtocolVersion: "2.0.0", DependsOn: []string{}, TargetRef: intent.Spec.TargetID,
		TargetKind: "KubernetesTarget", InputSchema: "https://schemas.hnb.cloud/storage/v1/retained-volume-workflow-step-input.schema.json",
		Inputs: inputs, IdempotencyKey: idempotencyKey, FencingPolicy: "target-pv-pvc-dependency-snapshot-and-worker-lease",
		RetryPolicy: &RetryPolicy{MaxAttempts: 1, Backoff: "none"}, TimeoutSeconds: 3600,
		Compensation: &CompensationPolicy{Type: "none", OwnershipScope: "none"}}
	return []Step{step}, &TargetSnapshot{TargetID: intent.Spec.TargetID, TargetKind: "KubernetesTarget", ProjectionVersion: intent.Spec.ExpectedVersion, ObservationSource: "provider.storage-retained-volume-preflight"}
}

func storageDriverSteps(intent *RuntimeIntent, tenantID, action string, provider ProviderResolution) ([]Step, *TargetSnapshot) {
	idempotencyKey := lifecycleStepIdempotencyKey(tenantID, intent)
	inputs := map[string]any{
		"schemaVersion": "1.0.0", "action": action, "installationId": intent.Spec.InstallationID,
		"packageId": provider.PackageID, "packageVersion": provider.PackageVersion, "packageDigest": provider.PackageDigest,
		"provisioners": provider.Provisioners, "capabilityClaims": provider.CapabilityClaims,
		"targetId": intent.Spec.TargetID, "targetKind": "KubernetesTarget",
		"targetProjectionVersion": intent.Spec.ExpectedVersion, "kubernetesVersion": intent.Spec.KubernetesVersion,
		"idempotencyKey": idempotencyKey, "fencingGeneration": intent.Spec.ExpectedVersion,
	}
	if intent.Spec.CurrentVersion != "" {
		inputs["currentVersion"] = intent.Spec.CurrentVersion
	}
	if provider.RollbackVersion != "" {
		inputs["rollbackVersion"] = provider.RollbackVersion
	}
	if len(intent.Spec.Parameters) > 0 {
		inputs["parameters"] = intent.Spec.Parameters
	}
	compensation := &CompensationPolicy{Type: "none", OwnershipScope: "none"}
	if action == "install" {
		compensation = &CompensationPolicy{Type: "rollback", OwnershipScope: "operation-owned-only"}
	}
	if action == "upgrade" {
		compensation = &CompensationPolicy{Type: "rollback", OwnershipScope: "management-relation-only"}
	}
	stepType := "storage.driver." + action
	step := Step{StepID: stepType, StepType: stepType, ProviderID: provider.ProviderID, ProviderVersion: provider.ProviderVersion,
		ProviderDigest: provider.ProviderDigest, ProviderProtocolVersion: "2.0.0", DependsOn: []string{}, TargetRef: intent.Spec.TargetID,
		TargetKind: "KubernetesTarget", InputSchema: "https://schemas.hnb.cloud/storage/v1/storage-driver-lifecycle-step-input.schema.json",
		Inputs: inputs, SecretReferences: lifecycleSecretReferences(intent), IdempotencyKey: idempotencyKey,
		FencingPolicy: "monotonic-worker-lease-v2", RetryPolicy: &RetryPolicy{MaxAttempts: 3, Backoff: "exponential"}, TimeoutSeconds: 900, Compensation: compensation}
	return []Step{step}, &TargetSnapshot{TargetID: intent.Spec.TargetID, TargetKind: "KubernetesTarget", ProjectionVersion: intent.Spec.ExpectedVersion, ObservationSource: "Agent"}
}

func storageBindingSteps(intent *RuntimeIntent, tenantID, action string, provider ProviderResolution) ([]Step, *TargetSnapshot) {
	idempotencyKey := lifecycleStepIdempotencyKey(tenantID, intent)
	inputs := map[string]any{
		"schemaVersion": "1.0.0", "action": action, "bindingId": intent.Spec.BindingID,
		"bindingVersion": intent.Spec.BindingVersion, "offeringId": intent.Spec.OfferingID,
		"offeringVersion": intent.Spec.OfferingVersion, "targetId": intent.Spec.TargetID,
		"targetKind": "KubernetesTarget", "targetProjectionVersion": intent.Spec.ExpectedVersion,
		"storageClassName": intent.Spec.StorageClassName, "storageClassUid": intent.Spec.StorageClassUID,
		"storageClassResourceVersion": intent.Spec.StorageClassResourceVersion,
		"idempotencyKey":              idempotencyKey, "fencingGeneration": intent.Spec.ExpectedVersion,
	}
	stepType := "storage.binding." + action
	step := Step{
		StepID: stepType, StepType: stepType, ProviderID: provider.ProviderID,
		ProviderVersion: provider.ProviderVersion, ProviderDigest: provider.ProviderDigest,
		ProviderProtocolVersion: "2.0.0", DependsOn: []string{}, TargetRef: intent.Spec.TargetID,
		TargetKind: "KubernetesTarget", InputSchema: "https://schemas.hnb.cloud/storage/v1/storage-class-binding-step-input.schema.json",
		Inputs: inputs, SecretReferences: []SecretReferenceEntry{}, IdempotencyKey: idempotencyKey,
		FencingPolicy: "target-projection-and-storageclass-resource-version", RetryPolicy: &RetryPolicy{MaxAttempts: 3, Backoff: "exponential"},
		TimeoutSeconds: 300, Compensation: &CompensationPolicy{Type: "none", OwnershipScope: "none"},
	}
	return []Step{step}, &TargetSnapshot{TargetID: intent.Spec.TargetID, TargetKind: "KubernetesTarget", ProjectionVersion: intent.Spec.ExpectedVersion, ObservationSource: "runtime-target.storage-inventory"}
}

// generateSteps produces deterministic plan steps keyed by intent kind.
func (p *Planner) generateSteps(intent *RuntimeIntent) []Step {
	switch intent.Kind {
	case IntentInstallRelease:
		return p.stepsForInstall(intent)
	case IntentUninstallRelease:
		return p.stepsForUninstall(intent)
	case IntentUpgradeRelease:
		return p.stepsForUpgrade(intent)
	case IntentRollbackRelease:
		return p.stepsForRollback(intent)
	case IntentChangeConfiguration:
		return p.stepsForConfigChange(intent)
	case IntentCreateKubernetesTarget:
		return nil
	case IntentImportRuntimeTarget:
		return nil
	case IntentUpgradeRuntimeTarget:
		return nil
	case IntentDeleteRuntimeTarget:
		return nil
	default:
		return nil
	}
}

func lifecycleSteps(intent *RuntimeIntent, tenantID string, decision CompatibilityDecision, provider ProviderResolution) ([]Step, *TargetSnapshot) {
	targetID := intent.Spec.TargetID
	if targetID == "" {
		targetID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("hnb.runtime-target\x00"+tenantID+"\x00"+intent.ComputeIntentDigest())).String()
	}
	observationVersion := intent.Spec.ExpectedVersion
	snapshot := &TargetSnapshot{TargetID: targetID, TargetKind: decision.TargetKind, ProjectionVersion: observationVersion, ObservationSource: decision.ObservationSource}
	inputs := map[string]any{
		"schemaVersion": "1.0.0", "targetId": targetID, "targetKind": decision.TargetKind,
		"action": decision.Action, "idempotencyKey": lifecycleStepIdempotencyKey(tenantID, intent),
		"fencingGeneration": int64(1), "observationVersion": observationVersion,
	}
	if intent.Spec.DisplayName != "" {
		inputs["displayName"] = intent.Spec.DisplayName
	}
	if intent.Spec.CredentialSecretRef != nil {
		inputs["credentialSecretRef"] = *intent.Spec.CredentialSecretRef
	}
	if intent.Spec.CloudCoreEndpoint != "" {
		inputs["cloudCoreEndpoint"] = intent.Spec.CloudCoreEndpoint
	}
	if len(intent.Spec.NodeGroupMappings) > 0 {
		inputs["nodeGroupMappings"] = intent.Spec.NodeGroupMappings
	}
	desiredVersion := intent.Spec.DesiredVersion
	if desiredVersion == "" {
		desiredVersion = intent.Spec.KubernetesVersion
	}
	if desiredVersion != "" {
		inputs["desiredVersion"] = desiredVersion
	}

	kind := "kubernetes"
	inputSchema := "https://schemas.hnb.cloud/runtime-target/v1/kubernetes-lifecycle-step-input.schema.json"
	if decision.TargetKind == "EdgeRuntimeTarget" {
		kind = "edge"
		inputSchema = "https://schemas.hnb.cloud/runtime-target/v1/edge-lifecycle-step-input.schema.json"
	}
	operation := map[string]string{"create": "provision-and-register", "import": "register", "upgrade": "upgrade", "unmanage": "unregister"}[decision.Action]
	compensation := &CompensationPolicy{Type: "none", OwnershipScope: "none"}
	if decision.Action == "create" {
		compensation = &CompensationPolicy{Type: "unregister", OwnershipScope: "operation-owned-only"}
	} else if decision.Action == "import" {
		compensation = &CompensationPolicy{Type: "unregister", OwnershipScope: "management-relation-only"}
	}
	stepType := "runtime_target." + kind + "." + operation
	step := Step{
		StepID: stepType, StepType: stepType, ProviderID: provider.ProviderID,
		ProviderVersion: provider.ProviderVersion, ProviderDigest: provider.ProviderDigest,
		ProviderProtocolVersion: decision.ProviderProtocolVersion, DependsOn: []string{},
		TargetRef: targetID, TargetKind: decision.TargetKind, InputSchema: inputSchema, Inputs: inputs,
		SecretReferences: lifecycleSecretReferences(intent), IdempotencyKey: lifecycleStepIdempotencyKey(tenantID, intent),
		FencingPolicy: "monotonic-worker-lease-v2", RetryPolicy: &RetryPolicy{MaxAttempts: 3, Backoff: "exponential"},
		TimeoutSeconds: 900, Compensation: compensation,
	}
	if decision.Action == "create" {
		step.TimeoutSeconds = 1800
	}
	return []Step{step}, snapshot
}

func lifecycleStepIdempotencyKey(tenantID string, intent *RuntimeIntent) string {
	digest := sha256.Sum256([]byte(tenantID + "\x00" + string(intent.Kind) + "\x00" + intent.Metadata.IdempotencyKey))
	return fmt.Sprintf("step:%x", digest[:])
}

func lifecycleSecretReferences(intent *RuntimeIntent) []SecretReferenceEntry {
	refs := append([]SecretReferenceEntry{}, intent.Spec.SecretReferences...)
	if intent.Spec.CredentialSecretRef == nil {
		return refs
	}
	for _, ref := range refs {
		if ref == *intent.Spec.CredentialSecretRef {
			return refs
		}
	}
	return append(refs, *intent.Spec.CredentialSecretRef)
}

func (p *Planner) stepsForInstall(intent *RuntimeIntent) []Step {
	return []Step{
		{StepID: "install.validate", StepType: "validate", DependsOn: nil},
		{StepID: "install.deploy", StepType: "helm", DependsOn: []string{"install.validate"}},
		{StepID: "install.verify", StepType: "verify", DependsOn: []string{"install.deploy"}},
	}
}

func (p *Planner) stepsForUninstall(intent *RuntimeIntent) []Step {
	return []Step{
		{StepID: "uninstall.verify", StepType: "verify", DependsOn: nil},
		{StepID: "uninstall.resources", StepType: "helm", DependsOn: nil},
	}
}

func (p *Planner) stepsForUpgrade(intent *RuntimeIntent) []Step {
	return []Step{
		{StepID: "upgrade.snapshot", StepType: "snapshot", DependsOn: nil},
		{StepID: "upgrade.validate", StepType: "validate", DependsOn: []string{"upgrade.snapshot"}},
		{StepID: "upgrade.canary", StepType: "helm", DependsOn: []string{"upgrade.validate"}},
		{StepID: "upgrade.verify", StepType: "verify", DependsOn: []string{"upgrade.canary"}},
	}
}

func (p *Planner) stepsForRollback(intent *RuntimeIntent) []Step {
	return []Step{
		{StepID: "rollback.snapshot", StepType: "snapshot", DependsOn: nil},
		{StepID: "rollback.apply", StepType: "helm", DependsOn: []string{"rollback.snapshot"}},
		{StepID: "rollback.verify", StepType: "verify", DependsOn: []string{"rollback.apply"}},
	}
}

func (p *Planner) stepsForConfigChange(intent *RuntimeIntent) []Step {
	return []Step{
		{StepID: "config.validate", StepType: "validate", DependsOn: nil},
		{StepID: "config.update", StepType: "config", DependsOn: []string{"config.validate"}},
		{StepID: "config.reconfigure", StepType: "config", DependsOn: []string{"config.update"}},
		{StepID: "config.verify", StepType: "verify", DependsOn: []string{"config.reconfigure"}},
	}
}

func validateDAG(steps []Step) error {
	known := make(map[string]bool, len(steps))
	for _, s := range steps {
		known[s.StepID] = true
	}
	edges := make(map[string][]string, len(steps))
	for _, s := range steps {
		edges[s.StepID] = s.DependsOn
	}
	indegree := make(map[string]int, len(steps))
	for _, s := range steps {
		count := 0
		for _, dep := range s.DependsOn {
			if !known[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", s.StepID, dep)
			}
			if dep == s.StepID {
				return fmt.Errorf("step %q depends on itself", s.StepID)
			}
			count++
		}
		indegree[s.StepID] = count
	}
	queue := make([]string, 0, len(steps))
	for _, s := range steps {
		if indegree[s.StepID] == 0 {
			queue = append(queue, s.StepID)
		}
	}
	resolved := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		resolved++
		for _, s := range steps {
			for _, dep := range edges[s.StepID] {
				if dep == cur {
					indegree[s.StepID]--
					if indegree[s.StepID] == 0 {
						queue = append(queue, s.StepID)
					}
				}
			}
		}
	}
	if resolved != len(steps) {
		return fmt.Errorf("step dependencies contain a cycle")
	}
	return nil
}

func computePlanDigest(intent *RuntimeIntent, tenantID string, steps []Step, compatibility *CompatibilityDecision, targetSnapshot *TargetSnapshot, providerVersions map[string]string, artifactDigests []string) string {
	doc := planDoc{
		TenantID: tenantID, IntentDigest: intent.ComputeIntentDigest(), Steps: steps,
		Compatibility: compatibility, TargetSnapshot: targetSnapshot, ProviderVersions: providerVersions, ArtifactDigests: artifactDigests,
	}
	data, _ := json.Marshal(doc)
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h[:])
}

func computeCapabilitySnapshotDigest(targetRef string, compatibility *CompatibilityDecision, providerVersions map[string]string, artifactDigests []string) string {
	data, _ := json.Marshal(struct {
		TargetRef        string                 `json:"targetRef"`
		Compatibility    *CompatibilityDecision `json:"compatibility,omitempty"`
		ProviderVersions map[string]string      `json:"providerVersions,omitempty"`
		ArtifactDigests  []string               `json:"artifactDigests,omitempty"`
	}{targetRef, compatibility, providerVersions, artifactDigests})
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash[:])
}

type planDoc struct {
	TenantID         string                 `json:"tenant_id"`
	IntentDigest     string                 `json:"intent_digest"`
	Steps            []Step                 `json:"steps"`
	Compatibility    *CompatibilityDecision `json:"compatibility,omitempty"`
	TargetSnapshot   *TargetSnapshot        `json:"targetSnapshot,omitempty"`
	ProviderVersions map[string]string      `json:"providerVersions,omitempty"`
	ArtifactDigests  []string               `json:"artifactDigests,omitempty"`
}

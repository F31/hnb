package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	stalepolicy "github.com/F31/hnb/cmd/platform-api/internal/stale"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

type testAuthenticator struct{}
type noPermissionAuthenticator struct{}

type testPermissionResolver struct {
	policy      string
	permissions []iam.ScopedPermission
	err         error
}

type secretAwareStore struct {
	*fakeStore
	metadata *store.SecretReferenceMetadata
}

func (s *secretAwareStore) ResolveSecretReference(context.Context, string, string, string, string, string) (*store.SecretReferenceMetadata, error) {
	if s.metadata == nil {
		return nil, store.ErrSecretReferenceDenied
	}
	return s.metadata, nil
}

func (r testPermissionResolver) ResolvePermissions(context.Context, string, string, string) (string, []iam.ScopedPermission, error) {
	return r.policy, r.permissions, r.err
}

type tokenTestKeys struct{ key *ecdsa.PrivateKey }

var testDelegationSigners sync.Map

func (k tokenTestKeys) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return "key-1", k.key, nil
}

func (k tokenTestKeys) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid != "key-1" {
		return nil, fmt.Errorf("unknown key")
	}
	return &k.key.PublicKey, nil
}

type tokenTestIdentity struct{}

func (tokenTestIdentity) ResolveUserIdentity(context.Context, string, string) (*iam.Identity, error) {
	return &iam.Identity{UserID: "user", SubjectID: "subject", SubjectType: "user", TenantID: "tenant-a", MembershipID: "membership-a"}, nil
}

func (tokenTestIdentity) ResolveMembership(context.Context, string, string) (*iam.Identity, error) {
	return &iam.Identity{UserID: "user", SubjectID: "subject", SubjectType: "user", TenantID: "tenant-a", MembershipID: "membership-a"}, nil
}

func (tokenTestIdentity) ResolvePermissions(context.Context, string, string, string) (string, []iam.ScopedPermission, error) {
	return "default:1", []iam.ScopedPermission{{ResourceKind: "*", Action: iam.ActionList, TenantID: "tenant-a"}}, nil
}

type tokenTestRefreshStore struct{}

func (tokenTestRefreshStore) CreateRefreshToken(context.Context, iam.RefreshTokenRecord) error {
	return nil
}
func (tokenTestRefreshStore) RotateRefreshToken(context.Context, string, iam.RefreshTokenRecord, time.Time) (*iam.RefreshTokenRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testAuthenticator) Authenticate(_ context.Context, token, correlationID, traceparent string) (iam.TrustedContext, error) {
	if token != "valid-token" {
		return iam.TrustedContext{}, fmt.Errorf("invalid token")
	}
	return iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", TenantID: "tenant-a", MembershipID: "membership-a",
		PolicyVersion: "default:1", ScopedPermissions: platformTestPermissions("tenant-a"),
		CorrelationID: correlationID, Traceparent: traceparent,
	}, nil
}

func (noPermissionAuthenticator) Authenticate(_ context.Context, token, correlationID, traceparent string) (iam.TrustedContext, error) {
	if token != "valid-token" {
		return iam.TrustedContext{}, fmt.Errorf("invalid token")
	}
	return iam.TrustedContext{SubjectID: "subject", TenantID: "tenant-a", PolicyVersion: "default:1", CorrelationID: correlationID, Traceparent: traceparent}, nil
}

func platformTestPermissions(tenantID string) []iam.ScopedPermission {
	actions := []iam.AuthorizationAction{iam.ActionRead, iam.ActionList, iam.ActionCreate, iam.ActionUpdate, iam.ActionDelete, iam.ActionExecute, iam.ActionApprove, iam.ActionReject, iam.ActionCancel}
	permissions := make([]iam.ScopedPermission, 0, len(actions))
	for _, action := range actions {
		permissions = append(permissions, iam.ScopedPermission{ResourceKind: "*", Action: action, TenantID: tenantID})
	}
	return permissions
}

// fakeStore is an in-memory store.Store used to exercise handlers through
// httptest without a database.
type fakeStore struct {
	mu           sync.Mutex
	ops          map[string]*store.Operation
	byIdemKey    map[string]string
	targets      map[string]*store.RuntimeTarget
	clusters     map[string]*store.Cluster
	pingErr      error
	calls        int
	audits       []store.SecurityAuditRecord
	commitments  map[string]*store.IntentCommitment
	lastIntent   *store.IntentSubmitCommand
	manifests    map[string]*store.ProviderManifest
	secrets      map[string]*store.SecretReferenceMetadata
	secretValues map[string]string
}

func newFakeStore() *fakeStore {
	f := &fakeStore{
		ops:          map[string]*store.Operation{},
		byIdemKey:    map[string]string{},
		targets:      map[string]*store.RuntimeTarget{},
		clusters:     map[string]*store.Cluster{},
		commitments:  map[string]*store.IntentCommitment{},
		manifests:    map[string]*store.ProviderManifest{},
		secretValues: map[string]string{},
	}
	f.seedLifecycleManifest("runtime-target.lifecycle.kubernetes", "KubernetesTarget", []string{"create", "import", "upgrade", "unmanage"})
	f.seedLifecycleManifest("runtime-target.lifecycle.edge", "EdgeRuntimeTarget", []string{"import", "upgrade", "unmanage"})
	return f
}

func (f *fakeStore) seedLifecycleManifest(providerID, targetKind string, actions []string) {
	expires := time.Date(2027, 8, 1, 0, 0, 0, 0, time.UTC)
	evidence := make([]store.ConformanceEvidence, 0, len(actions))
	for _, action := range actions {
		evidence = append(evidence, store.ConformanceEvidence{
			TestName: "1.0.0/" + targetKind + "/" + action, Category: "runtime-target-lifecycle",
			Passed: true, EvidenceRef: "evidence://" + providerID + "/" + action,
		})
	}
	f.manifests[providerID] = &store.ProviderManifest{
		ProviderID: providerID, Name: providerID, Version: "1.0.0", ProtocolVersion: "2.0.0",
		Capabilities: []string{"runtime-target-lifecycle"}, Actions: actions,
		ConformanceLevel: "production_ready", ConformanceEvidence: evidence, ConformanceExpiresAt: &expires,
	}
}

func (f *fakeStore) recordCall() {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
}

func (f *fakeStore) Ping(ctx context.Context) error {
	f.recordCall()
	return f.pingErr
}

func (f *fakeStore) Ready(ctx context.Context) error {
	f.recordCall()
	return f.pingErr
}

func (f *fakeStore) SubmitOperation(ctx context.Context, cmd store.SubmitCommand) (*store.Operation, bool, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.byIdemKey[cmd.TenantID+"/"+cmd.IdempotencyKey]; ok {
		return cloneOp(f.ops[id]), false, nil
	}
	op := &store.Operation{
		ID:                 uuid.NewString(),
		TenantID:           cmd.TenantID,
		NamespaceID:        cmd.NamespaceID,
		OperationType:      cmd.OperationType,
		Status:             store.InitialStatus(cmd.OperationType),
		InitiatedBy:        cmd.InitiatedBy,
		IdempotencyKey:     cmd.IdempotencyKey,
		TotalSteps:         len(cmd.Steps),
		CreatedAt:          time.Now().UTC(),
		LastStateChangedAt: time.Now().UTC(),
	}
	for i, step := range cmd.Steps {
		id := step.PlanStepID
		if id == "" {
			id = fmt.Sprintf("step-%d", i+1)
		}
		op.Steps = append(op.Steps, store.Step{
			ID: uuid.NewString(), PlanStepID: id, Name: step.Name,
			StepType: step.StepType, Status: "pending", DependsOn: step.DependsOn,
		})
	}
	f.ops[op.ID] = op
	f.byIdemKey[cmd.TenantID+"/"+cmd.IdempotencyKey] = op.ID
	return cloneOp(op), true, nil
}

func (f *fakeStore) transition(id, tenantID, actorID, to string, requireApprovalState bool) (*store.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok || op.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	if requireApprovalState && op.Status != store.StatusPendingApproval {
		return nil, fmt.Errorf("%w: state %s", store.ErrInvalidState, op.Status)
	}
	if !store.CanTransition(op.Status, to) {
		return nil, fmt.Errorf("%w: state %s", store.ErrInvalidState, op.Status)
	}
	op.Status = to
	op.LastStateChangedAt = time.Now().UTC()
	if to == store.StatusQueued {
		op.ApprovedBy = actorID
	}
	return cloneOp(op), nil
}

func (f *fakeStore) ApproveOperation(ctx context.Context, id, tenantID, actorID, reason string) (*store.Operation, error) {
	f.recordCall()
	return f.transition(id, tenantID, actorID, store.StatusQueued, true)
}

func (f *fakeStore) RejectOperation(ctx context.Context, id, tenantID, actorID, reason string) (*store.Operation, error) {
	f.recordCall()
	return f.transition(id, tenantID, actorID, store.StatusCancelled, true)
}

func (f *fakeStore) CancelOperation(ctx context.Context, id, tenantID, actorID, reason string) (*store.Operation, error) {
	f.recordCall()
	return f.transition(id, tenantID, actorID, store.StatusCancelled, false)
}

func (f *fakeStore) GetOperation(ctx context.Context, id, tenantID string) (*store.Operation, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok || op.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	return cloneOp(op), nil
}

func (f *fakeStore) ListOperations(ctx context.Context, q store.ListQuery) ([]store.OperationSummary, int, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []store.OperationSummary
	for _, op := range f.ops {
		if op.TenantID != q.TenantID {
			continue
		}
		if q.Status != "" && op.Status != q.Status {
			continue
		}
		if q.OperationType != "" && op.OperationType != q.OperationType {
			continue
		}
		items = append(items, store.OperationSummary{
			ID: op.ID, TenantID: op.TenantID, OperationType: op.OperationType,
			Status: op.Status, TotalSteps: op.TotalSteps, InitiatedBy: op.InitiatedBy,
			CreatedAt: op.CreatedAt, LastStateChangedAt: op.LastStateChangedAt,
		})
	}
	total := len(items)
	if q.Offset < len(items) {
		items = items[q.Offset:]
	} else {
		items = nil
	}
	if q.Limit < len(items) {
		items = items[:q.Limit]
	}
	return items, total, nil
}

func cloneOp(op *store.Operation) *store.Operation {
	copied := *op
	copied.Steps = append([]store.Step(nil), op.Steps...)
	return &copied
}

func (f *fakeStore) CreateRuntimeTarget(ctx context.Context, rt *store.RuntimeTarget) error {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	rt.ID = uuid.NewString()
	rt.CreatedAt = time.Now().UTC()
	if rt.Distribution == "" {
		rt.Distribution = "standard"
	}
	if rt.Status == "" {
		rt.Status = "unknown"
	}
	if rt.StaleThresholdSec == 0 {
		rt.StaleThresholdSec = 300
	}
	rt.IsActive = true
	cloned := *rt
	f.targets[rt.ID] = &cloned
	return nil
}

func (f *fakeStore) GetRuntimeTarget(ctx context.Context, id, tenantID string) (*store.RuntimeTarget, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	rt, ok := f.targets[id]
	if !ok || rt.TenantID != tenantID {
		return nil, store.ErrTargetNotFound
	}
	cloned := *rt
	return &cloned, nil
}

func (f *fakeStore) ListRuntimeTargets(ctx context.Context, tenantID string) ([]*store.RuntimeTarget, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*store.RuntimeTarget
	for _, rt := range f.targets {
		if rt.TenantID == tenantID {
			cloned := *rt
			result = append(result, &cloned)
		}
	}
	return result, nil
}

func (f *fakeStore) UpdateRuntimeTargetStatus(ctx context.Context, id, tenantID string, status string, observedAt time.Time) error {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	rt, ok := f.targets[id]
	if !ok || rt.TenantID != tenantID {
		return store.ErrTargetNotFound
	}
	rt.Status = status
	rt.ObservedAt = &observedAt
	return nil
}

func (f *fakeStore) UpdateRuntimeTargetDescription(ctx context.Context, id, tenantID, description string) error {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	rt, ok := f.targets[id]
	if !ok || rt.TenantID != tenantID {
		return store.ErrTargetNotFound
	}
	rt.Description = description
	return nil
}

func (f *fakeStore) DeleteRuntimeTarget(ctx context.Context, id, tenantID string) error {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	if rt, ok := f.targets[id]; !ok || rt.TenantID != tenantID {
		return store.ErrTargetNotFound
	}
	delete(f.targets, id)
	return nil
}

// ClusterStore interface implementations for fakeStore
func (f *fakeStore) CreateCluster(ctx context.Context, req store.CreateClusterRequest) (*store.Cluster, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	c := &store.Cluster{
		ID:            uuid.NewString(),
		Name:          req.Name,
		TenantID:      req.TenantID,
		ClusterType:   req.ClusterType,
		APIEndpoint:   req.APIEndpoint,
		KubeconfigRef: req.KubeconfigRef,
		Region:        req.Region,
		Zone:          req.Zone,
		Labels:        req.Labels,
		Status:        "pending",
		CreatedAt:     now,
		UpdatedAt:     now,
		Version:       1,
		Scope:         "tenant", // default scope
	}
	f.clusters[c.ID] = c
	return c, nil
}

func (f *fakeStore) GetCluster(ctx context.Context, id, tenantID string) (*store.Cluster, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.clusters[id]
	if !ok || c.TenantID != tenantID {
		return nil, store.ErrClusterNotFound
	}
	cloned := *c
	return &cloned, nil
}

func (f *fakeStore) ListClusters(ctx context.Context, tenantID string) ([]*store.Cluster, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*store.Cluster
	for _, c := range f.clusters {
		if c.TenantID != tenantID || c.Scope != "tenant" {
			continue
		}
		cloned := *c
		result = append(result, &cloned)
	}
	return result, nil
}

func (f *fakeStore) DeleteCluster(ctx context.Context, id, tenantID string) error {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.clusters[id]
	if !ok || c.TenantID != tenantID {
		return store.ErrClusterNotFound
	}
	delete(f.clusters, id)
	return nil
}

func (f *fakeStore) HeartbeatCluster(ctx context.Context, id, tenantID string) error {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.clusters[id]
	if !ok || c.TenantID != tenantID {
		return store.ErrClusterNotFound
	}
	now := time.Now().UTC()
	c.Status = "heartbeat"
	c.LastHeartbeat = &now
	c.UpdatedAt = now
	return nil
}

func (f *fakeStore) UpdateCluster(ctx context.Context, id, tenantID string, req store.UpdateClusterRequest) (*store.Cluster, error) {
	f.recordCall()
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.clusters[id]
	if !ok || c.TenantID != tenantID {
		return nil, store.ErrClusterNotFound
	}
	if req.Region != nil {
		c.Region = *req.Region
	}
	if req.Zone != nil {
		c.Zone = *req.Zone
	}
	if req.Labels != nil {
		c.Labels = *req.Labels
	}
	if req.Status != nil {
		c.Status = *req.Status
	}
	c.UpdatedAt = time.Now().UTC()
	cloned := *c
	return &cloned, nil
}

func (f *fakeStore) GetManifest(ctx context.Context, providerID string) (*store.ProviderManifest, error) {
	manifest, ok := f.manifests[providerID]
	if !ok {
		return nil, store.ErrNotFound
	}
	copy := *manifest
	return &copy, nil
}
func (f *fakeStore) SaveManifest(ctx context.Context, manifest *store.ProviderManifest) error {
	copy := *manifest
	f.manifests[manifest.ProviderID] = &copy
	return nil
}
func (f *fakeStore) DeleteManifest(ctx context.Context, providerID string) error {
	delete(f.manifests, providerID)
	return nil
}
func (f *fakeStore) CheckCompatibility(ctx context.Context, coreVersion, providerID, providerVersion, targetType string) (*store.CompatibilityEntry, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) SaveCompatibility(ctx context.Context, entry *store.CompatibilityEntry) error {
	return nil
}
func (f *fakeStore) ExpireConformance(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (f *fakeStore) CheckProviderConformance(ctx context.Context, providerID string) error {
	return nil
}
func (f *fakeStore) UpdateConformanceLevel(ctx context.Context, providerID, level string, expiresAt *time.Time) error {
	return nil
}

func (f *fakeStore) ResolveSecretReference(_ context.Context, _, provider, scope, name, _ string) (*store.SecretReferenceMetadata, error) {
	if f.secrets != nil {
		if ref, ok := f.secrets[scope+"/"+name]; ok {
			return ref, nil
		}
	}
	if provider == "vault" && scope == "tenant" && name == "cluster-credential" {
		return &store.SecretReferenceMetadata{
			Purpose:                    "kubeconfig",
			AllowedLifecycleProviderID: "runtime-target.lifecycle.kubernetes",
		}, nil
	}
	if provider == "vault" && scope == "tenant" && name == "edge-credential" {
		return &store.SecretReferenceMetadata{
			Purpose:                    "cloudcore-client",
			AllowedLifecycleProviderID: "runtime-target.lifecycle.edge",
		}, nil
	}
	return nil, store.ErrSecretReferenceDenied
}

func (f *fakeStore) RegisterSecretReference(_ context.Context, req store.RegisterSecretReferenceRequest) (*store.SecretReferenceMetadata, error) {
	if req.Name == "" || req.Scope == "" || req.Purpose == "" || req.EncryptedValue == "" {
		return nil, fmt.Errorf("register secret: name, scope, purpose, encrypted value and tenant are required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.secrets == nil {
		f.secrets = map[string]*store.SecretReferenceMetadata{}
	}
	ref := &store.SecretReferenceMetadata{
		TenantID: req.TenantID, Provider: "local-aes", Scope: req.Scope,
		Name: req.Name, Version: "1", Purpose: req.Purpose,
		AllowedLifecycleProviderID: req.AllowedLifecycleProviderID,
	}
	f.secrets[req.Scope+"/"+req.Name] = ref
	f.secretValues[req.TenantID+"/"+req.Purpose] = req.EncryptedValue
	f.secretValues[req.TenantID+"/"+req.Scope+"/"+req.Name] = req.EncryptedValue
	return ref, nil
}

func (f *fakeStore) LatestKubeConfigEncrypted(_ context.Context, tenantID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.secretValues[tenantID+"/kubeconfig"]; ok {
		return v, nil
	}
	return "", store.ErrSecretReferenceDenied
}

func (f *fakeStore) KubeConfigEncryptedForRef(_ context.Context, tenantID, scope, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ref, ok := f.secrets[scope+"/"+name]; ok && ref.TenantID == tenantID && ref.Purpose == "kubeconfig" {
		if v, ok := f.secretValues[tenantID+"/"+scope+"/"+name]; ok {
			return v, nil
		}
	}
	return "", store.ErrSecretReferenceDenied
}

func (f *fakeStore) AppendSecurityAudit(_ context.Context, record store.SecurityAuditRecord) error {
	f.audits = append(f.audits, record)
	return nil
}

func (f *fakeStore) SubmitIntent(ctx context.Context, cmd store.IntentSubmitCommand) (*store.Operation, bool, error) {
	f.recordCall()
	command := cmd
	f.lastIntent = &command
	commitmentKey := cmd.TenantID + "/" + string(cmd.Intent.Kind) + "/" + cmd.CommitmentAction + "/" + cmd.Intent.Metadata.IdempotencyKey
	if commitment, ok := f.commitments[commitmentKey]; ok {
		if commitment.SemanticDigest != cmd.Intent.ComputeIntentDigest() {
			return nil, false, store.ErrIdempotencyConflict
		}
		op := cloneOp(f.ops[commitment.OperationID])
		op.IntentID, op.Status, op.CorrelationID, op.CreatedAt = commitment.IntentID, commitment.AcceptedStatus, commitment.CorrelationID, commitment.CreatedAt
		return op, false, nil
	}
	opType := intentKindToOperationType(cmd.Intent.Kind)
	initialStatus := store.InitialStatus(opType)
	if cmd.InitialStatus != "" {
		initialStatus = cmd.InitialStatus
	}
	op := &store.Operation{
		IntentID:           uuid.NewString(),
		ID:                 uuid.New().String(),
		PlanID:             uuid.NewString(),
		TenantID:           cmd.TenantID,
		NamespaceID:        "",
		OperationType:      opType,
		Status:             initialStatus,
		InitiatedBy:        cmd.InitiatedBy,
		IdempotencyKey:     cmd.Intent.Metadata.IdempotencyKey,
		CorrelationID:      cmd.CorrelationID,
		TotalSteps:         len(cmd.ExecutionPlan.Steps),
		CreatedAt:          time.Now().UTC(),
		LastStateChangedAt: time.Now().UTC(),
	}
	for i, step := range cmd.ExecutionPlan.Steps {
		op.Steps = append(op.Steps, store.Step{
			ID: uuid.New().String(), PlanStepID: step.StepID, Name: step.StepID + "-task",
			StepType: step.StepType, Status: "pending", DependsOn: step.DependsOn,
		})
		_ = i
	}
	f.ops[op.ID] = op
	f.byIdemKey[cmd.TenantID+"/"+cmd.Intent.Metadata.IdempotencyKey] = op.ID
	f.commitments[commitmentKey] = &store.IntentCommitment{
		IntentID: op.IntentID, ExecutionPlanID: op.PlanID, OperationID: op.ID,
		SemanticDigest: cmd.Intent.ComputeIntentDigest(), Kind: string(cmd.Intent.Kind), Action: cmd.CommitmentAction,
		AcceptedStatus: op.Status, CorrelationID: op.CorrelationID, CreatedAt: op.CreatedAt, HTTPStatus: http.StatusAccepted,
	}
	if cmd.ConfirmationAccepted {
		f.audits = append(f.audits, store.SecurityAuditRecord{
			EventType: "intent_received", Decision: "allow", Outcome: cmd.StalePolicyOutcome,
			Detail: map[string]any{"confirmationAccepted": true, "policyOutcome": cmd.StalePolicyOutcome},
		})
	}
	return cloneOp(op), true, nil
}

func (f *fakeStore) GetIntentCommitment(_ context.Context, tenantID, kind, action, idempotencyKey string) (*store.IntentCommitment, error) {
	commitment, ok := f.commitments[tenantID+"/"+kind+"/"+action+"/"+idempotencyKey]
	if !ok {
		return nil, store.ErrNotFound
	}
	copy := *commitment
	return &copy, nil
}

func intentKindToOperationType(kind engine.IntentKind) string {
	switch kind {
	case engine.IntentInstallRelease:
		return "deploy"
	case engine.IntentUninstallRelease:
		return "delete"
	case engine.IntentUpgradeRelease:
		return "upgrade"
	case engine.IntentRollbackRelease:
		return "rollback"
	case engine.IntentChangeConfiguration:
		return "config_change"
	default:
		return "deploy"
	}
}

func newTestServer() (*Server, *fakeStore) {
	st := newFakeStore()
	return NewServer(st, testAuthenticator{}, testPermissionResolver{policy: "default:2", permissions: platformTestPermissions("tenant-a")}), st
}

func clusterExistingTargetIntent(kind, targetID, targetKind string, expectedVersion int64) string {
	desiredVersion := ""
	if kind == string(engine.IntentUpgradeRuntimeTarget) {
		desiredVersion = `,"desiredVersion":"v1.31.0"`
	}
	return fmt.Sprintf(`{"apiVersion":"hnb.io/v1","kind":%q,"metadata":{"idempotencyKey":"cluster-action","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetId":%q,"targetKind":%q,"expectedVersion":%d%s}}`,
		kind, targetID, targetKind, expectedVersion, desiredVersion)
}

func doRequest(t *testing.T, srv *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, reader)
	if target != "/healthz" {
		if method == http.MethodPost && target == "/v1/intents" {
			setTestIntentDelegation(t, srv, req, body)
		} else {
			req.Header.Set("Authorization", "Bearer valid-token")
		}
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func setTestIntentDelegation(t *testing.T, srv *Server, req *http.Request, body string) {
	t.Helper()
	value, ok := testDelegationSigners.Load(srv)
	if !ok {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		keys := tokenTestKeys{key: key}
		config := iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}
		signer, err := iam.NewDelegationSigner(config, keys)
		if err != nil {
			t.Fatal(err)
		}
		verifier, err := iam.NewDelegationVerifier(config, keys)
		if err != nil {
			t.Fatal(err)
		}
		srv.ConfigureIntentDelegation(verifier)
		if srv.permissionResolver == nil {
			if _, denied := srv.auth.(noPermissionAuthenticator); denied {
				srv.permissionResolver = testPermissionResolver{policy: "default:2"}
			} else {
				srv.permissionResolver = testPermissionResolver{policy: "default:2", permissions: platformTestPermissions("tenant-a")}
			}
		}
		testDelegationSigners.Store(srv, signer)
		value = signer
	}
	signer := value.(*iam.DelegationSigner)
	correlationID := "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"
	kind := string(engine.IntentImportRuntimeTarget)
	targetID := ""
	action := iam.ActionCreate
	digest := "sha256:" + strings.Repeat("0", 64)
	if intent, err := engine.ParseRuntimeIntent([]byte(body)); err == nil {
		kind = string(intent.Kind)
		targetID = intent.Spec.TargetID
		digest = intent.ComputeIntentDigest()
		if intent.Metadata.CorrelationID != "" {
			correlationID = intent.Metadata.CorrelationID
		}
		if resolved, ok := iam.ClusterActionForIntentKind(kind); ok {
			action = resolved
		}
	}
	token, err := signer.Sign(req.Context(), iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:2",
	}, iam.DelegationEvidence{
		Scope:  iam.DelegationScope{ResourceKind: string(iam.ResourceCluster), ResourceID: targetID},
		Action: action, IntentKind: kind, SemanticDigest: digest, CorrelationID: correlationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", correlationID)
	req.Header.Set("X-Semantic-Digest", digest)
}

// setTestSecretDelegation signs a secret-registration delegation (no intent
// kind or semantic digest) mirroring the BFF forwarding for /v1/secrets:register.
func setTestSecretDelegation(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()
	value, ok := testDelegationSigners.Load(srv)
	if !ok {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		keys := tokenTestKeys{key: key}
		config := iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}
		signer, err := iam.NewDelegationSigner(config, keys)
		if err != nil {
			t.Fatal(err)
		}
		verifier, err := iam.NewDelegationVerifier(config, keys)
		if err != nil {
			t.Fatal(err)
		}
		srv.ConfigureIntentDelegation(verifier)
		if srv.permissionResolver == nil {
			if _, denied := srv.auth.(noPermissionAuthenticator); denied {
				srv.permissionResolver = testPermissionResolver{policy: "default:2"}
			} else {
				srv.permissionResolver = testPermissionResolver{policy: "default:2", permissions: platformTestPermissions("tenant-a")}
			}
		}
		testDelegationSigners.Store(srv, signer)
		value = signer
	}
	signer := value.(*iam.DelegationSigner)
	correlationID := "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"
	token, err := signer.Sign(req.Context(), iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:2",
	}, iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceSecret)},
		Action:        iam.ActionCreate,
		CorrelationID: correlationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", correlationID)
}

// setTestDescriptionDelegation signs a cluster-description update delegation
// mirroring the BFF forwarding for PATCH /v1/clusters/{id}/description.
func setTestDescriptionDelegation(t *testing.T, srv *Server, req *http.Request, resourceID string) {
	t.Helper()
	value, ok := testDelegationSigners.Load(srv)
	if !ok {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		keys := tokenTestKeys{key: key}
		config := iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}
		signer, err := iam.NewDelegationSigner(config, keys)
		if err != nil {
			t.Fatal(err)
		}
		verifier, err := iam.NewDelegationVerifier(config, keys)
		if err != nil {
			t.Fatal(err)
		}
		srv.ConfigureIntentDelegation(verifier)
		if srv.permissionResolver == nil {
			if _, denied := srv.auth.(noPermissionAuthenticator); denied {
				srv.permissionResolver = testPermissionResolver{policy: "default:2"}
			} else {
				srv.permissionResolver = testPermissionResolver{policy: "default:2", permissions: platformTestPermissions("tenant-a")}
			}
		}
		testDelegationSigners.Store(srv, signer)
		value = signer
	}
	signer := value.(*iam.DelegationSigner)
	correlationID := "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"
	token, err := signer.Sign(req.Context(), iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:2",
	}, iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceClusterMetadata), ResourceID: resourceID},
		Action:        iam.ActionUpdate,
		CorrelationID: correlationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", correlationID)
}

// setTestKubeconfigDelegation signs a read-scoped cluster-metadata delegation
// mirroring the BFF forwarding for POST /v1/clusters/{id}/kubeconfig:issue.
func setTestKubeconfigDelegation(t *testing.T, srv *Server, req *http.Request, resourceID string) {
	t.Helper()
	signer := testDelegationSignerFor(t, srv)
	token, err := signer.Sign(req.Context(), delegatedTrustedContext(), iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceClusterMetadata), ResourceID: resourceID},
		Action:        iam.ActionRead,
		CorrelationID: testCorrelationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", testCorrelationID)
}

func doUnauthenticatedRequest(srv *Server, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestProviderManifestRegistryRejectsInvalidStorageDriverPackage(t *testing.T) {
	srv, st := newTestServer()
	body := `{
		"name":"storage", "version":"1.0.0", "protocolVersion":"2.0.0",
		"capabilities":["storage"], "actions":["validate"],
		"storageDriverPackage":{"schemaVersion":"0.0.0"}
	}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/providers/storage.example/manifest", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, exists := st.manifests["storage.example"]; exists {
		t.Fatal("invalid storage driver package was registered")
	}
}

func submitBody(opType, idemKey string) string {
	return fmt.Sprintf(`{
		"tenantId": "tenant-a",
		"namespaceId": "ns-prod",
		"releaseId": "rel-1",
		"operationType": %q,
		"idempotencyKey": %q,
		"initiatedBy": "user-1",
		"steps": [{"id": "rollout", "name": "rollout", "stepType": "http"}]
	}`, opType, idemKey)
}

func TestHealthz(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestSubmitOperationCreated(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp operationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != store.StatusQueued {
		t.Fatalf("status = %q, want queued", resp.Status)
	}
	if resp.TenantID != "tenant-a" || resp.InitiatedBy != "signed-subject" {
		t.Fatalf("trusted identity was not authoritative: %+v", resp)
	}
	if resp.LastObservedAt.IsZero() || resp.LastStateChangedAt.IsZero() {
		t.Fatal("response must carry lastObservedAt and last_state_changed_at")
	}
	if len(resp.Steps) != 1 || resp.Steps[0].PlanStepID != "rollout" {
		t.Fatalf("steps = %+v", resp.Steps)
	}
}

func TestSubmitOperationHighRiskPendingApproval(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("delete", "k-2"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != store.StatusPendingApproval {
		t.Fatalf("status = %q, want pending_approval", resp.Status)
	}
}

func TestSubmitOperationIdempotentReplay(t *testing.T) {
	srv, _ := newTestServer()
	first := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-3"))
	second := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-3"))
	if first.Code != http.StatusCreated || second.Code != http.StatusOK {
		t.Fatalf("codes = %d, %d", first.Code, second.Code)
	}
	var a, b operationResponse
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a.ID != b.ID {
		t.Fatalf("idempotent replay returned different operation %s vs %s", a.ID, b.ID)
	}
}

func TestSubmitOperationValidationError(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", `{"tenantId":"t"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type = %q", got)
	}
	var resp problemDetails
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Code != "VALIDATION_FAILED" {
		t.Fatalf("error code = %q", resp.Code)
	}
}

func TestRuntimeIntentValidationUsesProblemViolationWithoutRejectedValue(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"UpgradeRuntimeTarget","metadata":{"idempotencyKey":"invalid-target"},"spec":{"targetId":"secret-target-value","targetKind":"KubernetesTarget","expectedVersion":1,"desiredVersion":"v1.31"}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusBadRequest || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status=%d content-type=%q body=%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	var problem problemDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "VALIDATION_FAILED" || len(problem.Violations) != 1 || problem.Violations[0].Field != "spec.targetId" {
		t.Fatalf("problem=%+v", problem)
	}
	if strings.Contains(rec.Body.String(), "secret-target-value") {
		t.Fatal("rejected value leaked into problem")
	}
}

func TestSubmitOperationRejectsUnknownFields(t *testing.T) {
	srv, _ := newTestServer()
	body := strings.Replace(submitBody("deploy", "k-4"), `"steps"`, `"unexpected": 1, "steps"`, 1)
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestApproveFlow(t *testing.T) {
	srv, st := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("rollback", "k-5"))
	var created operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	action := fmt.Sprintf(`{"tenantId":"tenant-a","actorId":"approver-1","reason":"ok"}`)
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/approve", action)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body %s", rec.Code, rec.Body)
	}
	var approved operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &approved)
	if approved.Status != store.StatusQueued || approved.ApprovedBy != "signed-subject" {
		t.Fatalf("approved = %+v", approved)
	}

	// Second approval must conflict: no longer pending_approval.
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/approve", action)
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-approve status = %d, body %s", rec.Code, rec.Body)
	}

	if got := st.ops[created.ID].Status; got != store.StatusQueued {
		t.Fatalf("stored status = %q", got)
	}
}

func TestApproveRequiresPendingApproval(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-6"))
	var created operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	action := `{"tenantId":"tenant-a","actorId":"approver-1"}`
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/approve", action)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestRejectFlow(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("config_change", "k-7"))
	var created operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	action := `{"tenantId":"tenant-a","actorId":"approver-1","reason":"too risky"}`
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/reject", action)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var rejected operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &rejected)
	if rejected.Status != store.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", rejected.Status)
	}
}

func TestCancelFlow(t *testing.T) {
	srv, st := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-8"))
	var created operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	action := `{"tenantId":"tenant-a","actorId":"user-1","reason":"no longer needed"}`
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/cancel", action)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	// Terminal state cannot be cancelled again.
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/cancel", action)
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-cancel status = %d, body %s", rec.Code, rec.Body)
	}
	if got := st.ops[created.ID].Status; got != store.StatusCancelled {
		t.Fatalf("stored status = %q", got)
	}
}

func TestBodyAndQueryIdentitySpoofingIsOverridden(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-9"))
	var created operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// Caller-controlled tenant and actor values are ignored in favor of signed context.
	rec = doRequest(t, srv, http.MethodGet, "/v1/operations/"+created.ID+"?tenant_id=tenant-b", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body %s", rec.Code, rec.Body)
	}
	action := `{"tenantId":"tenant-b","actorId":"mallory"}`
	rec = doRequest(t, srv, http.MethodPost, "/v1/operations/"+created.ID+"/cancel", action)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestListOperations(t *testing.T) {
	srv, _ := newTestServer()
	doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-10"))
	doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("delete", "k-11"))
	doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-12"))

	rec := doRequest(t, srv, http.MethodGet, "/v1/operations?tenant_id=tenant-a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp listOperationsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total != 3 || len(resp.Operations) != 3 {
		t.Fatalf("total = %d, items = %d", resp.Total, len(resp.Operations))
	}
	if resp.Operations[0].LastObservedAt.IsZero() {
		t.Fatal("list items must carry lastObservedAt")
	}

	rec = doRequest(t, srv, http.MethodGet, "/v1/operations?tenant_id=tenant-a&status=pending_approval", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total != 1 || resp.Operations[0].OperationType != "delete" {
		t.Fatalf("filtered = %+v", resp)
	}

	rec = doRequest(t, srv, http.MethodGet, "/v1/operations?tenant_id=tenant-a&type=deploy&limit=1&offset=1", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Total != 2 || len(resp.Operations) != 1 {
		t.Fatalf("paged = total %d items %d", resp.Total, len(resp.Operations))
	}

	// Tenant query input is optional and non-authoritative.
	rec = doRequest(t, srv, http.MethodGet, "/v1/operations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("missing tenant status = %d", rec.Code)
	}

	rec = doRequest(t, srv, http.MethodGet, "/v1/operations?tenant_id=tenant-a&status=bogus", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status filter = %d", rec.Code)
	}
}

func TestGetOperation(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodPost, "/v1/operations", submitBody("deploy", "k-13"))
	var created operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = doRequest(t, srv, http.MethodGet, "/v1/operations/"+created.ID+"?tenant_id=tenant-a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp operationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ID != created.ID || len(resp.Steps) != 1 {
		t.Fatalf("resp = %+v", resp)
	}

	rec = doRequest(t, srv, http.MethodGet, "/v1/operations/"+uuid.NewString()+"?tenant_id=tenant-a", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing op status = %d", rec.Code)
	}

	rec = doRequest(t, srv, http.MethodGet, "/v1/operations/not-a-uuid?tenant_id=tenant-a", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad uuid status = %d", rec.Code)
	}
}

func TestCreateRuntimeTarget(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"tenantId":"tenant-a","name":"my-edge","targetType":"edge_runtime","edgeType":"kubeedge","connectionEndpoint":"https://cloudcore:10002"}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/targets", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp runtimeTargetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "my-edge" {
		t.Fatalf("name = %q", resp.Name)
	}
	if resp.TargetType != "edge_runtime" {
		t.Fatalf("targetType = %q", resp.TargetType)
	}
	if resp.EdgeType != "kubeedge" {
		t.Fatalf("edgeType = %q", resp.EdgeType)
	}
	if resp.Distribution != "standard" {
		t.Fatalf("distribution = %q", resp.Distribution)
	}
	if !resp.IsActive {
		t.Fatal("target should be active")
	}
}

func TestCreateRuntimeTargetValidation(t *testing.T) {
	srv, _ := newTestServer()
	tests := []struct {
		name string
		body string
	}{
		{"missing name", `{"tenantId":"t","targetType":"edge_runtime"}`},
		{"missing targetType", `{"tenantId":"t","name":"t"}`},
	}
	for _, tt := range tests {
		rec := doRequest(t, srv, http.MethodPost, "/v1/targets", tt.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body %s", tt.name, rec.Code, rec.Body)
		}
	}
}

func TestListRuntimeTargets(t *testing.T) {
	srv, _ := newTestServer()
	doRequest(t, srv, http.MethodPost, "/v1/targets", `{"tenantId":"tenant-a","name":"t1","targetType":"kubernetes"}`)
	doRequest(t, srv, http.MethodPost, "/v1/targets", `{"tenantId":"tenant-a","name":"t2","targetType":"edge_runtime"}`)
	doRequest(t, srv, http.MethodPost, "/v1/targets", `{"tenantId":"tenant-b","name":"t3","targetType":"kubernetes"}`)

	rec := doRequest(t, srv, http.MethodGet, "/v1/targets?tenant_id=tenant-a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp listRuntimeTargetsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 || len(resp.Targets) != 3 {
		t.Fatalf("total = %d, items = %d", resp.Total, len(resp.Targets))
	}
}

func TestProtectedRoutesRequireTokenAndPropagateTypedContext(t *testing.T) {
	srv, _ := newTestServer()
	if rec := doUnauthenticatedRequest(srv, http.MethodGet, "/v1/operations", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/operations?tenant_id=spoofed", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("X-Tenant-ID", "spoofed")
	req.Header.Set("X-User-ID", "spoofed")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestWrongAudienceIsRejected(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := tokenTestKeys{key: key}
	manager, err := iam.NewTokenManager(iam.TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "other-service", Audiences: []string{"other-service"},
		AccessTTL: iam.MaxAccessTokenTTL, RefreshTTL: time.Hour,
	}, keys, keys, tokenTestIdentity{}, tokenTestIdentity{}, tokenTestRefreshStore{})
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := manager.Issue(context.Background(), "user", "membership-a")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "hnb-platform-api", AccessTTL: iam.MaxAccessTokenTTL,
	}, keys)
	if err != nil {
		t.Fatal(err)
	}
	st := newFakeStore()
	srv := NewServer(st, verifier)
	req := httptest.NewRequest(http.MethodGet, "/v1/operations", nil)
	req.Header.Set("Authorization", "Bearer "+access.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetRuntimeTarget(t *testing.T) {
	srv, st := newTestServer()
	doRequest(t, srv, http.MethodPost, "/v1/targets", `{"tenantId":"tenant-a","name":"my-edge","targetType":"edge_runtime"}`)
	var id string
	for k := range st.targets {
		id = k
		break
	}

	rec := doRequest(t, srv, http.MethodGet, "/v1/targets/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp runtimeTargetResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ID != id || resp.Name != "my-edge" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestDeleteRuntimeTarget(t *testing.T) {
	srv, st := newTestServer()
	doRequest(t, srv, http.MethodPost, "/v1/targets", `{"tenantId":"tenant-a","name":"to-delete","targetType":"kubernetes"}`)
	var id string
	for k := range st.targets {
		id = k
		break
	}

	rec := doRequest(t, srv, http.MethodDelete, "/v1/targets/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	rec = doRequest(t, srv, http.MethodGet, "/v1/targets/"+id, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after delete status = %d", rec.Code)
	}
}

func TestEveryProtectedRouteDeniesBeforeStoreWithoutPermission(t *testing.T) {
	resourceID := uuid.NewString()
	for _, route := range platformRoutes {
		if route.Public {
			continue
		}
		st := newFakeStore()
		srv := NewServer(st, noPermissionAuthenticator{})
		path := strings.ReplaceAll(route.Pattern, "{id}", resourceID)
		body := ""
		switch {
		case route.Pattern == "/v1/intents":
			body = `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`
		case route.Method == http.MethodPost && route.Pattern == "/v1/operations":
			body = submitBody("deploy", "denied")
		case route.Method == http.MethodPost && route.Pattern == "/v1/targets":
			body = `{"name":"target","targetType":"kubernetes"}`
		case strings.Contains(route.Pattern, "/approve") || strings.Contains(route.Pattern, "/reject") || strings.Contains(route.Pattern, "/cancel"):
			body = `{}`
		}
		req := httptest.NewRequest(route.Method, path, strings.NewReader(body))
		if route.Pattern == "/v1/intents" {
			setTestIntentDelegation(t, srv, req, body)
		} else if strings.Contains(route.Pattern, "/description") {
			setTestDescriptionDelegation(t, srv, req, resourceID)
		} else if strings.Contains(route.Pattern, "/kubeconfig:issue") {
			setTestKubeconfigDelegation(t, srv, req, resourceID)
		} else if route.Pattern == "/v1/secrets:register" {
			setTestSecretDelegation(t, srv, req)
		} else {
			req.Header.Set("Authorization", "Bearer valid-token")
		}
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, body %s", route.Method, path, recorder.Code, recorder.Body)
		}
		if st.calls != 0 {
			t.Errorf("%s %s store calls = %d", route.Method, path, st.calls)
		}
	}

	st := newFakeStore()
	srv := NewServer(st, noPermissionAuthenticator{})
	req := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden || st.calls != 0 {
		t.Fatalf("unknown status = %d, calls = %d", recorder.Code, st.calls)
	}
}

func TestForeignRuntimeTargetUUIDIsNotFound(t *testing.T) {
	srv, st := newTestServer()
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{ID: id, TenantID: "tenant-b", Name: "foreign"}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		recorder := doRequest(t, srv, method, "/v1/targets/"+id, "")
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, body %s", method, recorder.Code, recorder.Body)
		}
	}
	if _, exists := st.targets[id]; !exists {
		t.Fatal("foreign target was deleted")
	}
}

func TestSubmitIntentValid(t *testing.T) {
	srv, st := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"intent-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"releaseId":"rel-42","targetRef":"target-a","scopeRef":"ns-prod"}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp IntentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.IntentID == "" || resp.OperationID == "" || resp.PlanID == "" {
		t.Fatalf("missing fields in response: %+v", resp)
	}
	if resp.Kind != "InstallRelease" {
		t.Fatalf("kind = %q", resp.Kind)
	}
	if st.lastIntent == nil || st.lastIntent.ServiceSubject != "hnb-apiserver" || st.lastIntent.MembershipID != "membership-a" ||
		st.lastIntent.DelegationTokenID == "" || st.lastIntent.DelegationKeyID == "" ||
		st.lastIntent.AuthorizationScope.ResourceKind != "cluster" || st.lastIntent.CorrelationID != "018f6c2a-4a64-7b58-9cc3-9f70462f36c1" {
		t.Fatalf("delegated audit evidence was not preserved: %+v", st.lastIntent)
	}
}

func TestSubmitIntentRejectsMissingUserAndTamperedDelegationBeforeStore(t *testing.T) {
	body := `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"delegation-boundary","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a"}}`
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *Server, *http.Request)
	}{
		{name: "missing", setup: func(_ *testing.T, _ *Server, _ *http.Request) {}},
		{name: "user token", setup: func(_ *testing.T, _ *Server, req *http.Request) {
			req.Header.Set("Authorization", "Bearer valid-token")
		}},
		{name: "tampered", setup: func(t *testing.T, srv *Server, req *http.Request) {
			setTestIntentDelegation(t, srv, req, body)
			token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
			parts := strings.Split(token, ".")
			first := parts[2][0]
			replacement := byte('A')
			if first == replacement {
				replacement = 'B'
			}
			parts[2] = string(replacement) + parts[2][1:]
			req.Header.Set("Authorization", "Bearer "+strings.Join(parts, "."))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv, st := newTestServer()
			req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
			test.setup(t, srv, req)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized || st.calls != 0 || len(st.ops) != 0 {
				t.Fatalf("status=%d calls=%d operations=%d body=%s", rec.Code, st.calls, len(st.ops), rec.Body.String())
			}
		})
	}
}

func TestSubmitIntentRejectsForgedCorrelationEvidenceBeforeStore(t *testing.T) {
	srv, st := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"delegation-correlation","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	setTestIntentDelegation(t, srv, req, body)
	req.Header.Set("X-Correlation-ID", "018f6c2a-4a64-7b58-9cc3-9f70462f36c2")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || st.calls != 0 || len(st.ops) != 0 {
		t.Fatalf("status=%d calls=%d operations=%d body=%s", rec.Code, st.calls, len(st.ops), rec.Body.String())
	}
}

func TestSubmitIntentExactReplayReturnsOriginalCommitment(t *testing.T) {
	srv, st := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"replay-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"releaseId":"rel-42","targetRef":"target-a","scopeRef":"ns-prod"}}`
	first := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	replayBody := strings.Replace(body, "018f6c2a-4a64-7b58-9cc3-9f70462f36c1", "018f6c2a-4a64-7b58-9cc3-9f70462f36c2", 1)
	replay := doRequest(t, srv, http.MethodPost, "/v1/intents", replayBody)
	if first.Code != http.StatusAccepted || replay.Code != http.StatusAccepted {
		t.Fatalf("statuses first=%d replay=%d", first.Code, replay.Code)
	}
	var original, repeated IntentResponse
	if json.Unmarshal(first.Body.Bytes(), &original) != nil || json.Unmarshal(replay.Body.Bytes(), &repeated) != nil {
		t.Fatal("decode replay responses")
	}
	if original.IntentID != repeated.IntentID || original.PlanID != repeated.PlanID || original.OperationID != repeated.OperationID || original.CorrelationID != repeated.CorrelationID || original.Status != repeated.Status {
		t.Fatalf("commitment changed: original=%+v replay=%+v", original, repeated)
	}
	if original.Replayed || !repeated.Replayed || len(st.ops) != 1 || len(st.commitments) != 1 {
		t.Fatalf("replay flags or side effects: original=%v replay=%v ops=%d commitments=%d", original.Replayed, repeated.Replayed, len(st.ops), len(st.commitments))
	}
}

func TestSubmitIntentSemanticConflictHasNoSideEffects(t *testing.T) {
	srv, st := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"conflict-1"},"spec":{"releaseId":"rel-42","targetRef":"target-a","scopeRef":"ns-prod"}}`
	if rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body); rec.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", rec.Code, rec.Body.String())
	}
	conflictBody := strings.Replace(body, "target-a", "target-b", 1)
	conflict := doRequest(t, srv, http.MethodPost, "/v1/intents", conflictBody)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"IDEMPOTENCY_CONFLICT"`) {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	if len(st.ops) != 1 || len(st.commitments) != 1 {
		t.Fatalf("conflict created side effects: ops=%d commitments=%d", len(st.ops), len(st.commitments))
	}
}

func TestSubmitIntentRejectsBoundaryDigestMismatch(t *testing.T) {
	srv, st := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"digest-1"},"spec":{"releaseId":"rel-42","targetRef":"target-a","scopeRef":"ns-prod"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	setTestIntentDelegation(t, srv, req, body)
	req.Header.Set("X-Semantic-Digest", "sha256:incorrect")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || len(st.ops) != 0 {
		t.Fatalf("status=%d operations=%d body=%s", rec.Code, len(st.ops), rec.Body.String())
	}
}

func TestSubmitClusterIntentRejectsCrossTenantTargetWithoutMutation(t *testing.T) {
	srv, st := newTestServer()
	targetID := uuid.NewString()
	st.targets[targetID] = &store.RuntimeTarget{ID: targetID, TenantID: "tenant-b", TargetType: "kubernetes", ProjectionVersion: 3, IsActive: true}
	body := clusterExistingTargetIntent(string(engine.IntentUpgradeRuntimeTarget), targetID, "KubernetesTarget", 3)

	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(st.ops) != 0 {
		t.Fatalf("unexpected operations: %d", len(st.ops))
	}
}

func TestSubmitClusterIntentRejectsWrongTargetKindWithoutMutation(t *testing.T) {
	srv, st := newTestServer()
	targetID := uuid.NewString()
	st.targets[targetID] = &store.RuntimeTarget{ID: targetID, TenantID: "tenant-a", TargetType: "edge_runtime", ProjectionVersion: 3, IsActive: true}
	body := clusterExistingTargetIntent(string(engine.IntentUpgradeRuntimeTarget), targetID, "KubernetesTarget", 3)

	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(st.ops) != 0 {
		t.Fatalf("unexpected operations: %d", len(st.ops))
	}
}

func TestSubmitClusterIntentRejectsTargetVersionConflictWithoutMutation(t *testing.T) {
	srv, st := newTestServer()
	targetID := uuid.NewString()
	st.targets[targetID] = &store.RuntimeTarget{ID: targetID, TenantID: "tenant-a", TargetType: "kubernetes", ProjectionVersion: 4, IsActive: true}
	body := clusterExistingTargetIntent(string(engine.IntentUpgradeRuntimeTarget), targetID, "KubernetesTarget", 3)

	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"TARGET_VERSION_CONFLICT"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(st.ops) != 0 {
		t.Fatalf("unexpected operations: %d", len(st.ops))
	}
}

func TestSubmitClusterIntentUsesCurrentPermissions(t *testing.T) {
	st := newFakeStore()
	targetID := uuid.NewString()
	st.targets[targetID] = &store.RuntimeTarget{ID: targetID, TenantID: "tenant-a", TargetType: "kubernetes", ProjectionVersion: 3, IsActive: true}
	resolver := testPermissionResolver{
		policy: "default:2",
		permissions: []iam.ScopedPermission{{
			TenantID: "tenant-a", ResourceKind: string(iam.ResourceCluster), ResourceID: targetID, Action: iam.ActionRead,
		}},
	}
	srv := NewServer(st, testAuthenticator{}, resolver)
	body := clusterExistingTargetIntent(string(engine.IntentUpgradeRuntimeTarget), targetID, "KubernetesTarget", 3)

	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)

	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(st.ops) != 0 {
		t.Fatalf("unexpected operations: %d", len(st.ops))
	}
}

func TestSubmitClusterIntentRejectsSecretWithWrongPurpose(t *testing.T) {
	st := &secretAwareStore{fakeStore: newFakeStore(), metadata: &store.SecretReferenceMetadata{
		TenantID: "tenant-a", Provider: "vault", Scope: "tenant:tenant-a", Name: "edge-credential",
		Purpose: "kubeconfig", AllowedLifecycleProviderID: "runtime-target.lifecycle.edge",
	}}
	srv := NewServer(st, testAuthenticator{})
	body := `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"edge-import","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"EdgeRuntimeTarget","displayName":"edge-a","cloudCoreEndpoint":"wss://cloudcore.internal:10002","credentialSecretRef":{"provider":"vault","scope":"tenant:tenant-a","name":"edge-credential"}}}`

	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)

	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"SECRET_REFERENCE_DENIED"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(st.ops) != 0 {
		t.Fatalf("unexpected operations: %d", len(st.ops))
	}
}

func TestStaleTargetChallengeCommitsNothingUntilBoundConfirmation(t *testing.T) {
	st := newFakeStore()
	targetID := uuid.NewString()
	observedAt := time.Now().Add(-time.Hour)
	st.targets[targetID] = &store.RuntimeTarget{
		ID: targetID, TenantID: "tenant-a", TargetType: "kubernetes", ProjectionVersion: 3, IsActive: true,
		ObservedAt: &observedAt, LastKnownStateAt: &observedAt, StaleThresholdSec: 300,
		LifecycleState: "ACTIVE", HealthState: "HEALTHY", ConnectivityState: "DISCONNECTED",
		ObservationGeneration: 2, ObservationRevision: 9,
	}
	signer, err := stalepolicy.NewSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, testAuthenticator{})
	srv.ConfigureStaleAdmission(signer, stalepolicy.DefaultPolicy())
	body := clusterExistingTargetIntent(string(engine.IntentUpgradeRuntimeTarget), targetID, "KubernetesTarget", 3)

	challenge := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if challenge.Code != http.StatusConflict || !strings.Contains(challenge.Body.String(), `"code":"STALE_CONFIRMATION_REQUIRED"`) {
		t.Fatalf("status=%d body=%s", challenge.Code, challenge.Body.String())
	}
	if len(st.ops) != 0 || len(st.byIdemKey) != 0 {
		t.Fatalf("challenge reserved execution state: ops=%d idempotency=%d", len(st.ops), len(st.byIdemKey))
	}
	if len(st.audits) != 1 || st.audits[0].EventType != "stale_challenge_issued" {
		t.Fatalf("challenge audit = %+v", st.audits)
	}
	var problem map[string]any
	if err := json.Unmarshal(challenge.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	token, _ := problem["confirmation"].(string)
	if token == "" {
		t.Fatal("challenge confirmation missing")
	}
	confirmedBody := strings.TrimSuffix(body, "}}") + fmt.Sprintf(`,"riskConfirmation":{"acknowledged":true,"confirmation":%q}}}`, token)
	confirmed := doRequest(t, srv, http.MethodPost, "/v1/intents", confirmedBody)
	if confirmed.Code != http.StatusAccepted {
		t.Fatalf("confirmed status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	var response IntentResponse
	if err := json.Unmarshal(confirmed.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != store.StatusPendingApproval || len(st.ops) != 1 {
		t.Fatalf("status=%s operations=%d", response.Status, len(st.ops))
	}
	if len(st.audits) != 2 || st.audits[1].Outcome != "require_approval" {
		t.Fatalf("confirmation audit = %+v", st.audits)
	}
	for _, audit := range st.audits {
		encoded, _ := json.Marshal(audit)
		if strings.Contains(string(encoded), token) {
			t.Fatal("raw challenge token leaked into audit")
		}
	}
}

func TestStalePolicyHTTPOutcomes(t *testing.T) {
	cases := []struct {
		outcome    string
		wantCode   int
		wantStatus string
		wantOps    int
	}{
		{"allow", http.StatusAccepted, store.StatusQueued, 1},
		{"require_approval", http.StatusAccepted, store.StatusPendingApproval, 1},
		{"queued_offline", http.StatusAccepted, store.StatusQueuedOffline, 1},
		{"deny", http.StatusConflict, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.outcome, func(t *testing.T) {
			st := newFakeStore()
			targetID := uuid.NewString()
			observedAt := time.Now().Add(-time.Hour)
			st.targets[targetID] = &store.RuntimeTarget{
				ID: targetID, TenantID: "tenant-a", TargetType: "kubernetes", ProjectionVersion: 3, IsActive: true,
				ObservedAt: &observedAt, StaleThresholdSec: 300, ObservationGeneration: 1, ObservationRevision: 2,
			}
			signer, err := stalepolicy.NewSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			policy, err := stalepolicy.NewPolicy(tc.outcome, tc.outcome)
			if err != nil {
				t.Fatal(err)
			}
			srv := NewServer(st, testAuthenticator{})
			srv.ConfigureStaleAdmission(signer, policy)
			body := clusterExistingTargetIntent(string(engine.IntentUpgradeRuntimeTarget), targetID, "KubernetesTarget", 3)
			challenge := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
			var problem map[string]any
			if err := json.Unmarshal(challenge.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			token, _ := problem["confirmation"].(string)
			confirmedBody := strings.TrimSuffix(body, "}}") + fmt.Sprintf(`,"riskConfirmation":{"acknowledged":true,"confirmation":%q}}}`, token)
			confirmed := doRequest(t, srv, http.MethodPost, "/v1/intents", confirmedBody)
			if confirmed.Code != tc.wantCode {
				t.Fatalf("status=%d body=%s", confirmed.Code, confirmed.Body.String())
			}
			if len(st.ops) != tc.wantOps {
				t.Fatalf("operations=%d want=%d", len(st.ops), tc.wantOps)
			}
			if tc.wantStatus != "" {
				var response IntentResponse
				if err := json.Unmarshal(confirmed.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Status != tc.wantStatus {
					t.Fatalf("operation status=%s want=%s", response.Status, tc.wantStatus)
				}
			}
		})
	}
}

func TestSubmitIntentRejectsStepsField(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"i1"},"spec":{"releaseId":"r1","targetRef":"t1","scopeRef":"s1","steps":[{"id":"hack"}]}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for steps field injection, status = %d", rec.Code)
	}
}

func TestSubmitIntentRejectsCredentialField(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"i2"},"spec":{"releaseId":"r1","targetRef":"t1","scopeRef":"s1","parameters":{"credential":"secret123"}}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for credential field injection, status = %d", rec.Code)
	}
}

func TestSubmitIntentRejectsUnknownTopLevelField(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"i3"},"spec":{"releaseId":"r1","targetRef":"t1","scopeRef":"s1"},"extra":"rejected"}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown top-level field, status = %d", rec.Code)
	}
}

func TestSubmitIntentRejectsInvalidKind(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"DeleteRelease","metadata":{"idempotencyKey":"i4"},"spec":{"releaseId":"r1","targetRef":"t1","scopeRef":"s1"}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid kind, status = %d", rec.Code)
	}
}

func TestSubmitIntentRejectsMissingRequiredFields(t *testing.T) {
	srv, _ := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"i5"}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing spec fields, status = %d", rec.Code)
	}
}

func TestConsoleBootstrap(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodGet, "/v1/console/bootstrap", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, body %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["subject"] == nil {
		t.Fatal("missing subject in bootstrap response")
	}
	subject, ok := resp["subject"].(map[string]any)
	if !ok {
		t.Fatal("subject is not an object")
	}
	if subject["type"] != "user" {
		t.Fatalf("subject type = %q", subject["type"])
	}
	if memberships, ok := resp["memberships"].([]any); !ok || len(memberships) == 0 {
		t.Fatal("no memberships in bootstrap response")
	}
	if capabilities, ok := resp["capabilities"].([]any); !ok || len(capabilities) == 0 {
		t.Fatal("no capabilities in bootstrap response")
	}
	if perms, ok := resp["permissions"].([]any); !ok || len(perms) == 0 {
		t.Fatal("no permissions in bootstrap response")
	}
}

func TestSessionBootstrapAlias(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodGet, "/v1/session/bootstrap", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, body %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["subject"] == nil || resp["selectedTenantId"] == "" {
		t.Fatalf("unexpected bootstrap response: %#v", resp)
	}
}

func TestConsoleBootstrapRequiresAuth(t *testing.T) {
	srv, _ := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/console/bootstrap", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated bootstrap status = %d", rec.Code)
	}
}

func TestMenusRouteRemovedFromPlatformAPI(t *testing.T) {
	srv, _ := newTestServer()
	rec := doRequest(t, srv, http.MethodGet, "/api/v1/menus", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("menus route status = %d", rec.Code)
	}
}

func TestProviderRoutesRemainCatalogOnly(t *testing.T) {
	for _, route := range platformRoutes {
		if strings.Contains(route.Pattern, "/providers/") && strings.Contains(route.Pattern, "lifecycle") {
			t.Fatalf("platform-api exposes provider lifecycle route: %+v", route)
		}
	}
}

func TestErrorResponseIncludesTraceID(t *testing.T) {
	srv, _ := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/operations/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("X-Trace-Id", "11111111-1111-4111-8111-111111111111")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Header().Get("X-Trace-Id") != "11111111111141118111111111111111" {
		t.Fatalf("trace header = %q", rec.Header().Get("X-Trace-Id"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["traceId"] != "11111111111141118111111111111111" || body["code"] == nil || body["detail"] == nil {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}
}

func TestIntentsRequireExecutePermission(t *testing.T) {
	st := newFakeStore()
	srv := NewServer(st, noPermissionAuthenticator{})
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"i-perm"},"spec":{"releaseId":"r1","targetRef":"t1","scopeRef":"s1"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	setTestIntentDelegation(t, srv, req, body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for no permission, status = %d", rec.Code)
	}
}

func TestIntentsRouteNotInTestDenial(t *testing.T) {
	_ = uuid.New()
	st := newFakeStore()
	srv := NewServer(st, noPermissionAuthenticator{})
	body := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"i-route"},"spec":{"releaseId":"r1","targetRef":"t1","scopeRef":"s1"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/intents", strings.NewReader(body))
	setTestIntentDelegation(t, srv, req, body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("route /v1/intents should deny without permission, got %d", rec.Code)
	}
}

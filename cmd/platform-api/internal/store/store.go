package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound                   = errors.New("operation not found")
	ErrInvalidState               = errors.New("operation state does not allow this action")
	ErrTargetNotFound             = errors.New("runtime target not found")
	ErrTargetVersionConflict      = errors.New("runtime target version conflict")
	ErrStorageBindingConflict     = errors.New("storage binding version or identity conflict")
	ErrStorageObservationConflict = errors.New("storage class observation is stale")
	ErrSecretReferenceDenied      = errors.New("secret reference denied")
	ErrClusterNotFound            = errors.New("cluster not found")
	ErrTenantMismatch             = errors.New("tenant mismatch")
	ErrIdempotencyConflict        = errors.New("idempotency key conflict: same key with different payload")
	ErrForeignKeyViolation        = errors.New("foreign key violation")
	ErrSerializationFailure       = errors.New("serialization failure")
	ErrDeadlockDetected           = errors.New("deadlock detected")
)

// StepRequestMessage mirrors the payload consumed by operation-worker from the
// hnb.command.operation.step-requested.v1 subject (see
// cmd/operation-worker/internal/nats/worker.go).
const (
	StepRequestedSubject = "hnb.command.operation.step-requested.v1"
	SchemaVersion        = "1.0.0"
)

type StepInput struct {
	PlanStepID      string            `json:"id,omitempty"`
	Name            string            `json:"name"`
	StepType        string            `json:"step_type"`
	ProviderID      string            `json:"provider_id,omitempty"`
	DependsOn       []string          `json:"depends_on,omitempty"`
	Optional        bool              `json:"optional"`
	Inputs          map[string]string `json:"inputs,omitempty"`
	SecretReference string            `json:"secret_reference,omitempty"`
	MaxRetries      int               `json:"max_retries"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
}

type SubmitCommand struct {
	TenantID         string
	ProjectID        string
	EnvironmentID    string
	NamespaceID      string
	ReleaseID        string
	OperationType    string
	IdempotencyKey   string
	InitiatedBy      string
	CorrelationID    string
	TargetClusterIDs []string
	Tags             map[string]string
	Steps            []StepInput
}

type Operation struct {
	IntentID           string
	ID                 string
	TenantID           string
	ProjectID          string
	EnvironmentID      string
	NamespaceID        string
	PlanID             string
	OperationType      string
	Status             string
	InitiatedBy        string
	ApprovedBy         string
	CorrelationID      string
	IdempotencyKey     string
	PlanDigest         string
	StatusReason       string
	TotalSteps         int
	CompletedSteps     int
	FailedSteps        int
	Version            int64
	TargetClusterIDs   []string
	Tags               map[string]string
	CreatedAt          time.Time
	StartedAt          *time.Time
	CompletedAt        *time.Time
	LastStateChangedAt time.Time
	Steps              []Step
}

type IntentCommitment struct {
	IntentID        string
	ExecutionPlanID string
	OperationID     string
	SemanticDigest  string
	Kind            string
	Action          string
	AcceptedStatus  string
	CorrelationID   string
	CreatedAt       time.Time
	HTTPStatus      int
}

type Step struct {
	ID           string
	PlanStepID   string
	Name         string
	StepType     string
	ProviderID   string
	Status       string
	DependsOn    []string
	ErrorMessage string
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type OperationSummary struct {
	ID                 string
	IntentID           string
	TenantID           string
	ProjectID          string
	EnvironmentID      string
	NamespaceID        string
	OperationType      string
	Status             string
	TotalSteps         int
	CompletedSteps     int
	FailedSteps        int
	InitiatedBy        string
	ApprovedBy         string
	Summary            string
	TargetClusterIDs   []string
	CreatedAt          time.Time
	StartedAt          *time.Time
	CompletedAt        *time.Time
	LastStateChangedAt time.Time
}

type ListQuery struct {
	TenantID      string
	Status        string
	OperationType string
	Limit         int
	Offset        int
}

type RuntimeTarget struct {
	ID                    string            `json:"id"`
	TenantID              string            `json:"tenant_id"`
	Name                  string            `json:"name"`
	DisplayName           string            `json:"display_name,omitempty"`
	Description           string            `json:"description,omitempty"`
	TargetType            string            `json:"target_type"`
	Distribution          string            `json:"distribution"`
	EdgeType              string            `json:"edge_type,omitempty"`
	EdgeConfig            map[string]any    `json:"edge_config,omitempty"`
	ConnectionType        string            `json:"connection_type"`
	ConnectionEndpoint    string            `json:"connection_endpoint,omitempty"`
	AgentVersion          string            `json:"agent_version,omitempty"`
	KubernetesVersion     string            `json:"kubernetes_version,omitempty"`
	Status                string            `json:"status"`
	Labels                map[string]string `json:"labels"`
	ObservedAt            *time.Time        `json:"observed_at,omitempty"`
	LastKnownStateAt      *time.Time        `json:"last_known_state_at,omitempty"`
	LifecycleState        string            `json:"lifecycle_state"`
	HealthState           string            `json:"health_state"`
	ConnectivityState     string            `json:"connectivity_state"`
	FreshnessState        string            `json:"freshness_state"`
	ObservationGeneration int64             `json:"observation_generation"`
	ObservationRevision   int64             `json:"observation_revision"`
	StaleThresholdSec     int               `json:"stale_threshold_seconds"`
	IsActive              bool              `json:"is_active"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
	ProjectionVersion     int64             `json:"projection_version"`
	CredentialRef         *CredentialRef    `json:"credential_ref,omitempty"`
}

// CredentialRef is the secret reference bound to a runtime target at
// create/import time. Exact credential resolution (e.g. kubeconfig download)
// uses this reference; targets without one (legacy) fall back to the
// tenant-latest heuristic.
type CredentialRef struct {
	Provider string `json:"provider,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
}

type SecretReferenceMetadata struct {
	TenantID                   string
	Provider                   string
	Scope                      string
	Name                       string
	Version                    string
	Purpose                    string
	AllowedLifecycleProviderID string
}

// RegisterSecretReferenceRequest carries the fields needed to create a
// tenant-scoped secret reference (encrypted at rest).
type RegisterSecretReferenceRequest struct {
	TenantID                   string
	Scope                      string
	Name                       string
	Purpose                    string
	AllowedLifecycleProviderID string
	EncryptedValue             string
	Algorithm                  string
	SubjectID                  string
}

type SecretMetadataStore interface {
	ResolveSecretReference(context.Context, string, string, string, string, string) (*SecretReferenceMetadata, error)
	RegisterSecretReference(context.Context, RegisterSecretReferenceRequest) (*SecretReferenceMetadata, error)
	LatestKubeConfigEncrypted(context.Context, string) (string, error)
	KubeConfigEncryptedForRef(context.Context, string, string, string) (string, error)
}

type SecurityAuditRecord struct {
	TenantID      string
	SubjectID     string
	EventType     string
	Decision      string
	ReasonCode    string
	Action        string
	ResourceID    string
	CorrelationID string
	TraceID       string
	Outcome       string
	Detail        map[string]any
}

type SecurityAuditStore interface {
	AppendSecurityAudit(context.Context, SecurityAuditRecord) error
}

// ClusterRepository defines the cluster registry data-access contract.
type ClusterRepository interface {
	CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error)
	GetCluster(ctx context.Context, id, tenantID string) (*Cluster, error)
	ListClusters(ctx context.Context, tenantID string) ([]*Cluster, error)
	DeleteCluster(ctx context.Context, id, tenantID string) error
	HeartbeatCluster(ctx context.Context, id, tenantID string) error
	UpdateCluster(ctx context.Context, id, tenantID string, req UpdateClusterRequest) (*Cluster, error)
}

// ManifestStore defines the provider manifest data-access contract.
type ManifestStore interface {
	GetManifest(ctx context.Context, providerID string) (*ProviderManifest, error)
	SaveManifest(ctx context.Context, manifest *ProviderManifest) error
	DeleteManifest(ctx context.Context, providerID string) error
	CheckCompatibility(ctx context.Context, coreVersion, providerID, providerVersion, targetType string) (*CompatibilityEntry, error)
	SaveCompatibility(ctx context.Context, entry *CompatibilityEntry) error
	ExpireConformance(ctx context.Context) ([]string, error)
	CheckProviderConformance(ctx context.Context, providerID string) error
	UpdateConformanceLevel(ctx context.Context, providerID, level string, expiresAt *time.Time) error
}

// OperationStore defines the operation engine data-access contract.
type OperationStore interface {
	SubmitOperation(ctx context.Context, cmd SubmitCommand) (op *Operation, created bool, err error)
	SubmitIntent(ctx context.Context, cmd IntentSubmitCommand) (op *Operation, created bool, err error)
	ApproveOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error)
	RejectOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error)
	CancelOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error)
	GetOperation(ctx context.Context, id, tenantID string) (*Operation, error)
	ListOperations(ctx context.Context, q ListQuery) ([]OperationSummary, int, error)
	GetIntentCommitment(ctx context.Context, tenantID, kind, action, idempotencyKey string) (*IntentCommitment, error)
}

// BatchOperationStore is optional so existing single-operation stores and
// test doubles remain compatible while PostgreSQL enables batch orchestration.
type BatchOperationStore interface {
	CreateOperationBatch(ctx context.Context, batch OperationBatch) (*OperationBatch, bool, error)
	GetOperationBatch(ctx context.Context, id, tenantID string) (*OperationBatch, error)
	AttachOperationBatchChild(ctx context.Context, batchID, operationID, targetID string, ordinal int) error
	RefreshOperationBatchStatus(ctx context.Context, batchID, tenantID string) (*OperationBatch, error)
}

type OperationBatch struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	InitiatedBy       string    `json:"initiated_by"`
	CorrelationID     string    `json:"correlation_id"`
	IdempotencyKey    string    `json:"idempotency_key"`
	TargetIDs         []string  `json:"target_ids"`
	TotalChildren     int       `json:"total_children"`
	SucceededChildren int       `json:"succeeded_children"`
	FailedChildren    int       `json:"failed_children"`
	CancelledChildren int       `json:"cancelled_children"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RuntimeTargetStore defines the runtime-target data-access contract.
type RuntimeTargetStore interface {
	CreateRuntimeTarget(ctx context.Context, rt *RuntimeTarget) error
	GetRuntimeTarget(ctx context.Context, id, tenantID string) (*RuntimeTarget, error)
	ListRuntimeTargets(ctx context.Context, tenantID string) ([]*RuntimeTarget, error)
	UpdateRuntimeTargetStatus(ctx context.Context, id, tenantID string, status string, observedAt time.Time) error
	UpdateRuntimeTargetDescription(ctx context.Context, id, tenantID, description string) error
	DeleteRuntimeTarget(ctx context.Context, id, tenantID string) error
}

// Store is the aggregate interface used by the HTTP layer.
// Each domain interface can be used independently for testing and DI.
type Store interface {
	Ping(ctx context.Context) error
	Ready(ctx context.Context) error
	OperationStore
	RuntimeTargetStore
	ClusterRepository
	ManifestStore
	SecretMetadataStore
	SecurityAuditStore
}

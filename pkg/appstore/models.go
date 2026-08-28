package appstore

import "time"

type PublisherStatus string

const (
	PubActive         PublisherStatus = "active"
	PubSuspended      PublisherStatus = "suspended"
	PubDecommissioned PublisherStatus = "decommissioned"
)

type ProductCategory string

const (
	CatApplication ProductCategory = "application"
	CatDatabase    ProductCategory = "database"
	CatMiddleware  ProductCategory = "middleware"
	CatAI          ProductCategory = "ai"
	CatEdge        ProductCategory = "edge"
	CatTool        ProductCategory = "tool"
	CatOther       ProductCategory = "other"
)

type PackageType string

const (
	PkgHelm      PackageType = "helm"
	PkgContainer PackageType = "container"
	PkgOCI       PackageType = "oci_artifact"
	PkgTerraform PackageType = "terraform"
	PkgCompose   PackageType = "compose"
	PkgCustom    PackageType = "custom"
)

type ArtifactType string

const (
	ArtOCI           ArtifactType = "oci_image"
	ArtHelmChart     ArtifactType = "helm_chart"
	ArtContainer     ArtifactType = "container_image"
	ArtTerraformMod  ArtifactType = "terraform_module"
	ArtJAR           ArtifactType = "jar"
	ArtWAR           ArtifactType = "war"
	ArtOperator      ArtifactType = "operator"
	ArtConfiguration ArtifactType = "configuration"
	ArtModel         ArtifactType = "model"
	ArtPrompt        ArtifactType = "prompt"
	ArtGuardrail     ArtifactType = "guardrail"
	ArtEvaluation    ArtifactType = "evaluation"
	ArtSBOM          ArtifactType = "sbom"
	ArtOfflineBundle ArtifactType = "offline_bundle"
	ArtGeneric       ArtifactType = "generic"
)

type ArtifactVerificationStatus string

const (
	ArtifactPending  ArtifactVerificationStatus = "pending"
	ArtifactVerified ArtifactVerificationStatus = "verified"
	ArtifactFailed   ArtifactVerificationStatus = "failed"
)

type ArtifactLifecycleState string

const (
	ArtifactActive     ArtifactLifecycleState = "active"
	ArtifactTombstoned ArtifactLifecycleState = "tombstoned"
	ArtifactDeleting   ArtifactLifecycleState = "deleting"
	ArtifactDeleted    ArtifactLifecycleState = "deleted"
)

type ChannelType string

const (
	ChanDev        ChannelType = "dev"
	ChanStaging    ChannelType = "staging"
	ChanStable     ChannelType = "stable"
	ChanDeprecated ChannelType = "deprecated"
	ChanWithdrawn  ChannelType = "withdrawn"
)

type EntitlementType string

const (
	EntEvaluate   EntitlementType = "evaluate"
	EntStandard   EntitlementType = "standard"
	EntPremium    EntitlementType = "premium"
	EntEnterprise EntitlementType = "enterprise"
)

type ReleaseStatus string

const (
	RelDraft      ReleaseStatus = "draft"
	RelPublished  ReleaseStatus = "published"
	RelSuperseded ReleaseStatus = "superseded"
	RelWithdrawn  ReleaseStatus = "withdrawn"
)

type ProductStatus string

const (
	ProdDraft     ProductStatus = "draft"
	ProdPublished ProductStatus = "published"
	ProdArchived  ProductStatus = "archived"
)

// Publisher
type Publisher struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description,omitempty"`
	Status      PublisherStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Product
type Product struct {
	ID                  string            `json:"id"`
	PublisherID         string            `json:"publisher_id"`
	Name                string            `json:"name"`
	DisplayName         string            `json:"display_name"`
	Description         string            `json:"description,omitempty"`
	Category            ProductCategory   `json:"category"`
	Labels              map[string]string `json:"labels,omitempty"`
	Status              ProductStatus     `json:"status"`
	Visibility          string            `json:"visibility,omitempty"`
	VisibilityChangedAt *time.Time        `json:"visibility_changed_at,omitempty"`
	VisibilityChangedBy string            `json:"visibility_changed_by,omitempty"`
	ReleaseCount        int               `json:"release_count,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// Package
type Package struct {
	ID          string      `json:"id"`
	ProductID   string      `json:"product_id"`
	Name        string      `json:"name"`
	PackageType PackageType `json:"package_type"`
	Description string      `json:"description,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Artifact
type Artifact struct {
	ID                 string                     `json:"id"`
	TenantID           string                     `json:"tenant_id"`
	PackageID          string                     `json:"package_id,omitempty"`
	StorageProfileID   string                     `json:"storage_profile_id,omitempty"`
	Name               string                     `json:"name"`
	ArtifactType       ArtifactType               `json:"artifact_type"`
	MediaType          string                     `json:"media_type,omitempty"`
	Repository         string                     `json:"repository"`
	RegistryURL        string                     `json:"registry_url,omitempty"`
	Digest             string                     `json:"digest"`
	SizeBytes          int64                      `json:"size_bytes,omitempty"`
	VerificationStatus ArtifactVerificationStatus `json:"verification_status"`
	LifecycleState     ArtifactLifecycleState     `json:"lifecycle_state"`
	Metadata           map[string]any             `json:"metadata,omitempty"`
	CreatedAt          time.Time                  `json:"created_at"`
	UpdatedAt          *time.Time                 `json:"updated_at,omitempty"`
}

// ArtifactDescriptor is the canonical control-plane view of Artifact identity.
type ArtifactDescriptor = Artifact

// Release
type Release struct {
	ID             string        `json:"id"`
	ProductID      string        `json:"product_id"`
	Version        string        `json:"version"`
	ReleaseNotes   string        `json:"release_notes,omitempty"`
	Manifest       any           `json:"manifest"`
	ManifestDigest string        `json:"manifest_digest"`
	Status         ReleaseStatus `json:"status"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
	PublishedAt    *time.Time    `json:"published_at,omitempty"`
}

type ReleaseArtifact struct {
	ReleaseID  string    `json:"release_id"`
	ArtifactID string    `json:"artifact_id"`
	Purpose    string    `json:"purpose"`
	Position   int       `json:"position"`
	Digest     string    `json:"digest"`
	CreatedAt  time.Time `json:"created_at"`
}

// Channel
type Channel struct {
	ID             string      `json:"id"`
	ProductID      string      `json:"product_id"`
	Name           string      `json:"name"`
	ChannelType    ChannelType `json:"channel_type"`
	ReleaseID      string      `json:"release_id,omitempty"`
	PromotionOrder int         `json:"promotion_order"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Entitlement
type Entitlement struct {
	ID              string          `json:"id"`
	ProductID       string          `json:"product_id"`
	TenantID        string          `json:"tenant_id"`
	EntitlementType EntitlementType `json:"entitlement_type"`
	MaxDeployments  int             `json:"max_deployments,omitempty"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	IsActive        bool            `json:"is_active"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// Subscription
type Subscription struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	ProductID     string     `json:"product_id"`
	EntitlementID string     `json:"entitlement_id"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ApplicationGroup (microservice deployment unit)
type GroupType string

const (
	GroupSpringCloud GroupType = "springcloud"
	GroupIstio       GroupType = "istio"
	GroupCustom      GroupType = "custom"
)

type ApplicationGroup struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	WorkspaceID string            `json:"workspace_id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	GroupType   GroupType         `json:"group_type"`
	Labels      map[string]string `json:"labels,omitempty"`
	Status      string            `json:"status"`
	AppCount    int               `json:"app_count,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Application (runtime instance)
type AppStatus string

const (
	AppDeploying    AppStatus = "deploying"
	AppReady        AppStatus = "ready"
	AppDegraded     AppStatus = "degraded"
	AppUninstalling AppStatus = "uninstalling"
)

type Application struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	GroupID     string         `json:"group_id,omitempty"`
	ProductID   string         `json:"product_id"`
	ReleaseID   string         `json:"release_id"`
	Name        string         `json:"name"`
	Namespace   string         `json:"namespace,omitempty"`
	Status      AppStatus      `json:"status"`
	Config      map[string]any `json:"config,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Channel promotion matrix
var validPromotions = map[ChannelType][]ChannelType{
	ChanDev:        {ChanStaging},
	ChanStaging:    {ChanStable, ChanDev},
	ChanStable:     {ChanDeprecated, ChanStaging},
	ChanDeprecated: {ChanWithdrawn, ChanStable},
	ChanWithdrawn:  {},
}

func CanPromote(from, to ChannelType) bool {
	targets, ok := validPromotions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

var ChannelOrder = map[ChannelType]int{
	ChanDev:        0,
	ChanStaging:    1,
	ChanStable:     2,
	ChanDeprecated: 3,
	ChanWithdrawn:  4,
}

// UploadSession represents a direct-to-Harbor upload session
type UploadSessionStatus string

const (
	SessionPending   UploadSessionStatus = "pending"
	SessionUploading UploadSessionStatus = "uploading"
	SessionCompleted UploadSessionStatus = "completed"
	SessionExpired   UploadSessionStatus = "expired"
	SessionFailed    UploadSessionStatus = "failed"
)

type UploadSession struct {
	ID           string              `json:"id"`
	TenantID     string              `json:"tenant_id"`
	ReleaseID    *string             `json:"release_id,omitempty"`
	Filename     string              `json:"filename"`
	ArtifactType string              `json:"artifact_type"`
	SizeBytes    int64               `json:"size_bytes"`
	Status       UploadSessionStatus `json:"status"`
	HarborURL    string              `json:"harbor_url"`
	Repository   string              `json:"repository"`
	RobotID      int                 `json:"robot_id"`
	RobotName    string              `json:"robot_name,omitempty"`
	ArtifactID   *string             `json:"artifact_id,omitempty"`
	Digest       *string             `json:"digest,omitempty"`
	ExpiresAt    time.Time           `json:"expires_at"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type StorageBackend string

const (
	StorageLocal StorageBackend = "local"
	StoragePVC   StorageBackend = "pvc"
	StorageS3    StorageBackend = "s3"
	StorageOCI   StorageBackend = "oci"
)

type StorageServiceTier string

const (
	StorageTierMinimal    StorageServiceTier = "minimal"
	StorageTierLiteHA     StorageServiceTier = "lite_ha"
	StorageTierStandard   StorageServiceTier = "standard"
	StorageTierEnterprise StorageServiceTier = "enterprise"
)

type StorageAuthorityRole string

const (
	StorageAuthoritative StorageAuthorityRole = "authoritative"
	StorageMirror        StorageAuthorityRole = "mirror"
	StorageCache         StorageAuthorityRole = "cache"
)

type StorageProfileState string

const (
	StorageProfileActive    StorageProfileState = "active"
	StorageProfileMigrating StorageProfileState = "migrating"
	StorageProfileDisabled  StorageProfileState = "disabled"
)

type ArtifactStorageProfile struct {
	ID              string               `json:"id"`
	TenantID        string               `json:"tenant_id"`
	Name            string               `json:"name"`
	Backend         StorageBackend       `json:"backend"`
	ServiceTier     StorageServiceTier   `json:"service_tier"`
	AuthorityRole   StorageAuthorityRole `json:"authority_role"`
	SecretReference string               `json:"secret_reference,omitempty"`
	Endpoint        string               `json:"endpoint,omitempty"`
	Region          string               `json:"region,omitempty"`
	RPOSeconds      int                  `json:"rpo_seconds"`
	RTOSeconds      int                  `json:"rto_seconds"`
	LifecycleState  StorageProfileState  `json:"lifecycle_state"`
	Metadata        map[string]any       `json:"metadata,omitempty"`
	CreatedBy       string               `json:"created_by"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

type ArtifactProfileMigration struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ArtifactID      string         `json:"artifact_id"`
	SourceProfileID string         `json:"source_profile_id"`
	TargetProfileID string         `json:"target_profile_id"`
	ArtifactDigest  string         `json:"artifact_digest"`
	Status          string         `json:"status"`
	OperationID     string         `json:"operation_id,omitempty"`
	Checkpoint      map[string]any `json:"checkpoint,omitempty"`
	IdempotencyKey  string         `json:"idempotency_key"`
	RequestedBy     string         `json:"requested_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type DistributionTargetRole string

const (
	DistributionRegionalMirror DistributionTargetRole = "regional_mirror"
	DistributionEdgeCache      DistributionTargetRole = "edge_cache"
)

type DistributionState string

const (
	DistributionPending DistributionState = "pending"
	DistributionSyncing DistributionState = "syncing"
	DistributionReady   DistributionState = "ready"
	DistributionStale   DistributionState = "stale"
	DistributionFailed  DistributionState = "failed"
)

type DistributionHealth string

const (
	DistributionHealthUnknown     DistributionHealth = "unknown"
	DistributionHealthHealthy     DistributionHealth = "healthy"
	DistributionHealthDegraded    DistributionHealth = "degraded"
	DistributionHealthUnavailable DistributionHealth = "unavailable"
)

type ArtifactDistributionTarget struct {
	ID                 string                 `json:"id"`
	TenantID           string                 `json:"tenant_id"`
	ArtifactID         string                 `json:"artifact_id"`
	AuthorityProfileID string                 `json:"authority_profile_id"`
	TargetProfileID    string                 `json:"target_profile_id"`
	TargetRole         DistributionTargetRole `json:"target_role"`
	DesiredDigest      string                 `json:"desired_digest"`
	ObservedDigest     string                 `json:"observed_digest,omitempty"`
	State              DistributionState      `json:"state"`
	Health             DistributionHealth     `json:"health"`
	LowWatermarkBytes  int64                  `json:"low_watermark_bytes"`
	HighWatermarkBytes int64                  `json:"high_watermark_bytes"`
	CurrentBytes       int64                  `json:"current_bytes"`
	LocalLock          bool                   `json:"local_lock"`
	RebuildOperationID string                 `json:"rebuild_operation_id,omitempty"`
	LastError          string                 `json:"last_error,omitempty"`
	IdempotencyKey     string                 `json:"idempotency_key"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

type DistributionRebuildCommand struct {
	TargetID           string `json:"target_id"`
	TenantID           string `json:"tenant_id"`
	ArtifactID         string `json:"artifact_id"`
	AuthorityProfileID string `json:"authority_profile_id"`
	TargetProfileID    string `json:"target_profile_id"`
	DesiredDigest      string `json:"desired_digest"`
	OperationID        string `json:"operation_id,omitempty"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type ArtifactReference struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	ArtifactID string     `json:"artifact_id"`
	OwnerType  string     `json:"owner_type"`
	OwnerID    string     `json:"owner_id"`
	Purpose    string     `json:"purpose"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ArtifactTombstone struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ArtifactID     string    `json:"artifact_id"`
	ArtifactDigest string    `json:"artifact_digest"`
	State          string    `json:"state"`
	DeleteAfter    time.Time `json:"delete_after"`
	OperationID    string    `json:"operation_id,omitempty"`
	RequestedBy    string    `json:"requested_by"`
	Reason         string    `json:"reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type GCPreview struct {
	ArtifactID       string              `json:"artifact_id"`
	Digest           string              `json:"digest"`
	Blocked          bool                `json:"blocked"`
	Blockers         []ArtifactReference `json:"blockers"`
	EstimatedBytes   int64               `json:"estimated_bytes"`
	EarliestDeleteAt time.Time           `json:"earliest_delete_at"`
}

type GCSweepCommand struct {
	TombstoneID    string `json:"tombstone_id"`
	TenantID       string `json:"tenant_id"`
	ArtifactID     string `json:"artifact_id"`
	ArtifactDigest string `json:"artifact_digest"`
	OperationID    string `json:"operation_id,omitempty"`
}

type RecycleBinSettings struct {
	TenantID          string    `json:"tenant_id"`
	ProductRetention  string    `json:"product_retention"`
	ReleaseRetention  string    `json:"release_retention"`
	ArtifactRetention string    `json:"artifact_retention"`
	Enabled           bool      `json:"enabled"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type RecycleBinEntry struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ResourceName string    `json:"resource_name,omitempty"`
	State        string    `json:"state"`
	DeleteAfter  time.Time `json:"delete_after"`
	OperationID  string    `json:"operation_id,omitempty"`
	RequestedBy  string    `json:"requested_by,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ScanProfile struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	Name      string         `json:"name"`
	Engine    string         `json:"engine"`
	Config    map[string]any `json:"config,omitempty"`
	Enabled   bool           `json:"enabled"`
	IsDefault bool           `json:"is_default"`
	CreatedBy string         `json:"created_by,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type VulnerabilityDB struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	ProfileID  string     `json:"profile_id"`
	Engine     string     `json:"engine"`
	DbLabel    string     `json:"db_label"`
	Policy     string     `json:"policy"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	NextSyncAt *time.Time `json:"next_sync_at,omitempty"`
	Status     string     `json:"status"`
	LastError  string     `json:"last_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type ScanReport struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ArtifactID      string         `json:"artifact_id"`
	ProfileID       string         `json:"profile_id"`
	State           string         `json:"state"`
	SeveritySummary map[string]any `json:"severity_summary,omitempty"`
	Findings        []any          `json:"findings,omitempty"`
	TriggeredBy     string         `json:"triggered_by,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

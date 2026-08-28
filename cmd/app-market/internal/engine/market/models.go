package market

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
	ArtOCIImage      ArtifactType = "oci_image"
	ArtHelmChart     ArtifactType = "helm_chart"
	ArtContainer     ArtifactType = "container_image"
	ArtTerraform     ArtifactType = "terraform_module"
	ArtGeneric       ArtifactType = "generic"
)

type Publisher struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description,omitempty"`
	Status      PublisherStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Product struct {
	ID          string          `json:"id"`
	PublisherID string          `json:"publisher_id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description,omitempty"`
	Category    ProductCategory `json:"category"`
	Labels      map[string]string `json:"labels"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
}

type Package struct {
	ID          string      `json:"id"`
	ProductID   string      `json:"product_id"`
	Name        string      `json:"name"`
	PackageType PackageType `json:"package_type"`
	Description string      `json:"description,omitempty"`
}

type Artifact struct {
	ID           string            `json:"id"`
	PackageID    string            `json:"package_id"`
	Name         string            `json:"name"`
	ArtifactType ArtifactType      `json:"artifact_type"`
	RegistryURL  string            `json:"registry_url,omitempty"`
	Digest       string            `json:"digest"`
	SizeBytes    int64             `json:"size_bytes,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type ChannelType string

const (
	ChDev        ChannelType = "dev"
	ChStaging    ChannelType = "staging"
	ChStable     ChannelType = "stable"
	ChDeprecated ChannelType = "deprecated"
	ChWithdrawn  ChannelType = "withdrawn"
)

var channelOrder = map[ChannelType]int{
	ChDev:        1,
	ChStaging:    2,
	ChStable:     3,
	ChDeprecated: 4,
	ChWithdrawn:  5,
}

var validPromotions = map[ChannelType][]ChannelType{
	ChDev:        {ChStaging},
	ChStaging:    {ChStable, ChDev},
	ChStable:     {ChDeprecated, ChStaging},
	ChDeprecated: {ChWithdrawn, ChStable},
	ChWithdrawn:  {},
}

func CanPromote(from, to ChannelType) bool {
	allowed, ok := validPromotions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

type Release struct {
	ID             string    `json:"id"`
	ProductID      string    `json:"product_id"`
	Version        string    `json:"version"`
	ReleaseNotes   string    `json:"release_notes,omitempty"`
	ManifestDigest string    `json:"manifest_digest"`
	Status         string    `json:"status"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`

	// IntentEmission records whether this release was published via the
	// canonical RuntimeIntent path (KERNEL-002).  Zero-value means the flag
	// has not been set.
	IntentEmission SystemIntentEmission `json:"intent_emission,omitempty"`
}

// SystemIntentEmission tracks that a publish event was routed through the
// canonical /v1/intents entry point rather than a direct Operation write.
type SystemIntentEmission struct {
	Kind           string    `json:"kind"`
	ReleaseID      string    `json:"release_id"`
	PublishedAt    time.Time `json:"published_at"`
	EmittedBy      string    `json:"emitted_by"`
	CanonicalPath  string    `json:"canonical_path"`
	StandalonePlan bool      `json:"standalone_plan"` // always false per KERNEL-002
}

type EntitlementType string

const (
	EntEval      EntitlementType = "evaluate"
	EntStandard  EntitlementType = "standard"
	EntPremium   EntitlementType = "premium"
	EntEnterprise EntitlementType = "enterprise"
)

type Entitlement struct {
	ID              string          `json:"id"`
	ProductID       string          `json:"product_id"`
	TenantID        string          `json:"tenant_id"`
	EntitlementType EntitlementType `json:"entitlement_type"`
	MaxDeployments  int             `json:"max_deployments,omitempty"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	IsActive        bool            `json:"is_active"`
}

type Subscription struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant_id"`
	ProductID     string      `json:"product_id"`
	EntitlementID string      `json:"entitlement_id"`
	Status        string      `json:"status"`
	StartedAt     time.Time   `json:"started_at"`
	ExpiresAt     *time.Time  `json:"expires_at,omitempty"`
}

type ReleaseManifest struct {
	ReleaseID    string            `json:"release_id"`
	ProductID    string            `json:"product_id"`
	Version      string            `json:"version"`
	Packages     []PackageRef      `json:"packages"`
	Artifacts    []ArtifactRef     `json:"artifacts"`
	Dependencies []DependencySpec  `json:"dependencies"`
	Config       map[string]string `json:"config"`
}

type PackageRef struct {
	Name         string `json:"name"`
	PackageType  string `json:"package_type"`
}

type ArtifactRef struct {
	Name      string `json:"name"`
	Digest    string `json:"digest"`
	Registry  string `json:"registry"`
	MediaType string `json:"media_type"`
}

type DependencySpec struct {
	ProductID string `json:"product_id"`
	Version   string `json:"version"`
	Required  bool   `json:"required"`
}

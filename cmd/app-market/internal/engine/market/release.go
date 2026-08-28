package market

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

var artifactDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type ReleaseManager struct{}

func NewReleaseManager() *ReleaseManager {
	return &ReleaseManager{}
}

func (rm *ReleaseManager) CreateRelease(
	productID, version, releaseNotes, createdBy string,
	manifest *ReleaseManifest,
) (*Release, error) {
	if version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	digest := sha256.Sum256(manifestJSON)
	manifestDigest := fmt.Sprintf("%x", digest[:])

	now := time.Now().UTC()
	return &Release{
		ID:             fmt.Sprintf("rel-%s", manifestDigest[:16]),
		ProductID:      productID,
		Version:        version,
		ReleaseNotes:   releaseNotes,
		ManifestDigest: manifestDigest,
		Status:         "draft",
		CreatedBy:      createdBy,
		CreatedAt:      now,
	}, nil
}

func (rm *ReleaseManager) PublishRelease(release *Release) error {
	if release.Status != "draft" {
		return fmt.Errorf("can only publish draft releases, current: %s", release.Status)
	}
	now := time.Now().UTC()
	release.Status = "published"
	release.PublishedAt = &now

	publishingSystemID := "app-market-service"
	release.IntentEmission = SystemIntentEmission{
		Kind:           "InstallRelease",
		ReleaseID:      release.ID,
		PublishedAt:    now,
		EmittedBy:      publishingSystemID,
		CanonicalPath:  "/v1/intents",
		StandalonePlan: false,
	}
	return nil
}

func (rm *ReleaseManager) ValidateManifest(manifest *ReleaseManifest) error {
	if manifest.ReleaseID == "" {
		return fmt.Errorf("release_id is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("version is required")
	}
	if len(manifest.Packages) == 0 && len(manifest.Artifacts) == 0 {
		return fmt.Errorf("at least one package or artifact is required")
	}
	for _, a := range manifest.Artifacts {
		if !artifactDigestPattern.MatchString(a.Digest) {
			return fmt.Errorf("artifact %q must use lowercase sha256 digest", a.Name)
		}
	}
	return nil
}

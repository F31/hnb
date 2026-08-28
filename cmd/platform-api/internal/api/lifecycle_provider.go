package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/core"
)

type lifecycleManifestReader interface {
	GetManifest(context.Context, string) (*store.ProviderManifest, error)
}

type lifecycleProviderResolver struct {
	store lifecycleManifestReader
	now   func() time.Time
}

func (r lifecycleProviderResolver) ResolveLifecycleProvider(ctx context.Context, decision engine.CompatibilityDecision) (engine.ProviderResolution, error) {
	if r.store == nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "provider registry is unavailable"}
	}
	manifest, err := r.store.GetManifest(ctx, decision.ProviderID)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "matrix-selected provider is not registered"}
	}
	now := time.Now().UTC()
	if r.now != nil {
		now = r.now().UTC()
	}
	if manifest.ProviderID != decision.ProviderID || manifest.Version == "" || manifest.ProtocolVersion != decision.ProviderProtocolVersion ||
		manifest.ConformanceLevel != "production_ready" || manifest.ConformanceExpiresAt == nil || !now.Before(manifest.ConformanceExpiresAt.UTC()) ||
		!containsString(manifest.Actions, decision.Action) {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "matrix-selected provider is not eligible"}
	}
	expectedTest := decision.MatrixVersion + "/" + decision.TargetKind + "/" + decision.Action
	evidenceRef := ""
	for _, evidence := range manifest.ConformanceEvidence {
		if evidence.Category == "runtime-target-lifecycle" && evidence.TestName == expectedTest && evidence.Passed && evidence.EvidenceRef != "" {
			evidenceRef = evidence.EvidenceRef
			break
		}
	}
	if evidenceRef == "" {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "exact matrix cell has no current conformance evidence"}
	}
	document, err := json.Marshal(manifest)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "provider manifest cannot be pinned"}
	}
	digest := sha256.Sum256(document)
	return engine.ProviderResolution{
		ProviderID: manifest.ProviderID, ProviderVersion: manifest.Version,
		ProviderDigest: fmt.Sprintf("sha256:%x", digest[:]), EvidenceRef: evidenceRef,
	}, nil
}

func (r lifecycleProviderResolver) ResolveStorageProvider(ctx context.Context, action string) (engine.ProviderResolution, error) {
	const providerID = "kubernetes-provider"
	if action != "import" && action != "reconcile" {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "unsupported storage provider action"}
	}
	manifest, err := r.store.GetManifest(ctx, providerID)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "storage provider is not registered"}
	}
	requiredAction := "storage.binding." + action
	now := time.Now().UTC()
	if r.now != nil {
		now = r.now().UTC()
	}
	if manifest.ProviderID != providerID || manifest.Version == "" || manifest.ProtocolVersion != "2.0.0" ||
		manifest.ConformanceLevel != "production_ready" || manifest.ConformanceExpiresAt == nil ||
		!now.Before(manifest.ConformanceExpiresAt.UTC()) || !containsString(manifest.Actions, requiredAction) {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage provider is not eligible"}
	}
	evidenceRef := ""
	for _, evidence := range manifest.ConformanceEvidence {
		if evidence.Category == "storage-class-binding" && evidence.TestName == requiredAction && evidence.Passed && evidence.EvidenceRef != "" {
			evidenceRef = evidence.EvidenceRef
			break
		}
	}
	if evidenceRef == "" {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage provider has no action-specific conformance evidence"}
	}
	document, err := json.Marshal(manifest)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage provider manifest cannot be pinned"}
	}
	digest := sha256.Sum256(document)
	return engine.ProviderResolution{ProviderID: providerID, ProviderVersion: manifest.Version, ProviderDigest: fmt.Sprintf("sha256:%x", digest[:]), EvidenceRef: evidenceRef}, nil
}

func (r lifecycleProviderResolver) ResolveStorageDriverProvider(ctx context.Context, request engine.StorageDriverRequest) (engine.ProviderResolution, error) {
	if request.Action != "install" && request.Action != "upgrade" && request.Action != "uninstall" {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "unsupported storage driver action"}
	}
	manifest, err := r.store.GetManifest(ctx, request.PackageID)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "storage driver package is unknown"}
	}
	now := time.Now().UTC()
	if r.now != nil {
		now = r.now().UTC()
	}
	pkg := manifest.StorageDriverPackage
	action := "storage.driver." + request.Action
	licensed, licensedOK := manifest.Compatibility["licensed"].(bool)
	healthy, healthyOK := manifest.Compatibility["healthy"].(bool)
	if pkg == nil || manifest.ProviderID != request.PackageID || pkg.PackageID != request.PackageID ||
		pkg.PackageVersion != request.PackageVersion || manifest.Version != request.PackageVersion || manifest.ProtocolVersion != "2.0.0" ||
		manifest.ConformanceLevel != "production_ready" || manifest.ConformanceExpiresAt == nil || !now.Before(manifest.ConformanceExpiresAt.UTC()) ||
		!licensedOK || !licensed || !healthyOK || !healthy || !containsString(manifest.Actions, action) || pkg.Validate(manifest.Version, now) != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage driver package is not eligible"}
	}
	if request.KubernetesVersion == "" || !storagePackageSupportsKubernetes(pkg, request.KubernetesVersion) {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage driver package is incompatible with the target Kubernetes version"}
	}
	if request.Action == "upgrade" && !containsString(pkg.Compatibility.UpgradeFromVersions, request.CurrentVersion) {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage driver upgrade path is not declared"}
	}
	evidenceRef := ""
	for _, evidence := range manifest.ConformanceEvidence {
		if evidence.Category == "storage-driver-lifecycle" && evidence.TestName == action && evidence.Passed && evidence.EvidenceRef != "" {
			evidenceRef = evidence.EvidenceRef
			break
		}
	}
	if evidenceRef == "" {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage driver action has no provider conformance evidence"}
	}
	document, err := json.Marshal(manifest)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "storage driver provider cannot be pinned"}
	}
	digest := sha256.Sum256(document)
	rollback := ""
	if request.Action == "upgrade" && containsString(pkg.Compatibility.RollbackToVersions, request.CurrentVersion) {
		rollback = request.CurrentVersion
	}
	claimsJSON, _ := json.Marshal(pkg.Capabilities)
	claims := map[string]any{}
	_ = json.Unmarshal(claimsJSON, &claims)
	return engine.ProviderResolution{ProviderID: manifest.ProviderID, ProviderVersion: manifest.Version,
		ProviderDigest: fmt.Sprintf("sha256:%x", digest[:]), EvidenceRef: evidenceRef,
		PackageID: pkg.PackageID, PackageVersion: pkg.PackageVersion, PackageDigest: pkg.PackageDigest,
		Provisioners: append([]string(nil), pkg.Provisioners...), CapabilityClaims: claims, RollbackVersion: rollback}, nil
}

func (r lifecycleProviderResolver) ResolveRetainedVolumeProvider(ctx context.Context, request engine.RetainedVolumeProviderRequest) (engine.ProviderResolution, error) {
	if request.ProviderID == "" || (request.Action != "sanitize" && request.Action != "manual-release") {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "unsupported retained-volume provider action"}
	}
	manifest, err := r.store.GetManifest(ctx, request.ProviderID)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderRouteNotFound, Reason: "retained-volume provider is not registered"}
	}
	now := time.Now().UTC()
	if r.now != nil {
		now = r.now().UTC()
	}
	action := "storage.retained-volume." + request.Action
	if manifest.ProviderID != request.ProviderID || manifest.Version == "" || manifest.ProtocolVersion != "2.0.0" ||
		manifest.ConformanceLevel != "production_ready" || manifest.ConformanceExpiresAt == nil || !now.Before(manifest.ConformanceExpiresAt.UTC()) ||
		!containsString(manifest.Actions, action) {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "retained-volume provider is not eligible"}
	}
	evidenceRef := ""
	for _, evidence := range manifest.ConformanceEvidence {
		if evidence.Category == "storage-retained-volume" && evidence.TestName == action && evidence.Passed && evidence.EvidenceRef != "" {
			evidenceRef = evidence.EvidenceRef
			break
		}
	}
	if evidenceRef == "" {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "retained-volume action has no current provider conformance evidence"}
	}
	document, err := json.Marshal(manifest)
	if err != nil {
		return engine.ProviderResolution{}, &engine.CompatibilityError{Code: engine.CodeProviderIncompatible, Reason: "retained-volume provider cannot be pinned"}
	}
	digest := sha256.Sum256(document)
	return engine.ProviderResolution{ProviderID: manifest.ProviderID, ProviderVersion: manifest.Version, ProviderDigest: fmt.Sprintf("sha256:%x", digest[:]), EvidenceRef: evidenceRef}, nil
}

func storagePackageSupportsKubernetes(pkg *core.StorageDriverPackage, version string) bool {
	version = strings.TrimPrefix(version, "v")
	if _, ok := storageSemver(version); !ok {
		return false
	}
	for _, candidate := range pkg.Compatibility.KubernetesVersions {
		if compareStorageSemver(version, candidate.MinInclusive) >= 0 && compareStorageSemver(version, candidate.MaxExclusive) < 0 {
			return true
		}
	}
	return false
}

func storageSemver(version string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return result, false
		}
		result[i] = value
	}
	return result, true
}

func compareStorageSemver(left, right string) int {
	a, okA := storageSemver(left)
	b, okB := storageSemver(right)
	if !okA || !okB {
		return -1
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

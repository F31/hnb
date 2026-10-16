package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var repositorySegment = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)

func imageSource(image PinnedImage) (string, bool) {
	registry, repository, ok := strings.Cut(image.Ref, "/")
	if !ok {
		return "", false
	}
	if i := strings.LastIndex(repository, ":"); i >= 0 {
		repository = repository[:i]
	}
	for _, segment := range strings.Split(repository, "/") {
		if !repositorySegment.MatchString(segment) {
			return "", false
		}
	}
	digest, valid := probeDigest(image.Digest)
	if !valid {
		return "", false
	}
	source := "https://" + registry + "/v2/" + repository
	u, err := url.Parse(source)
	if err != nil || !validProbeURL(source) || u.Host != registry {
		return "", false
	}
	return source + "/manifests/" + digest, true
}

type ociDescriptor struct {
	Digest   string       `json:"digest"`
	Size     int64        `json:"size"`
	Platform *ociPlatform `json:"platform,omitempty"`
}

type ociPlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
	Manifests     []ociDescriptor `json:"manifests"`
}

// ProbeImage checks a pinned single-manifest image plus config/layer byte paths
// from the installer. Index/platform selection and authenticated registry flows
// remain blocked until their locked release recipe support is implemented.
func (p HTTPProbe) ProbeImage(ctx context.Context, id string, image PinnedImage) []PreflightCheck {
	source, ok := imageSource(image)
	if !ok {
		return []PreflightCheck{observedCheck(id+".reference", "artifacts", "fully qualified registry/repository and pinned digest", "image source unresolved", "OCI_SOURCE_UNRESOLVED", "Resolve an explicit OCI source; no guessed mirror/tag mapping is used.", false)}
	}
	checks, body, verified := p.probeContent(ctx, id+".manifest", source, image.Digest, 1024*1024, true, false)
	if !verified {
		return checks
	}
	var manifest ociManifest
	if err := json.Unmarshal(body, &manifest); err != nil || manifest.SchemaVersion != 2 {
		return append(checks, observedCheck(id+".closure", "artifacts", "OCI/Docker image manifest schema 2", "invalid manifest", "OCI_MANIFEST_INVALID", "Resolve and verify a supported locked manifest.", false))
	}
	if len(manifest.Manifests) > 0 || manifest.MediaType == "application/vnd.oci.image.index.v1+json" || manifest.MediaType == "application/vnd.docker.distribution.manifest.list.v2+json" {
		return append(checks, observedCheck(id+".closure", "artifacts", "locked target-architecture manifest", "index verified; architecture manifest not resolved", "OCI_ARCH_MANIFEST_UNRESOLVED", "Record the verified architecture-specific manifest in the release recipe; an index HEAD is insufficient.", false))
	}
	if manifest.MediaType != "application/vnd.oci.image.manifest.v1+json" && manifest.MediaType != "application/vnd.docker.distribution.manifest.v2+json" {
		return append(checks, observedCheck(id+".closure", "artifacts", "supported OCI/Docker image media type", "unsupported media type", "OCI_MANIFEST_UNSUPPORTED", "Resolve a supported image manifest; do not infer layer access.", false))
	}
	base := source[:strings.LastIndex(source, "/manifests/")]
	descriptors := append([]ociDescriptor{manifest.Config}, manifest.Layers...)
	if len(descriptors) > 128 {
		return append(checks, observedCheck(id+".closure", "artifacts", "bounded descriptor set", "manifest descriptor count exceeds probe budget", "OCI_PROBE_LIMIT_EXCEEDED", "Use prepare to resolve the full image dependency closure.", false))
	}
	for i, descriptor := range descriptors {
		key := fmt.Sprintf("%s.blob-%d", id, i)
		digest, valid := probeDigest(descriptor.Digest)
		if !valid || descriptor.Size <= 0 || descriptor.Size > 64*1024*1024 {
			checks = append(checks, observedCheck(key, "artifacts", "valid pinned blob within the 64 MiB probe budget", "invalid or oversized descriptor", "OCI_DESCRIPTOR_UNVERIFIED", "Validate descriptor identity/size; use prepare for larger objects.", false))
			continue
		}
		blobChecks, _, _ := p.probeContent(ctx, key, base+"/blobs/"+digest, digest, descriptor.Size, false, true)
		checks = append(checks, blobChecks...)
	}
	return checks
}

func (p HTTPProbe) ProbeLockedArtifacts(ctx context.Context, lock *ReleaseLock) []PreflightCheck {
	if lock == nil {
		return []PreflightCheck{observedCheck("artifacts.recipe", "artifacts", "validated release recipe", "missing lock", "ARTIFACT_RECIPE_MISSING", "Resolve the release lock before testing sources.", false)}
	}
	var checks []PreflightCheck
	for i, image := range lock.Images {
		checks = append(checks, p.ProbeImage(ctx, fmt.Sprintf("installer.image-%d", i), image)...)
	}
	for i, artifact := range lock.NodeArtifacts {
		id := fmt.Sprintf("installer.node-artifact-%d", i)
		if artifact.Kind == "container" {
			checks = append(checks, p.ProbeImage(ctx, id, PinnedImage{Ref: artifact.Ref, Digest: artifact.Digest})...)
		} else {
			checks = append(checks, p.ProbeContent(ctx, id, artifact.Ref, artifact.Digest, 64*1024*1024)...)
		}
	}
	return checks
}

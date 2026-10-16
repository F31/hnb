package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Chart Digest is the tar.gz package digest, as emitted by lock generate. An
// OCI tag is only a locator; its content layer must equal this locked digest.
func (d *Downloader) prepareOCIChart(ctx context.Context, dir string, chart PinnedChart, result *PrepareResult) error {
	ref := strings.TrimPrefix(chart.Ref, "oci://")
	last := ref[strings.LastIndex(ref, "/")+1:]
	if !strings.ContainsAny(last, ":@") {
		ref += ":" + chart.Version
	}
	base, selector, err := registryReference(ref)
	if err != nil {
		return fmt.Errorf("invalid OCI Chart reference")
	}
	if !strings.HasPrefix(selector, "sha256:") && selector != chart.Version {
		return fmt.Errorf("OCI Chart selector differs from locked version")
	}
	if d.MirrorBase != "" {
		base = d.MirrorBase + "/v2/" + strings.SplitN(base, "/v2/", 2)[1]
	}
	// Package identity and Chart.yaml are sufficient for a verified warm hit;
	// mutable locator tags must not force another network lookup.
	path, err := d.cachedPath(dir, chart.Digest)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		artifact, err := d.fetch(ctx, dir, "chart", chart.Name, base+"/blobs/"+chart.Digest, chart.Digest, 0)
		if err != nil {
			return err
		}
		return recordPreparedChart(chart, artifact, result)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("OCI Chart cache unavailable")
	}
	token := ""
	expected := ""
	if strings.HasPrefix(selector, "sha256:") {
		expected = selector
	}
	data, _, err := d.registry.fetch(ctx, base+"/manifests/"+selector, &token, expected, 0)
	if err != nil {
		return fmt.Errorf("OCI Chart manifest unavailable or invalid")
	}
	var manifest struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Config        struct {
			MediaType string `json:"mediaType"`
		} `json:"config"`
		Layers []struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
			Size      int64  `json:"size"`
		} `json:"layers"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.SchemaVersion != 2 || manifest.MediaType != "application/vnd.oci.image.manifest.v1+json" || manifest.Config.MediaType != "application/vnd.cncf.helm.config.v1+json" || len(manifest.Layers) != 1 {
		return fmt.Errorf("unsupported OCI Helm Chart manifest")
	}
	layer := manifest.Layers[0]
	if layer.MediaType != "application/vnd.cncf.helm.chart.content.v1.tar+gzip" || layer.Digest != chart.Digest || layer.Size <= 0 {
		return fmt.Errorf("OCI Chart package differs from locked digest")
	}
	artifact, err := d.fetch(ctx, dir, "chart", chart.Name, base+"/blobs/"+chart.Digest, chart.Digest, layer.Size)
	if err != nil {
		return err
	}
	return recordPreparedChart(chart, artifact, result)
}

func recordPreparedChart(chart PinnedChart, artifact PreparedArtifact, result *PrepareResult) error {
	name, version, err := chartMetadata(artifact.Path)
	if err != nil || name != chart.Name || version != chart.Version {
		return fmt.Errorf("OCI Chart metadata differs from lock")
	}
	result.Artifacts = append(result.Artifacts, artifact)
	result.BytesVerified += artifact.Size
	if artifact.FromCache {
		result.FromCache++
	}
	return nil
}

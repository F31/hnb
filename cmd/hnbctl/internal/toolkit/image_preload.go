package toolkit

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// ImagePreloader streams release-lock-pinned images from the local verified
// prepare cache into the blank management node's containerd before the cluster
// stages run. It has no standalone entry point: the only caller is the
// bootstrap adapter, which supplies the Day-0 fence (machine id + permanent
// claim + epoch) so node writes stay owned and single-writer. Image content is
// re-hashed on the way out; a cache entry that no longer matches its locked
// digest aborts the preload before the stream is accepted and never blocks the
// online bootstrap from pulling directly from the registry.
type ImagePreloader struct {
	cacheDir string
	runner   NodeStreamReader
	observer func(PreloadProgress)
}

// PreloadProgress reports one real preload step per pinned image.
type PreloadProgress struct {
	Ref   string `json:"ref"`
	State string `json:"state"` // assembling | streaming | imported | failed
	Bytes int64  `json:"bytes,omitempty"`
	Error string `json:"error,omitempty"`
}

// preloadAuth carries the adapter's verified Day-0 identity. The claim is bound
// to the run id, config digest, lock digest and node machine id, so only the
// owning bootstrap run can pass the node-side fence.
type preloadAuth struct {
	machineID, claim string
	epoch            int64
	node             NodeConfig
}

func (p *ImagePreloader) emit(progress PreloadProgress) {
	if p.observer != nil {
		p.observer(progress)
	}
}

// PreloadImages imports every pinned image found in the verified cache and
// returns the number of images streamed to the node.
func (p *ImagePreloader) PreloadImages(ctx context.Context, lock *ReleaseLock, auth preloadAuth) (int, error) {
	if p.runner == nil {
		return 0, fmt.Errorf("BOOTSTRAP_PRELOAD_UNSUPPORTED: runner cannot stream")
	}
	if lock == nil || len(lock.Validate()) > 0 {
		return 0, fmt.Errorf("BOOTSTRAP_PRELOAD_INVALID: release lock required")
	}
	imported := 0
	for _, image := range lock.Images {
		if ctx.Err() != nil {
			return imported, ctx.Err()
		}
		if err := p.preloadOne(ctx, image, auth); err != nil {
			p.emit(PreloadProgress{Ref: image.Ref, State: "failed", Error: SafeReportText(err.Error())})
			return imported, fmt.Errorf("BOOTSTRAP_PRELOAD_FAILED: %s: %w", image.Ref, err)
		}
		imported++
	}
	return imported, nil
}

func (p *ImagePreloader) preloadOne(ctx context.Context, image PinnedImage, auth preloadAuth) error {
	manifestDigest, manifest, err := p.selectManifest(image)
	if err != nil {
		return err
	}
	p.emit(PreloadProgress{Ref: image.Ref, State: "assembling"})
	pr, pw := io.Pipe()
	var streamed atomic.Int64
	counter := &countingWriter{total: &streamed, next: pw}
	// An assembly failure must fail the stream, not send truncated bytes.
	go func() {
		pw.CloseWithError(p.assemble(manifest, manifestDigest, image.Ref, counter))
	}()
	p.emit(PreloadProgress{Ref: image.Ref, State: "streaming"})
	script := bootstrapGuard(PhasePreparing, auth.machineID, auth.claim, auth.epoch) + `
umask 077
mkdir -p /var/lib/hnb/preload
tmp=$(mktemp /var/lib/hnb/preload/image.XXXXXX)
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
ctr -n k8s.io images import --digests --all-platforms "$tmp"
`
	if _, err := p.runner.Stream(ctx, auth.node, script, pr); err != nil {
		return fmt.Errorf("node stream failed")
	}
	p.emit(PreloadProgress{Ref: image.Ref, State: "imported", Bytes: streamed.Load()})
	return nil
}

// selectManifest resolves the locked single linux/amd64 manifest from the
// verified cache, re-hashing it on every call.
func (p *ImagePreloader) selectManifest(image PinnedImage) (string, ociManifest, error) {
	data, err := p.verifiedBlob(image.Digest, -1)
	if err != nil {
		return "", ociManifest{}, err
	}
	var m ociManifest
	if json.Unmarshal(data, &m) != nil || m.SchemaVersion != 2 {
		return "", ociManifest{}, fmt.Errorf("invalid cached OCI manifest")
	}
	if len(m.Manifests) == 0 {
		if image.ManifestDigest != "" && image.ManifestDigest != image.Digest {
			return "", ociManifest{}, fmt.Errorf("single manifest identity differs from lock")
		}
		return image.Digest, m, nil
	}
	if image.Platform != "linux/amd64" || !digestPattern.MatchString(image.ManifestDigest) {
		return "", ociManifest{}, fmt.Errorf("index needs locked linux/amd64 manifest")
	}
	for _, desc := range m.Manifests {
		if desc.Platform != nil && desc.Platform.OS == "linux" && desc.Platform.Architecture == "amd64" && desc.Digest == image.ManifestDigest {
			data, err := p.verifiedBlob(desc.Digest, desc.Size)
			if err != nil {
				return "", ociManifest{}, err
			}
			if json.Unmarshal(data, &m) != nil || m.SchemaVersion != 2 || len(m.Manifests) > 0 {
				return "", ociManifest{}, fmt.Errorf("invalid cached selected manifest")
			}
			return image.ManifestDigest, m, nil
		}
	}
	return "", ociManifest{}, fmt.Errorf("locked amd64 manifest absent from cache")
}

// verifiedBlob reads one content-addressed cache entry and re-verifies digest
// and, when known, size before any use.
func (p *ImagePreloader) verifiedBlob(digest string, size int64) ([]byte, error) {
	path, err := cachedBlobPath(p.cacheDir, digest)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cached blob unavailable")
	}
	if size >= 0 && int64(len(data)) != size {
		return nil, fmt.Errorf("cached blob size differs from manifest")
	}
	if sum := fmt.Sprintf("sha256:%x", sha256.Sum256(data)); sum != digest {
		return nil, fmt.Errorf("cached blob checksum mismatch")
	}
	return data, nil
}

// assemble writes an OCI layout tar (oci-layout, index.json, manifest, config,
// layers) re-verifying every blob on the way out. Streaming keeps large layers
// out of memory.
func (p *ImagePreloader) assemble(manifest ociManifest, manifestDigest, ref string, w io.Writer) error {
	tw := tar.NewWriter(w)
	defer tw.Close()
	layout, _ := json.Marshal(struct {
		ImageLayoutVersion string `json:"imageLayoutVersion"`
	}{"1.0.0"})
	if err := writeTarBytes(tw, "oci-layout", layout); err != nil {
		return err
	}
	manifestData, err := p.verifiedBlob(manifestDigest, -1)
	if err != nil {
		return err
	}
	index, _ := json.Marshal(struct {
		SchemaVersion int              `json:"schemaVersion"`
		MediaType     string           `json:"mediaType"`
		Manifests     []ociLayoutEntry `json:"manifests"`
	}{2, "application/vnd.oci.image.index.v1+json", []ociLayoutEntry{{
		MediaType:   manifest.MediaType,
		Digest:      manifestDigest,
		Size:        int64(len(manifestData)),
		Annotations: map[string]string{"org.opencontainers.image.ref.name": ref},
	}}})
	if err := writeTarBytes(tw, "index.json", index); err != nil {
		return err
	}
	if err := p.tarBlob(tw, ociDescriptor{Digest: manifestDigest, Size: int64(len(manifestData))}, manifestData); err != nil {
		return err
	}
	for _, desc := range append([]ociDescriptor{manifest.Config}, manifest.Layers...) {
		if err := p.tarBlob(tw, desc, nil); err != nil {
			return err
		}
	}
	return tw.Close()
}

// tarBlob streams one cached blob into the layout tar, re-verifying digest and
// size while copying. A nonzero inline value (the manifest) skips the cache
// read; callers already verified it.
func (p *ImagePreloader) tarBlob(tw *tar.Writer, desc ociDescriptor, inline []byte) error {
	if desc.Digest == "" || !digestPattern.MatchString(desc.Digest) || desc.Size <= 0 {
		return fmt.Errorf("invalid OCI descriptor")
	}
	hex := strings.TrimPrefix(desc.Digest, "sha256:")
	var r io.Reader
	if inline != nil {
		if int64(len(inline)) != desc.Size {
			return fmt.Errorf("manifest size differs from descriptor")
		}
		r = bytes.NewReader(inline)
	} else {
		path, err := cachedBlobPath(p.cacheDir, desc.Digest)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("cached blob unavailable")
		}
		defer f.Close()
		r = f
	}
	if err := tw.WriteHeader(&tar.Header{Name: "blobs/sha256/" + hex, Mode: 0o644, Size: desc.Size}); err != nil {
		return err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tw, hash), r)
	if err != nil {
		return err
	}
	if written != desc.Size {
		return fmt.Errorf("cached blob size differs from manifest")
	}
	if sum := fmt.Sprintf("sha256:%x", hash.Sum(nil)); sum != desc.Digest {
		return fmt.Errorf("cached blob checksum mismatch")
	}
	return nil
}

func writeTarBytes(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func cachedBlobPath(dir, digest string) (string, error) {
	hex := strings.TrimPrefix(digest, "sha256:")
	if hex == digest || !digestPattern.MatchString(digest) {
		return "", fmt.Errorf("invalid content digest")
	}
	return filepath.Join(dir, "blobs", "sha256", hex), nil
}

type countingWriter struct {
	total *atomic.Int64
	next  io.Writer
}

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.next.Write(b)
	c.total.Add(int64(n))
	return n, err
}

type ociLayoutEntry struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

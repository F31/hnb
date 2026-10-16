package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PreparedArtifact struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	Path      string `json:"path"`
	FromCache bool   `json:"fromCache"`
}
type PrepareProgress struct {
	ObservedAt      time.Time `json:"observedAt"`
	Artifact        string    `json:"artifact"`
	Digest          string    `json:"digest"`
	Source          string    `json:"source"` // origin only; never query/credentials
	State           string    `json:"state"`  // downloading | retry | verified | cache-hit | failed
	Attempt         int       `json:"attempt"`
	BytesDownloaded int64     `json:"bytesDownloaded"`
	BytesVerified   int64     `json:"bytesVerified"`
	TotalBytes      int64     `json:"totalBytes"` // 0 means unknown
	Error           string    `json:"error,omitempty"`
}
type PrepareObserver interface{ Observe(PrepareProgress) }
type Downloader struct {
	Client      *http.Client
	MirrorBase  string // explicit HTTPS origin; same repository path, locked digests
	SourceDir   string // release package; local lock refs are basename-only
	Observer    PrepareObserver
	MaxBlobSize int64
	registry    *RegistryResolver
}
type PrepareResult struct {
	Status            string             `json:"status"` // verified | failed | canceled
	LockDigest        string             `json:"lockDigest,omitempty"`
	StartedAt         time.Time          `json:"startedAt"`
	FinishedAt        time.Time          `json:"finishedAt"`
	Error             string             `json:"error,omitempty"`
	Artifacts         []PreparedArtifact `json:"artifacts"`
	BytesVerified     int64              `json:"bytesVerified"`
	FromCache         int                `json:"fromCache"`
	InstallAuthorized bool               `json:"installAuthorized"`
}
type prepareRetryError struct{ reason string }

func (e prepareRetryError) Error() string { return e.reason }

func (d *Downloader) cachedPath(dir, digest string) (string, error) {
	if !digestPattern.MatchString(digest) || digest == placeholderDigest {
		return "", fmt.Errorf("invalid artifact digest")
	}
	return filepath.Join(dir, "blobs", "sha256", strings.TrimPrefix(digest, "sha256:")), nil
}
func (d *Downloader) sourceURL(image PinnedImage, suffix string) (string, error) {
	base, _, err := registryReference(image.Ref)
	if err != nil {
		return "", fmt.Errorf("invalid OCI source")
	}
	if d.MirrorBase != "" {
		if err := validateMirrorOrigin(d.MirrorBase); err != nil {
			return "", err
		}
		base = d.MirrorBase + "/v2/" + strings.SplitN(base, "/v2/", 2)[1]
	}
	return base + suffix, nil
}

func validateMirrorOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil || !validProbeURL(origin) || u.Path != "" {
		return fmt.Errorf("mirror must be a credential-free HTTPS origin")
	}
	return nil
}

func (d *Downloader) emit(event PrepareProgress) {
	event.ObservedAt = time.Now().UTC()
	event.Artifact = SafeReportText(event.Artifact)
	event.Source = SafeReportText(event.Source)
	event.Error = SafeReportText(event.Error)
	if d.Observer != nil {
		d.Observer.Observe(event)
	}
}

// fetch always rehashes cache hits. Unique temporary files prevent concurrent
// writers from deleting each other's partial downloads. Only verified files
// are atomically published; interrupted transfers restart, never append bytes.
func (d *Downloader) fetch(ctx context.Context, dir, kind, name, source, digest string, expected int64) (PreparedArtifact, error) {
	a := PreparedArtifact{Kind: kind, Name: name, Digest: digest}
	path, err := d.cachedPath(dir, digest)
	if err != nil {
		return a, err
	}
	a.Path = path
	if err := ctx.Err(); err != nil {
		return a, fmt.Errorf("preparation canceled")
	}
	limit := d.MaxBlobSize
	if limit <= 0 {
		limit = 1 << 30
	}
	if kind == "image-manifest" || kind == "image-index" || kind == "image-config" {
		limit = 1 << 20
	}
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() || fi.Size() > limit || (expected > 0 && fi.Size() != expected) {
			return a, fmt.Errorf("cache entry invalid; remove it explicitly before retry")
		}
		actual, err := hashSource(path)
		if err != nil || actual != digest {
			return a, fmt.Errorf("cache checksum mismatch; remove it explicitly before retry")
		}
		a.Size, a.FromCache = fi.Size(), true
		d.emit(PrepareProgress{Artifact: name, Digest: digest, State: "cache-hit", BytesVerified: a.Size, TotalBytes: a.Size})
		return a, nil
	} else if !os.IsNotExist(err) {
		return a, fmt.Errorf("cache entry unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return a, fmt.Errorf("cannot create cache directory")
	}
	for attempt := 1; attempt <= 3; attempt++ {
		a, err = d.fetchAttempt(ctx, a, source, expected, attempt)
		if err == nil {
			return a, nil
		}
		if _, ok := err.(prepareRetryError); !ok || attempt == 3 {
			return a, err
		}
		timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return a, fmt.Errorf("preparation canceled")
		case <-timer.C:
		}
	}
	return a, err
}

func (d *Downloader) fetchAttempt(ctx context.Context, a PreparedArtifact, source string, expected int64, attempt int) (out PreparedArtifact, err error) {
	var body io.ReadCloser
	var total int64
	origin := "release-package"
	event := PrepareProgress{Artifact: a.Name, Digest: a.Digest, Source: origin, Attempt: attempt}
	defer func() {
		if err != nil {
			event.State = "failed"
			if _, retryable := err.(prepareRetryError); retryable && attempt < 3 {
				event.State = "retry"
			}
			event.Error = err.Error()
			d.emit(event)
		}
	}()
	if strings.HasPrefix(source, "https://") {
		if !validProbeURL(source) {
			return a, fmt.Errorf("invalid source URL")
		}
		u, _ := url.Parse(source)
		origin = u.Scheme + "://" + u.Host
		event.Source = origin
		var resp *http.Response
		var err error
		if d.registry != nil {
			if strings.Contains(u.Path, "/v2/") {
				token := ""
				resp, err = d.registry.open(ctx, source, &token)
			} else {
				resp, err = d.registry.get(ctx, source, "")
			}
		} else {
			return a, fmt.Errorf("release source policy unavailable")
		}
		if err != nil {
			if ctx.Err() != nil {
				return a, fmt.Errorf("preparation canceled")
			}
			return a, prepareRetryError{"source request failed"}
		}
		if resp.Request != nil && resp.Request.URL != nil {
			event.Source = resp.Request.URL.Scheme + "://" + resp.Request.URL.Host
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
				return a, prepareRetryError{"source temporarily unavailable"}
			}
			return a, fmt.Errorf("source returned HTTP %d", resp.StatusCode)
		}
		if header := resp.Header.Get("Docker-Content-Digest"); header != "" && header != a.Digest {
			resp.Body.Close()
			return a, fmt.Errorf("registry digest header differs from locked identity")
		}
		body, total = resp.Body, resp.ContentLength
	} else {
		if d.SourceDir == "" || source == "" || filepath.Base(source) != source || source == "." || source == ".." || strings.ContainsAny(source, "\\:\r\n\x00") {
			return a, fmt.Errorf("local artifact requires a release-package basename")
		}
		path := filepath.Join(d.SourceDir, source)
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() {
			return a, fmt.Errorf("local release artifact unavailable or not regular")
		}
		file, err := os.Open(path)
		if err != nil {
			return a, fmt.Errorf("local release artifact unreadable")
		}
		body, total = file, fi.Size()
	}
	defer body.Close()
	if expected > 0 {
		total = expected
	}
	if total < 0 {
		total = 0
	}
	limit := d.MaxBlobSize
	if limit <= 0 {
		limit = 1 << 30
	}
	if a.Kind == "image-manifest" || a.Kind == "image-index" || a.Kind == "image-config" {
		limit = 1 << 20
	}
	if total > limit {
		return a, fmt.Errorf("artifact exceeds preparation size ceiling")
	}
	file, err := os.CreateTemp(filepath.Dir(a.Path), ".part-")
	if err != nil {
		return a, fmt.Errorf("cannot create private cache temporary file")
	}
	defer func() { file.Close(); os.Remove(file.Name()) }()
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	event.State, event.TotalBytes = "downloading", total
	d.emit(event)
	for {
		if ctx.Err() != nil {
			return a, fmt.Errorf("preparation canceled")
		}
		n, readErr := body.Read(buffer)
		if n > 0 {
			event.BytesDownloaded += int64(n)
			if event.BytesDownloaded > limit {
				return a, fmt.Errorf("artifact exceeds preparation size ceiling")
			}
			if _, err := file.Write(buffer[:n]); err != nil {
				return a, fmt.Errorf("cache write failed")
			}
			hash.Write(buffer[:n])
			d.emit(event)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return a, fmt.Errorf("preparation canceled")
			}
			return a, prepareRetryError{"content transfer interrupted"}
		}
	}
	if expected > 0 && event.BytesDownloaded != expected {
		return a, prepareRetryError{"content size differs from locked descriptor"}
	}
	if fmt.Sprintf("sha256:%x", hash.Sum(nil)) != a.Digest {
		return a, fmt.Errorf("content checksum differs from release lock")
	}
	if err := file.Sync(); err != nil {
		return a, fmt.Errorf("cache sync failed")
	}
	if err := file.Close(); err != nil {
		return a, fmt.Errorf("cache close failed")
	}
	if err := os.Rename(file.Name(), a.Path); err != nil {
		return a, fmt.Errorf("cache publish failed")
	}
	a.Size = event.BytesDownloaded
	event.State = "verified"
	event.BytesVerified = a.Size
	d.emit(event)
	return a, nil
}

func (d *Downloader) PrepareLock(ctx context.Context, lock *ReleaseLock, dir string) (r PrepareResult, err error) {
	r = PrepareResult{Artifacts: []PreparedArtifact{}, StartedAt: time.Now().UTC()}
	defer func() {
		r.FinishedAt = time.Now().UTC()
		r.Status = "verified"
		if err != nil {
			r.Status = "failed"
			r.Error = SafeReportText(err.Error())
			if ctx.Err() != nil {
				r.Status = "canceled"
			}
		}
	}()
	if lock == nil {
		return r, fmt.Errorf("missing release lock")
	}
	if problems := lock.Validate(); len(problems) > 0 {
		return r, fmt.Errorf("invalid release lock")
	}
	r.LockDigest = lockContentDigest(lock)
	if d.MirrorBase != "" {
		if err := validateMirrorOrigin(d.MirrorBase); err != nil {
			return r, err
		}
	}
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	registry := NewRegistryResolver(client)
	if err := registry.DeclareRedirectOrigins(lock.SourceRedirectOrigins); err != nil {
		return r, err
	}
	d.registry = &registry
	if err := preparePrivateDir(dir); err != nil {
		return r, err
	}
	add := func(kind, name, source, digest string, size int64) ([]byte, error) {
		a, err := d.fetch(ctx, dir, kind, name, source, digest, size)
		if err != nil {
			return nil, err
		}
		r.Artifacts = append(r.Artifacts, a)
		r.BytesVerified += a.Size
		if a.FromCache {
			r.FromCache++
		}
		if kind == "image-manifest" || kind == "image-index" || kind == "image-config" {
			data, err := os.ReadFile(a.Path)
			return data, err
		}
		return nil, nil
	}
	images := append([]PinnedImage{}, lock.Images...)
	for _, a := range lock.NodeArtifacts {
		if a.Kind == "container" {
			images = append(images, PinnedImage{Name: "node-image", Ref: a.Ref, Digest: a.Digest})
		}
	}
	for _, image := range images {
		source, err := d.sourceURL(image, "/manifests/"+image.Digest)
		if err != nil {
			return r, err
		}
		data, err := add("image-manifest", image.Name, source, image.Digest, 0)
		if err != nil {
			return r, err
		}
		var m ociManifest
		if json.Unmarshal(data, &m) != nil || m.SchemaVersion != 2 {
			return r, fmt.Errorf("invalid OCI manifest")
		}
		if len(m.Manifests) > 0 {
			if image.Platform != "linux/amd64" || !digestPattern.MatchString(image.ManifestDigest) {
				return r, fmt.Errorf("index needs locked linux/amd64 manifest")
			}
			var selected []ociDescriptor
			for _, desc := range m.Manifests {
				if p := desc.Platform; p != nil && p.OS == "linux" && p.Architecture == "amd64" && p.Variant == "" {
					selected = append(selected, desc)
				}
			}
			if len(selected) != 1 || selected[0].Digest != image.ManifestDigest || selected[0].Size <= 0 {
				return r, fmt.Errorf("index platform differs from release lock")
			}
			source, err = d.sourceURL(image, "/manifests/"+image.ManifestDigest)
			if err != nil {
				return r, err
			}
			data, err = add("image-manifest", image.Name, source, image.ManifestDigest, selected[0].Size)
			if err != nil {
				return r, err
			}
			m = ociManifest{}
			if json.Unmarshal(data, &m) != nil || m.SchemaVersion != 2 || len(m.Manifests) > 0 {
				return r, fmt.Errorf("invalid selected manifest")
			}
		} else if image.ManifestDigest != "" && image.ManifestDigest != image.Digest {
			return r, fmt.Errorf("single manifest identity differs from lock")
		}
		if m.MediaType != "application/vnd.oci.image.manifest.v1+json" && m.MediaType != "application/vnd.docker.distribution.manifest.v2+json" {
			return r, fmt.Errorf("unsupported OCI manifest type")
		}
		if len(m.Layers) > 128 {
			return r, fmt.Errorf("OCI descriptor budget exceeded")
		}
		for i, desc := range append([]ociDescriptor{m.Config}, m.Layers...) {
			if desc.Size <= 0 {
				return r, fmt.Errorf("invalid OCI descriptor size")
			}
			source, err = d.sourceURL(image, "/blobs/"+desc.Digest)
			if err != nil {
				return r, err
			}
			kind := "image-layer"
			if i == 0 {
				kind = "image-config"
			}
			data, err = add(kind, image.Name, source, desc.Digest, desc.Size)
			if err != nil {
				return r, err
			}
			if i == 0 && image.Platform != "" {
				var p ociPlatform
				if json.Unmarshal(data, &p) != nil || p.OS+"/"+p.Architecture != image.Platform {
					return r, fmt.Errorf("image config platform differs from lock")
				}
			}
		}
	}
	for _, c := range lock.Charts {
		if strings.HasPrefix(c.Ref, "oci://") {
			if err := d.prepareOCIChart(ctx, dir, c, &r); err != nil {
				return r, err
			}
			continue
		}
		if _, err := add("chart", c.Name, c.Ref, c.Digest, 0); err != nil {
			return r, err
		}
	}
	for i, c := range lock.CLI {
		if _, err := add("cli", fmt.Sprintf("cli-%d", i), c.Ref, c.Digest, 0); err != nil {
			return r, err
		}
	}
	for i, a := range lock.NodeArtifacts {
		if a.Kind != "container" {
			digest, _ := probeDigest(a.Digest)
			if _, err := add("node-artifact", fmt.Sprintf("node-artifact-%d", i), a.Ref, digest, 0); err != nil {
				return r, err
			}
			if a.Kind == "package-index" {
				if err := d.prepareIndexedPackages(ctx, dir, a, &r); err != nil {
					return r, err
				}
			}
		}
	}
	for _, m := range lock.Migrations {
		if _, err := add("migration", m.Owner+"/"+m.Name, m.Name, "sha256:"+m.Checksum, 0); err != nil {
			return r, err
		}
	}
	return r, nil
}

// Cache directories are caller-owned/private. Do not traverse a supplied
// symlink or chmod an unrelated existing directory to make it acceptable.
func preparePrivateDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil || abs == string(filepath.Separator) {
		return fmt.Errorf("invalid private cache directory")
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		fi, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0700); err != nil && !os.IsExist(err) {
				return fmt.Errorf("cannot create private cache directory")
			}
			fi, err = os.Lstat(current)
		}
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("cache path must contain regular directories, not symlinks")
		}
		if current == abs && fi.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("cache directory must be private (0700)")
		}
	}
	for _, part := range []string{"blobs", "sha256"} {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0700); err != nil && !os.IsExist(err) {
			return fmt.Errorf("cannot create cache content directory")
		}
		fi, err := os.Lstat(current)
		if err != nil || !fi.IsDir() || fi.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("invalid private cache content directory")
		}
	}
	return nil
}

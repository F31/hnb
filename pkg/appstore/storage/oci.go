package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

const (
	emptyConfigDigest = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	emptyConfigBody   = "{}"
)

var (
	sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	ErrManifestNotFound = errors.New("OCI manifest not found")
	ErrDigestMismatch   = errors.New("OCI manifest digest mismatch")
)

type OCIStorage struct {
	config StorageConfig
	client *http.Client
}

func NewOCIStorage(config StorageConfig) *OCIStorage {
	return &OCIStorage{
		config: config,
		client: &http.Client{},
	}
}

func (s *OCIStorage) url(paths ...string) string {
	return fmt.Sprintf("%s/v2/%s", s.config.RegistryURL, strings.Join(paths, "/"))
}

func (s *OCIStorage) do(req *http.Request) (*http.Response, error) {
	if s.config.Username != "" {
		req.SetBasicAuth(s.config.Username, s.config.Password)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	if s.config.Insecure {
		req.URL.Scheme = "http"
	}
	return s.client.Do(req)
}

// drainAndClose reads the remaining response body so the underlying
// connection can be reused by the transport pool, then closes it. Errors are
// best-effort and intentionally not propagated.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	_ = resp.Body.Close()
}

func (s *OCIStorage) Download(ctx context.Context, registryURL string) (io.ReadCloser, *ArtifactMeta, error) {
	ref := strings.TrimPrefix(registryURL, s.config.RegistryURL+"/")
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid ref: %s", registryURL)
	}
	repoName := parts[0]
	tag := parts[1]

	manifest, desc, err := s.pullManifest(ctx, repoName, tag)
	if err != nil {
		return nil, nil, fmt.Errorf("pull manifest: %w", err)
	}

	digest := manifest.Layers[0].Digest
	url := s.url(repoName, "blobs", digest)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := s.do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("download blob: %w", err)
	}

	meta := &ArtifactMeta{
		Digest:      digest,
		SizeBytes:   desc.Size,
		RegistryURL: registryURL,
	}

	return resp.Body, meta, nil
}

func (s *OCIStorage) Delete(ctx context.Context, registryURL string) error {
	ref := strings.TrimPrefix(registryURL, s.config.RegistryURL+"/")
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid ref: %s", registryURL)
	}
	repoName := parts[0]
	tag := parts[1]

	manifest, _, err := s.pullManifest(ctx, repoName, tag)
	if err != nil {
		return fmt.Errorf("pull manifest: %w", err)
	}

	// Delete manifest
	mURL := s.url(repoName, "manifests", tag)
	mReq, _ := http.NewRequestWithContext(ctx, "DELETE", mURL, nil)
	mResp, err := s.do(mReq)
	if err != nil {
		return fmt.Errorf("delete manifest: %w", err)
	}
	drainAndClose(mResp)

	// Delete blob
	bURL := s.url(repoName, "blobs", manifest.Layers[0].Digest)
	bReq, _ := http.NewRequestWithContext(ctx, "DELETE", bURL, nil)
	bResp, err := s.do(bReq)
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	drainAndClose(bResp)

	log.Printf("[oci] deleted %s", registryURL)
	return nil
}

func (s *OCIStorage) ListTags(ctx context.Context, repoName string) ([]string, error) {
	url := s.url(repoName, "tags", "list")
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := s.do(req)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp)

	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tags, nil
}

func IsSHA256Digest(digest string) bool {
	return sha256DigestPattern.MatchString(digest)
}

// VerifyManifest confirms that Harbor serves repository content by the claimed
// digest and returns metadata derived from the authoritative manifest.
func (s *OCIStorage) VerifyManifest(ctx context.Context, repository, digest string) (*ArtifactMeta, error) {
	if !IsSHA256Digest(digest) {
		return nil, fmt.Errorf("invalid SHA-256 digest")
	}
	url := s.url(repository, "manifests", digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.do(req)
	if err != nil {
		return nil, fmt.Errorf("verify manifest: %w", err)
	}
	drainAndClose(resp)
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrManifestNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("verify manifest: registry status %d", resp.StatusCode)
	}
	actualDigest := resp.Header.Get("Docker-Content-Digest")
	if actualDigest == "" || actualDigest != digest {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, digest, actualDigest)
	}

	manifest, _, err := s.pullManifest(ctx, repository, digest)
	if err != nil {
		return nil, fmt.Errorf("read verified manifest: %w", err)
	}
	var size int64
	for _, layer := range manifest.Layers {
		size += layer.Size
	}
	mediaType := manifest.MediaType
	if mediaType == "" {
		mediaType = resp.Header.Get("Content-Type")
	}
	return &ArtifactMeta{
		MediaType:   mediaType,
		Digest:      digest,
		SizeBytes:   size,
		RegistryURL: fmt.Sprintf("%s/%s@%s", s.config.RegistryURL, repository, digest),
	}, nil
}

func (s *OCIStorage) pullManifest(ctx context.Context, repoName, tag string) (*ociManifest, *ociDescriptor, error) {
	url := s.url(repoName, "manifests", tag)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := s.do(req)
	if err != nil {
		return nil, nil, err
	}
	defer drainAndClose(resp)
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, ErrManifestNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("registry status %d", resp.StatusCode)
	}

	digest := resp.Header.Get("Docker-Content-Digest")

	var manifest ociManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, nil, err
	}

	desc := &ociDescriptor{
		Digest: digest,
		Size:   resp.ContentLength,
	}

	return &manifest, desc, nil
}

func (s *OCIStorage) ResolveRepository(filename string) string {
	ext := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(ext, ".jar"):
		return "hnb/jars"
	case strings.HasSuffix(ext, ".war"):
		return "hnb/wars"
	case strings.HasSuffix(ext, ".tar.gz") || strings.HasSuffix(ext, ".tgz"):
		return "hnb/charts"
	case strings.HasSuffix(ext, ".zip"):
		return "hnb/zips"
	default:
		return "hnb/generic"
	}
}

type ociManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// Push uploads a generic artifact as an OCI artifact to the registry.
// All blobs are streamed to the registry as single PUT requests with the
// computed digest returned for use in the manifest. Returns the resulting
// registry URL.
func (s *OCIStorage) Push(ctx context.Context, repository, tag string, content io.ReadSeeker, contentSize int64, configAnnotations map[string]string) (string, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	layerDigest, err := s.uploadBlob(ctx, repository, content, contentSize)
	if err != nil {
		return "", fmt.Errorf("upload layer blob: %w", err)
	}

	configDigest, err := s.uploadBlob(ctx, repository, bytes.NewReader([]byte(emptyConfigBody)), int64(len(emptyConfigBody)))
	if err != nil {
		return "", fmt.Errorf("upload config blob: %w", err)
	}

	manifest := ociManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    configDigest,
			Size:      int64(len(emptyConfigBody)),
		},
		Layers: []ociDescriptor{{
			MediaType: "application/octet-stream",
			Digest:    layerDigest,
			Size:      contentSize,
		}},
		Annotations: configAnnotations,
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}

	mURL := s.url(repository, "manifests", tag)
	mReq, err := http.NewRequestWithContext(ctx, http.MethodPut, mURL, bytes.NewReader(manifestBody))
	if err != nil {
		return "", err
	}
	mReq.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	mResp, err := s.do(mReq)
	if err != nil {
		return "", fmt.Errorf("put manifest: %w", err)
	}
	defer drainAndClose(mResp)
	if mResp.StatusCode < 200 || mResp.StatusCode >= 300 {
		body, _ := io.ReadAll(mResp.Body)
		return "", fmt.Errorf("put manifest: registry status %d: %s", mResp.StatusCode, body)
	}

	manifestDigest := mResp.Header.Get("Docker-Content-Digest")
	if manifestDigest == "" {
		manifestDigest = layerDigest
	}

	registryURL := fmt.Sprintf("%s/%s@%s", s.config.RegistryURL, repository, manifestDigest)
	return registryURL, nil
}

// chunkSize is the buffer size used for streaming blob uploads.
const chunkSize = 16 * 1024 * 1024 // 16 MiB

// uploadBlob uploads content to the registry using chunked PATCH followed by
// a PUT ?digest=<sha256> finalize. The sha256 is computed while streaming so
// the file is only read once.
func (s *OCIStorage) uploadBlob(ctx context.Context, repository string, content io.Reader, size int64) (string, error) {
	initURL := s.url(repository, "blobs", "uploads") + "/"
	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, nil)
	if err != nil {
		return "", err
	}
	initResp, err := s.do(initReq)
	if err != nil {
		return "", err
	}
	drainAndClose(initResp)
	if initResp.StatusCode < 200 || initResp.StatusCode >= 300 {
		return "", fmt.Errorf("init blob upload: status %d", initResp.StatusCode)
	}

	uploadURL := initResp.Header.Get("Location")
	if uploadURL == "" {
		return "", fmt.Errorf("init blob upload: missing Location")
	}
	if !strings.HasPrefix(uploadURL, "http") {
		uploadURL = s.config.RegistryURL + uploadURL
	}

	pr, pw := io.Pipe()
	hasher := sha256.New()
	mw := io.MultiWriter(pw, hasher)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer pw.Close()
		buf := make([]byte, chunkSize)
		for {
			n, rerr := io.ReadFull(content, buf)
			if n > 0 {
				if _, werr := mw.Write(buf[:n]); werr != nil {
					_ = pw.CloseWithError(werr)
					return
				}
			}
			if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
				return
			}
			if rerr != nil {
				_ = pw.CloseWithError(rerr)
				return
			}
		}
	}()

	patchReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, uploadURL, pr)
	if err != nil {
		return "", err
	}
	patchReq.Header.Set("Content-Type", "application/octet-stream")
	patchReq.ContentLength = -1
	patchReq.TransferEncoding = []string{"chunked"}
	patchResp, err := s.do(patchReq)
	if err != nil {
		return "", fmt.Errorf("patch blob: %w", err)
	}
	<-writerDone
	patchRespBody, _ := io.ReadAll(patchResp.Body)
	drainAndClose(patchResp)
	if patchResp.StatusCode < 200 || patchResp.StatusCode >= 300 {
		return "", fmt.Errorf("patch blob: status %d: %s", patchResp.StatusCode, patchRespBody)
	}

	nextLocation := patchResp.Header.Get("Location")
	if nextLocation != "" {
		if !strings.HasPrefix(nextLocation, "http") {
			nextLocation = s.config.RegistryURL + nextLocation
		}
		uploadURL = nextLocation
	}

	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	sep := "&"
	if !strings.Contains(uploadURL, "?") {
		sep = "?"
	}
	finalURL := uploadURL + sep + "digest=" + digest
	finalReq, err := http.NewRequestWithContext(ctx, http.MethodPut, finalURL, nil)
	if err != nil {
		return "", err
	}
	finalReq.Header.Set("Content-Length", "0")
	finalResp, err := s.do(finalReq)
	if err != nil {
		return "", fmt.Errorf("finalize blob: %w", err)
	}
	defer drainAndClose(finalResp)
	finalBody, _ := io.ReadAll(finalResp.Body)
	if finalResp.StatusCode < 200 || finalResp.StatusCode >= 300 {
		return "", fmt.Errorf("finalize blob: status %d: %s", finalResp.StatusCode, finalBody)
	}
	return digest, nil
}

func digestReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// noCloseSeeker wraps an io.ReadSeeker so the http client does not close the
// underlying reader (e.g. *os.File), allowing the caller to reuse it across
// multiple requests.
type noCloseSeeker struct {
	r io.ReadSeeker
}

func (n noCloseSeeker) Read(p []byte) (int, error)         { return n.r.Read(p) }
func (n noCloseSeeker) Seek(o int64, w int) (int64, error) { return n.r.Seek(o, w) }
func (n noCloseSeeker) Close() error                       { return nil }

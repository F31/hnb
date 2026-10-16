package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func ioBody(s string) io.ReadCloser                                       { return io.NopCloser(strings.NewReader(s)) }

func TestPrepareBearerAndDeclaredCDNStreaming(t *testing.T) {
	lock, paths := prepareFixture(t, true)
	var leaked atomic.Bool
	var cdnCalls, tokenCalls atomic.Int32
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
		w.Write(paths[r.URL.Path])
	}))
	defer cdn.Close()
	var registry *httptest.Server
	registry = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenCalls.Add(1)
			if r.Header.Get("Authorization") != "" || r.URL.Query().Get("scope") != "repository:project/image:pull" {
				leaked.Store(true)
			}
			w.Write([]byte(`{"token":"opaque-auth-token"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer opaque-auth-token" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+registry.URL+`/token",service="fixture",scope="repository:project/image:pull"`)
			w.WriteHeader(401)
			return
		}
		if strings.Contains(r.URL.Path, "/blobs/") {
			http.Redirect(w, r, cdn.URL+r.URL.Path+"?signature=must-not-log", 307)
			return
		}
		w.Write(paths[r.URL.Path])
	}))
	defer registry.Close()
	lock.SourceRedirectOrigins = []string{cdn.URL}
	events := &prepareEvents{}
	d := Downloader{Client: registry.Client(), MirrorBase: registry.URL, Observer: events}
	result, err := d.PrepareLock(context.Background(), lock, prepareTestCache(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 4 || cdnCalls.Load() != 2 || tokenCalls.Load() == 0 || leaked.Load() {
		t.Fatalf("bad token/CDN behavior: %+v", result)
	}
	data, _ := json.Marshal(events.events)
	if strings.Contains(string(data), "opaque-auth-token") || strings.Contains(string(data), "must-not-log") {
		t.Fatal("token or signed query leaked into progress")
	}
	actualCDN := false
	for _, e := range events.events {
		if e.Source == cdn.URL {
			actualCDN = true
		}
	}
	if !actualCDN {
		t.Fatal("progress does not state actual redirected source")
	}
}

func TestPrepareRetryReportsActualCDNOriginAndReason(t *testing.T) {
	lock, paths := prepareFixture(t, false)
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer cdn.Close()
	registry := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/") {
			http.Redirect(w, r, cdn.URL+r.URL.Path+"?signature=must-not-log", http.StatusTemporaryRedirect)
			return
		}
		w.Write(paths[r.URL.Path])
	}))
	defer registry.Close()
	lock.SourceRedirectOrigins = []string{cdn.URL}
	events := &prepareEvents{}
	d := Downloader{Client: registry.Client(), MirrorBase: registry.URL, Observer: events}
	result, err := d.PrepareLock(context.Background(), lock, prepareTestCache(t))
	if err == nil || result.Status != "failed" || len(result.Artifacts) != 1 {
		t.Fatalf("failed CDN transfer lost verified manifest: %+v %v", result, err)
	}
	var retries, failures int
	for _, event := range events.events {
		if event.State != "retry" && event.State != "failed" {
			continue
		}
		if event.Source != cdn.URL || event.Error != "source temporarily unavailable" || event.ObservedAt.IsZero() {
			t.Fatalf("failure reported original registry or lost reason: %+v", event)
		}
		if event.State == "retry" {
			retries++
		} else {
			failures++
		}
	}
	if retries != 2 || failures != 1 {
		t.Fatalf("retry budget not reflected in progress: retries=%d failures=%d", retries, failures)
	}
	data, _ := json.Marshal(events.events)
	if strings.Contains(string(data), "must-not-log") {
		t.Fatal("retry progress leaked signed query")
	}
}

func TestRegistryTokenNeverCrossesSchemeOnSameHost(t *testing.T) {
	resolver := NewRegistryResolver(nil)
	// Deterministic transport keeps both origins on the same host, without TLS
	// bypass or an actual plaintext network connection.
	var leaked bool
	resolver.probe.client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme == "http" {
			leaked = r.Header.Get("Authorization") != ""
			return &http.Response{StatusCode: 200, Body: ioBody("fixture"), Header: make(http.Header), Request: r}, nil
		}
		return &http.Response{StatusCode: 307, Body: ioBody(""), Header: http.Header{"Location": []string{"http://registry.example.invalid/blob"}}, Request: r}, nil
	})}
	if err := resolver.DeclareRedirectOrigins([]string{"http://registry.example.invalid"}); err != nil {
		t.Fatal(err)
	}
	response, err := resolver.get(context.Background(), "https://registry.example.invalid/blob", "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if leaked {
		t.Fatal("same-host HTTP redirect received HTTPS registry token")
	}
}

func chartPackageFixture(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	archive := tar.NewWriter(gz)
	data := []byte("apiVersion: v2\nname: fixture-chart\nversion: 1.0.0\n")
	if err := archive.WriteHeader(&tar.Header{Name: "fixture-chart/Chart.yaml", Size: int64(len(data)), Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestPrepareOCIChartPinsPackageAndWarmCache(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(fmt.Sprint(wrong), func(t *testing.T) {
			lock, paths := prepareFixture(t, false)
			pkg := chartPackageFixture(t)
			digest := bodyDigest(string(pkg))
			layerDigest := digest
			if wrong {
				layerDigest = bodyDigest("wrong package")
			}
			manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.cncf.helm.config.v1+json"},"layers":[{"mediaType":"application/vnd.cncf.helm.chart.content.v1.tar+gzip","digest":%q,"size":%d}]}`, layerDigest, len(pkg))
			paths["/v2/project/chart/manifests/1.0.0"] = []byte(manifest)
			paths["/v2/project/chart/blobs/"+digest] = pkg
			var calls, blobCalls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if strings.Contains(r.URL.Path, "/project/chart/blobs/") {
					blobCalls.Add(1)
				}
				data, ok := paths[r.URL.Path]
				if !ok {
					w.WriteHeader(404)
					return
				}
				w.Write(data)
			}))
			defer server.Close()
			lock.Charts = []PinnedChart{{Name: "fixture-chart", Version: "1.0.0", Ref: "oci://original.invalid/project/chart", Digest: digest}}
			d := Downloader{Client: server.Client(), MirrorBase: server.URL}
			cache := prepareTestCache(t)
			r, err := d.PrepareLock(context.Background(), lock, cache)
			if wrong {
				if err == nil || blobCalls.Load() != 0 {
					t.Fatal("OCI tag changed package accepted")
				}
				return
			}
			if err != nil || len(r.Artifacts) != 4 {
				t.Fatalf("Chart preparation failed: %+v %v", r, err)
			}
			before := calls.Load()
			r, err = d.PrepareLock(context.Background(), lock, cache)
			if err != nil || r.FromCache != 4 || calls.Load() != before {
				t.Fatal("warm OCI Chart contacted a mutable tag")
			}
		})
	}
}

func TestPrepareRealRegistryClosure(t *testing.T) {
	ref := os.Getenv("HNB_PREPARE_E2E_REF")
	if ref == "" {
		t.Skip("set HNB_PREPARE_E2E_REF for explicit real registry download")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response, err := http.DefaultTransport.RoundTrip(request)
		if response != nil {
			t.Logf("source=%s://%s status=%d", request.URL.Scheme, request.URL.Host, response.StatusCode)
			if location, parseErr := url.Parse(response.Header.Get("Location")); parseErr == nil && location.Host != "" {
				t.Logf("redirectOrigin=%s://%s (explicit release source policy required)", location.Scheme, location.Host)
			}
		}
		return response, err
	})}
	resolver := NewRegistryResolver(client)
	origins := os.Getenv("HNB_PREPARE_E2E_REDIRECT_ORIGINS")
	if origins != "" {
		if err := resolver.DeclareRedirectOrigins(strings.Split(origins, ",")); err != nil {
			t.Fatal(err)
		}
	}
	image, err := resolver.Resolve(ctx, "real-registry-observation", ref)
	if err != nil {
		t.Fatal(err)
	}
	lock := &ReleaseLock{SchemaVersion: LockSchemaVersion, ReleaseID: "real-image-only", ReleaseVersion: "observation", Images: []PinnedImage{image}, Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}
	for origin := range resolver.ObservedRedirect {
		lock.SourceRedirectOrigins = append(lock.SourceRedirectOrigins, origin)
	}
	d := Downloader{Client: client}
	cache := prepareTestCache(t)
	r, err := d.PrepareLock(ctx, lock, cache)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("root=%s manifest=%s artifacts=%d verifiedBytes=%d coldCacheHits=%d", image.Digest, image.ManifestDigest, len(r.Artifacts), r.BytesVerified, r.FromCache)
	warm, err := d.PrepareLock(ctx, lock, cache)
	if err != nil || warm.FromCache != len(warm.Artifacts) {
		t.Fatalf("warm rehash failed: %+v %v", warm, err)
	}
	t.Logf("warmCacheHits=%d; image bytes only, no node import or platform certification", warm.FromCache)
}

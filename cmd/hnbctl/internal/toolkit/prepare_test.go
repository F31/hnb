package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type prepareEvents struct{ events []PrepareProgress }

func prepareTestCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func (p *prepareEvents) Observe(e PrepareProgress) { p.events = append(p.events, e) }

func prepareFixture(t *testing.T, index bool) (*ReleaseLock, map[string][]byte) {
	t.Helper()
	config := []byte(`{"os":"linux","architecture":"amd64"}`)
	layer := []byte(strings.Repeat("layer-contents", 20000))
	desc := func(b []byte) ociDescriptor { return ociDescriptor{Digest: bodyDigest(string(b)), Size: int64(len(b))} }
	m := ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: desc(config), Layers: []ociDescriptor{desc(layer)}}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	// Real single manifests omit index fields rather than setting them to null.
	var leaf map[string]any
	if err := json.Unmarshal(manifest, &leaf); err != nil {
		t.Fatal(err)
	}
	delete(leaf, "manifests")
	manifest, err = json.Marshal(leaf)
	if err != nil {
		t.Fatal(err)
	}
	image := PinnedImage{Name: "fixture", Ref: "origin.invalid/project/image:1.0", Digest: bodyDigest(string(manifest)), ManifestDigest: bodyDigest(string(manifest)), Platform: "linux/amd64"}
	paths := map[string][]byte{"/v2/project/image/manifests/" + image.Digest: manifest, "/v2/project/image/blobs/" + desc(config).Digest: config, "/v2/project/image/blobs/" + desc(layer).Digest: layer}
	if index {
		descriptor := desc(manifest)
		descriptor.Platform = &ociPlatform{OS: "linux", Architecture: "amd64"}
		root, err := json.Marshal(ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json", Manifests: []ociDescriptor{descriptor}})
		if err != nil {
			t.Fatal(err)
		}
		image.Digest = bodyDigest(string(root))
		paths["/v2/project/image/manifests/"+image.Digest] = root
	}
	return &ReleaseLock{SchemaVersion: LockSchemaVersion, ReleaseID: "fixture", ReleaseVersion: "1.0", Images: []PinnedImage{image}, Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}, paths
}

func TestPreparePinnedIndexMirrorLocalPackageAndCache(t *testing.T) {
	lock, paths := prepareFixture(t, true)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		data, ok := paths[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Write(data)
	}))
	defer server.Close()
	sourceDir, cache := t.TempDir(), prepareTestCache(t)
	for _, name := range []string{"hnbctl", "hnb.tgz", "001.sql"} {
		data := []byte("local-" + name)
		if err := os.WriteFile(filepath.Join(sourceDir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	lock.CLI = []PinnedCLI{{OS: "linux", Arch: "amd64", Ref: "hnbctl", Digest: bodyDigest("local-hnbctl")}}
	lock.Charts = []PinnedChart{{Name: "hnb", Version: "1.0", Ref: "hnb.tgz", Digest: bodyDigest("local-hnb.tgz")}}
	lock.Migrations = []Migration{{Owner: "platform", Name: "001.sql", Checksum: strings.TrimPrefix(bodyDigest("local-001.sql"), "sha256:")}}
	events := &prepareEvents{}
	d := Downloader{Client: server.Client(), MirrorBase: server.URL, SourceDir: sourceDir, Observer: events}
	r, err := d.PrepareLock(context.Background(), lock, cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Artifacts) != 7 || r.FromCache != 0 || r.InstallAuthorized {
		t.Fatalf("bad result: %+v", r)
	}
	var total int64
	for _, a := range r.Artifacts {
		fi, err := os.Stat(a.Path)
		if err != nil || fi.Mode().Perm() != 0600 {
			t.Fatalf("cache file not private: %+v %v", a, err)
		}
		total += a.Size
	}
	if total != r.BytesVerified {
		t.Fatal("verified byte totals omitted lock entries")
	}
	intermediate := false
	for _, e := range events.events {
		if e.State == "downloading" && e.BytesDownloaded > 0 && e.BytesVerified == 0 {
			intermediate = true
		}
		if e.Source != "" && e.Source != "release-package" && e.Source != server.URL {
			t.Fatal("progress reported guessed/original source")
		}
	}
	if !intermediate {
		t.Fatal("no real streamed progress")
	}
	count := calls.Load()
	r, err = d.PrepareLock(context.Background(), lock, cache)
	if err != nil || r.FromCache != 7 || calls.Load() != count {
		t.Fatalf("cache hit made network requests: %+v %v", r, err)
	}
	if err := os.WriteFile(r.Artifacts[0].Path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = d.PrepareLock(context.Background(), lock, cache); err == nil {
		t.Fatal("corrupt cache trusted")
	}
}

func TestPrepareInterruptedTransferRetriesAndPublishesOnlyVerified(t *testing.T) {
	lock, paths := prepareFixture(t, false)
	var layerCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := paths[r.URL.Path]
		if strings.Contains(r.URL.Path, "/blobs/") && len(data) > 1000 && layerCalls.Add(1) == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			w.Write(data[:30])
			return
		}
		w.Write(data)
	}))
	defer server.Close()
	events := &prepareEvents{}
	cache := prepareTestCache(t)
	d := Downloader{Client: server.Client(), MirrorBase: server.URL, Observer: events}
	result, err := d.PrepareLock(context.Background(), lock, cache)
	if err != nil {
		t.Fatal(err)
	}
	if layerCalls.Load() != 2 || len(result.Artifacts) != 3 {
		t.Fatalf("no bounded retry: %+v", result)
	}
	retried := false
	for _, e := range events.events {
		if e.State == "retry" {
			retried = true
		}
		if e.State == "downloading" && e.BytesVerified != 0 {
			t.Fatal("partial bytes called verified")
		}
	}
	if !retried {
		t.Fatal("retry progress absent")
	}
	for _, e := range events.events {
		if e.ObservedAt.IsZero() {
			t.Fatal("progress timestamp missing")
		}
		if e.State == "retry" && (e.Source != server.URL || e.Error != "content transfer interrupted" || e.BytesDownloaded != 30 || e.Attempt != 1) {
			t.Fatalf("retry lost source, cause or actual byte progress: %+v", e)
		}
	}
	files, err := os.ReadDir(filepath.Join(cache, "blobs", "sha256"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".part-") {
			t.Fatal("partial download left published")
		}
	}
}

func TestPrepareDigestMismatchAndRedirectDoNotPublish(t *testing.T) {
	for _, mode := range []string{"digest", "redirect", "credential-url"} {
		t.Run(mode, func(t *testing.T) {
			lock, paths := prepareFixture(t, false)
			var destinations atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/destination" {
					destinations.Add(1)
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, "/destination", 302)
					return
				}
				w.Write([]byte("token=do-not-leak"))
			}))
			defer server.Close()
			_ = paths
			d := Downloader{Client: server.Client(), MirrorBase: server.URL}
			if mode == "credential-url" {
				d.MirrorBase = server.URL + "?token=do-not-leak"
			}
			cache := prepareTestCache(t)
			_, err := d.PrepareLock(context.Background(), lock, cache)
			if err == nil || strings.Contains(err.Error(), "do-not-leak") || destinations.Load() != 0 {
				t.Fatalf("invalid content/redirect accepted or leaked: %v", err)
			}
			path, _ := d.cachedPath(cache, lock.Images[0].Digest)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("invalid bytes entered cache")
			}
		})
	}
}

func TestPrepareCancellationAndSymlinkFailClosed(t *testing.T) {
	lock, _ := prepareFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := Downloader{}
	if _, err := d.PrepareLock(ctx, lock, prepareTestCache(t)); err == nil {
		t.Fatal("canceled transfer succeeded")
	}
	parent := t.TempDir()
	link := filepath.Join(parent, "linked-cache")
	target := t.TempDir()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PrepareLock(context.Background(), lock, link); err == nil {
		t.Fatal("symlink cache accepted")
	}
}

func TestPrepareWrongArchitectureBlocksBeforeLayers(t *testing.T) {
	lock, paths := prepareFixture(t, true)
	lock.Images[0].ManifestDigest = bodyDigest("wrong")
	var blobCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/") {
			blobCalls.Add(1)
		}
		w.Write(paths[r.URL.Path])
	}))
	defer server.Close()
	d := Downloader{Client: server.Client(), MirrorBase: server.URL}
	if _, err := d.PrepareLock(context.Background(), lock, prepareTestCache(t)); err == nil || blobCalls.Load() != 0 {
		t.Fatal("wrong locked architecture downloaded layers")
	}
}

func TestPrepareFailedRunCanReuseOnlyCompletedArtifacts(t *testing.T) {
	lock, paths := prepareFixture(t, false)
	var failed atomic.Bool
	failed.Store(true)
	var layerCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := paths[r.URL.Path]
		if len(data) > 1000 {
			layerCalls.Add(1)
			if failed.Load() {
				w.WriteHeader(503)
				return
			}
		}
		w.Write(data)
	}))
	defer server.Close()
	d := Downloader{Client: server.Client(), MirrorBase: server.URL}
	cache := prepareTestCache(t)
	failedResult, err := d.PrepareLock(context.Background(), lock, cache)
	if err == nil || layerCalls.Load() != 3 {
		t.Fatal("retry budget not enforced")
	}
	if failedResult.Status != "failed" || failedResult.Error == "" || len(failedResult.Artifacts) != 2 || failedResult.LockDigest != lockContentDigest(lock) || failedResult.FinishedAt.Before(failedResult.StartedAt) {
		t.Fatalf("verified partial state was lost: %+v", failedResult)
	}
	failed.Store(false)
	r, err := d.PrepareLock(context.Background(), lock, cache)
	if err != nil {
		t.Fatal(err)
	}
	if r.FromCache != 2 || layerCalls.Load() != 4 {
		t.Fatalf("restart did not preserve only verified entries: %+v", r)
	}
}

func TestPrepareCancellationRetainsCompletedArtifacts(t *testing.T) {
	lock, paths := prepareFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := paths[r.URL.Path]
		if len(data) > 1000 {
			cancel()
			return
		}
		w.Write(data)
	}))
	defer server.Close()
	d := Downloader{Client: server.Client(), MirrorBase: server.URL}
	result, err := d.PrepareLock(ctx, lock, prepareTestCache(t))
	if err == nil || result.Status != "canceled" || len(result.Artifacts) != 2 || result.Error == "" || result.InstallAuthorized {
		t.Fatalf("cancellation lost verified partial state: %+v %v", result, err)
	}
}

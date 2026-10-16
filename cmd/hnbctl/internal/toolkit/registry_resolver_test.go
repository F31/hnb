package toolkit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func resolverFixture(t *testing.T, auth, wrongConfig, wrongHeader bool) (*httptest.Server, string, string) {
	t.Helper()
	config := `{"os":"linux","architecture":"amd64"}`
	if wrongConfig {
		config = `{"os":"linux","architecture":"arm64"}`
	}
	manifest := ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: ociDescriptor{Digest: bodyDigest(config), Size: int64(len(config))}}
	manifestBytes, _ := json.Marshal(manifest)
	digest := bodyDigest(string(manifestBytes))
	index := ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json", Manifests: []ociDescriptor{{Digest: digest, Size: int64(len(manifestBytes)), Platform: &ociPlatform{OS: "linux", Architecture: "amd64"}}}}
	indexBytes, _ := json.Marshal(index)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if r.Header.Get("Authorization") != "" || r.URL.Query().Get("scope") != "repository:fixture:pull" {
				t.Error("credentials forwarded or scope changed")
			}
			_, _ = io.WriteString(w, `{"token":"opaque-fixture-token"}`)
			return
		}
		if auth && r.Header.Get("Authorization") != "Bearer opaque-fixture-token" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="fixture",scope="repository:fixture:pull"`)
			w.WriteHeader(401)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/manifests/v1") {
			if wrongHeader {
				w.Header().Set("Docker-Content-Digest", bodyDigest("wrong"))
			}
			_, _ = w.Write(indexBytes)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/manifests/"+digest) {
			_, _ = w.Write(manifestBytes)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/blobs/"+bodyDigest(config)) {
			_, _ = io.WriteString(w, config)
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(server.Close)
	return server, bodyDigest(string(indexBytes)), digest
}

func TestRegistryResolvesActualIndexManifestAndConfigBytes(t *testing.T) {
	server, index, manifest := resolverFixture(t, true, false, false)
	resolved, err := NewRegistryResolver(server.Client()).Resolve(context.Background(), "fixture", strings.TrimPrefix(server.URL, "https://")+"/fixture:v1")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Digest != index || resolved.ManifestDigest != manifest || resolved.Platform != "linux/amd64" {
		t.Fatalf("wrong content identities: %+v", resolved)
	}
}

func TestRegistryRejectsUnverifiedIdentityOrArchitecture(t *testing.T) {
	for _, tc := range []struct{ config, header bool }{{true, false}, {false, true}} {
		server, _, _ := resolverFixture(t, false, tc.config, tc.header)
		if _, err := NewRegistryResolver(server.Client()).Resolve(context.Background(), "fixture", strings.TrimPrefix(server.URL, "https://")+"/fixture:v1"); err == nil {
			t.Fatal("wrong header/config platform accepted")
		}
	}
	for _, ref := range []string{"registry.invalid/image", "registry.invalid/image:latest", "registry.invalid/image:Latest", "registry.invalid/../image:v1", "https://user:secret@registry.invalid/image:v1", "registry.invalid/image@" + placeholderDigest} {
		if _, _, err := registryReference(ref); err == nil {
			t.Fatal("invalid OCI input accepted")
		}
	}
}

func TestRegistryRealIdentity(t *testing.T) {
	ref := os.Getenv("HNB_REGISTRY_IDENTITY_E2E_REF")
	if ref == "" {
		t.Skip("set HNB_REGISTRY_IDENTITY_E2E_REF for explicit registry observation")
	}
	resolver := NewRegistryResolver(nil)
	if err := resolver.DeclareRedirectOrigins(strings.Split(os.Getenv("HNB_REGISTRY_IDENTITY_E2E_REDIRECT_ORIGINS"), ",")); err != nil {
		t.Fatal(err)
	}
	image, err := resolver.Resolve(context.Background(), "registry-observation", ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ref=%s index/root=%s manifest=%s platform=%s (manifest/config bytes only; no layers imported)", image.Ref, image.Digest, image.ManifestDigest, image.Platform)
	for origin := range resolver.ObservedRedirect {
		t.Logf("observed content redirect origin=%s", origin)
	}
}

func TestUndeclaredRedirectIsRefused(t *testing.T) {
	blob := "synthetic blob"
	var sawTokenOnCDN bool
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawTokenOnCDN = true
		}
		_, _ = io.WriteString(w, blob)
	}))
	defer cdn.Close()
	registry := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/blob", http.StatusTemporaryRedirect)
	}))
	defer registry.Close()
	resolver := NewRegistryResolver(registry.Client())
	token := "fixture-registry-token"
	if _, _, err := resolver.fetch(context.Background(), registry.URL+"/blobs/fixture", &token, "", 0); err == nil {
		t.Fatal("undeclared redirect origin accepted")
	}
	if sawTokenOnCDN {
		t.Fatal("registry token forwarded to the redirect host")
	}
	// The CDN origin is a different server, so declare its origin explicitly.
	if err := resolver.DeclareRedirectOrigins([]string{cdn.URL}); err != nil {
		t.Fatal(err)
	}
	body, digest, err := resolver.fetch(context.Background(), registry.URL+"/blobs/fixture", &token, "", 0)
	if err != nil || digest != bodyDigest(blob) || string(body) != blob {
		t.Fatalf("declared redirect origin could not be verified: %v", err)
	}
	if sawTokenOnCDN {
		t.Fatal("registry token forwarded to the declared CDN origin")
	}
	for _, origin := range []string{"http://host/path", "https://user:secret@host", "cdn.example.invalid", "ftp://cdn.example.invalid"} {
		declaring := NewRegistryResolver(nil)
		if err := declaring.DeclareRedirectOrigins([]string{origin}); err == nil {
			t.Fatal("invalid declared redirect origin accepted")
		}
	}
}

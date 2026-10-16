package toolkit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOCIManifestMetadataCannotProveLayerDownload(t *testing.T) {
	for _, failLayer := range []bool{false, true} {
		config, layer := "synthetic config bytes", "synthetic layer bytes"
		manifest := ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: ociDescriptor{Digest: bodyDigest(config), Size: int64(len(config))}, Layers: []ociDescriptor{{Digest: bodyDigest(layer), Size: int64(len(layer))}}}
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		objects := map[string]string{"/v2/hnb/fixture/manifests/" + bodyDigest(string(body)): string(body), "/v2/hnb/fixture/blobs/" + bodyDigest(config): config, "/v2/hnb/fixture/blobs/" + bodyDigest(layer): layer}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, exists := objects[r.URL.Path]
			if !exists {
				w.WriteHeader(404)
				return
			}
			if r.Method == http.MethodHead {
				return
			}
			if failLayer && strings.HasSuffix(r.URL.Path, bodyDigest(layer)) {
				w.WriteHeader(403)
				_, _ = io.WriteString(w, "token=secret")
				return
			}
			_, _ = io.WriteString(w, data)
		}))
		image := PinnedImage{Ref: strings.TrimPrefix(server.URL, "https://") + "/hnb/fixture:v1", Digest: bodyDigest(string(body))}
		checks := NewHTTPProbe(server.Client()).ProbeImage(context.Background(), "fixture", image)
		server.Close()
		if len(checks) != 6 {
			t.Fatalf("missing manifest/config/layer paths: %+v", checks)
		}
		if checks[0].Status != StatusPass || checks[1].Status != StatusPass || checks[4].Status != StatusPass {
			t.Fatalf("metadata unavailable in fixture: %+v", checks)
		}
		if failLayer {
			if checks[5].ReasonCode != "ARTIFACT_CONTENT_UNAVAILABLE" || strings.Contains(checks[5].Actual, "secret") {
				t.Fatal("manifest/HEAD access concealed layer failure or leaked its body")
			}
		} else {
			for _, check := range checks {
				if check.Status != StatusPass {
					t.Fatalf("locked-byte fixture rejected: %+v", check)
				}
			}
		}
	}
}

func TestOCIIndexCannotClaimArchitectureClosure(t *testing.T) {
	body := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, body)
		}
	}))
	defer server.Close()
	checks := NewHTTPProbe(server.Client()).ProbeImage(context.Background(), "fixture", PinnedImage{Ref: strings.TrimPrefix(server.URL, "https://") + "/fixture:v1", Digest: bodyDigest(body)})
	if len(checks) != 3 || checks[2].ReasonCode != "OCI_ARCH_MANIFEST_UNRESOLVED" {
		t.Fatalf("index access promoted to full image availability: %+v", checks)
	}
}

func TestOCIDescriptorSizeMustMatchBytes(t *testing.T) {
	config := "synthetic config"
	manifest := ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: ociDescriptor{Digest: bodyDigest(config), Size: int64(len(config) + 1)}}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			_, _ = w.Write(body)
		} else {
			_, _ = io.WriteString(w, config)
		}
	}))
	defer server.Close()
	checks := NewHTTPProbe(server.Client()).ProbeImage(context.Background(), "fixture", PinnedImage{Ref: strings.TrimPrefix(server.URL, "https://") + "/fixture:v1", Digest: bodyDigest(string(body))})
	if len(checks) != 4 || checks[3].ReasonCode != "ARTIFACT_SIZE_MISMATCH" {
		t.Fatalf("incorrect descriptor size accepted: %+v", checks)
	}
}

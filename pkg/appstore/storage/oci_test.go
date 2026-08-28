package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestIsSHA256Digest(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{
		{testDigest, true},
		{"sha256:abc", false},
		{"sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"sha512:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
	} {
		if got := IsSHA256Digest(test.value); got != test.valid {
			t.Errorf("IsSHA256Digest(%q) = %v, want %v", test.value, got, test.valid)
		}
	}
}

func TestVerifyManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/hnb/generic/manifests/"+testDigest {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":2},"layers":[{"mediaType":"application/octet-stream","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":42}]}`))
	}))
	defer server.Close()

	storage := NewOCIStorage(StorageConfig{RegistryURL: server.URL})
	meta, err := storage.VerifyManifest(context.Background(), "hnb/generic", testDigest)
	if err != nil {
		t.Fatalf("VerifyManifest failed: %v", err)
	}
	if meta.Digest != testDigest || meta.SizeBytes != 42 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if meta.RegistryURL != server.URL+"/hnb/generic@"+testDigest {
		t.Fatalf("unexpected registry URL %s", meta.RegistryURL)
	}
}

func TestVerifyManifestRejectsMissingAndMismatchedDigest(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()
		_, err := NewOCIStorage(StorageConfig{RegistryURL: server.URL}).VerifyManifest(context.Background(), "hnb/generic", testDigest)
		if !errors.Is(err, ErrManifestNotFound) {
			t.Fatalf("expected ErrManifestNotFound, got %v", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Docker-Content-Digest", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		_, err := NewOCIStorage(StorageConfig{RegistryURL: server.URL}).VerifyManifest(context.Background(), "hnb/generic", testDigest)
		if !errors.Is(err, ErrDigestMismatch) {
			t.Fatalf("expected ErrDigestMismatch, got %v", err)
		}
	})
}

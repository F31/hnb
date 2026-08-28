package appstore

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestEncodeReleaseManifestIsDeterministic(t *testing.T) {
	first, firstDigest, err := EncodeReleaseManifest(map[string]any{
		"version": 1,
		"config":  map[string]any{"region": "west", "replicas": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, secondDigest, err := EncodeReleaseManifest(map[string]any{
		"config":  map[string]any{"replicas": 2, "region": "west"},
		"version": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || firstDigest != secondDigest {
		t.Fatalf("manifest encoding is not deterministic: %s %s", first, second)
	}
}

func TestEncodeReleaseManifestHashesEmptyObject(t *testing.T) {
	data, digest, err := EncodeReleaseManifest(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("empty manifest = %s", data)
	}
	sum := sha256.Sum256(data)
	expected := "sha256:" + hex.EncodeToString(sum[:])
	if digest != expected {
		t.Fatalf("digest = %s, want %s", digest, expected)
	}
}

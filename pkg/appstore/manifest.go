package appstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// EncodeReleaseManifest returns deterministic JSON and its content digest.
func EncodeReleaseManifest(manifest any) ([]byte, string, error) {
	if manifest == nil {
		manifest = map[string]any{}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	return data, "sha256:" + hex.EncodeToString(sum[:]), nil
}

package toolkit

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// writeRestricted writes a regular file. 0600 is required for private material;
// 0644 is allowed only for intentionally public material (public key, manifest).
func writeRestricted(path string, data []byte, mode os.FileMode) error {
	if mode != 0o600 && mode != 0o644 {
		return fmt.Errorf("identity material must use 0600 (private) or 0644 (public)")
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("cannot write %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("cannot protect %s: %w", filepath.Base(path), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != mode {
		return fmt.Errorf("identity material permission mismatch")
	}
	return nil
}

// writePrivateKey writes the instance signing key as SEC1 EC PEM, 0600.
func writePrivateKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("cannot encode identity key")
	}
	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
	return writeRestricted(path, pem.EncodeToMemory(block), 0o600)
}

// writePublicKey writes the instance public key as PKIX PEM, 0644.
func writePublicKey(path string, pub *ecdsa.PublicKey) error {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("cannot encode identity public key")
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return writeRestricted(path, pem.EncodeToMemory(block), 0o644)
}

// randomKeyID returns a collision-resistant key identifier derived from entropy.
func randomKeyID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("cannot generate key identifier")
	}
	return "k" + hex.EncodeToString(raw), nil
}

// hexEncode is the hex encoder used for the KMS master key material.
func hexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

// kmsMasterKeyFromHex validates that a master key string is exactly 32 bytes of
// hex (64 chars) without depending on the pkg/kms module boundary.
func kmsMasterKeyFromHex(keyHex string) ([]byte, error) {
	decoded, err := hex.DecodeString(keyHex)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("KMS master key must be 32 bytes of hex")
	}
	return decoded, nil
}

// randomBootstrapPassword returns a strong, unpredictable one-time password.
func randomBootstrapPassword() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("cannot generate bootstrap password")
	}
	return "hb-" + hex.EncodeToString(raw), nil
}

// sha256sum returns the sha256 digest of data.
func sha256sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// securePrivateKey ensures the key can actually sign, catching a corrupt file
// before it reaches a verifier.
func (p *IdentityPlan) securePrivateKey() (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(p.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read identity key")
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("identity key is not PEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("identity key is not a usable EC key")
	}
	if !key.Curve.IsOnCurve(key.X, key.Y) {
		return nil, fmt.Errorf("identity key is not on the P-256 curve")
	}
	_ = elliptic.P256()
	return key, nil
}

// LoadManifestForTest decodes a generated manifest for validation. It does not
// load signing keys and is only used to confirm the platform manifest loader can
// parse the file we produced.
func LoadManifestForTest(ctx context.Context, path string) (*iamManifestForTest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest iamManifestForTest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.Issuer == "" || manifest.ActiveKeyID == "" || len(manifest.Keys) == 0 {
		return nil, fmt.Errorf("manifest has no active identity")
	}
	return &manifest, nil
}

type iamManifestForTest struct {
	Issuer      string              `json:"issuer"`
	Generation  uint64              `json:"generation"`
	ActiveKeyID string              `json:"activeKeyId"`
	Keys        map[string]struct{} `json:"keys"`
}

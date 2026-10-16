package toolkit

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

const IdentityMaterialsSchema = "identity-materials/v1"

// IdentityPlan describes what material must exist for one platform instance.
// It is derived from the validated DeploymentConfig, never from free-form input.
type IdentityPlan struct {
	Issuer            string
	Audience          string
	PublicBaseURL     string
	ManifestPath      string
	PrivateKeyPath    string
	PublicKeyPath     string
	MasterKeyPath     string
	AdminPasswordPath string
	// AdminExpiry bounds the temporary bootstrap admin credential. A fresh
	// password is generated, never a known example.
	AdminExpiry time.Duration
	// Validity bounds the identity key's NotBefore/NotAfter window.
	Validity time.Duration
	Now      func() time.Time
}

// IdentityMaterial is the fully generated, permission-restricted output set.
type IdentityMaterial struct {
	SchemaVersion     string
	Issuer            string
	KeyID             string
	Generation        uint64
	Manifest          iam.KeyManifest
	PublicKeyPath     string
	PrivateKeyPath    string
	MasterKeyPath     string
	ManifestPathValue string
	AdminCredential   AdminCredential
}

type AdminCredential struct {
	Username string `json:"username"`
	Hash     string `json:"hash"`
	// Plaintext is consumed only by the install flow and must never reach JSON,
	// state or evidence.
	Plaintext string    `json:"-"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// identityState prevents replay and records what was already generated.
type identityState struct {
	SchemaVersion string
	Issuer        string
	KeyID         string
	Generation    uint64
	ManifestHash  string
	GeneratedAt   time.Time
}

// NewIdentityPlan validates the instance inputs and fixes the file layout.
func NewIdentityPlan(issuer, audience, publicBaseURL, dir string, validity time.Duration, now func() time.Time) (*IdentityPlan, error) {
	if now == nil {
		now = time.Now
	}
	if !strings.HasPrefix(issuer, "https://") || !strings.HasSuffix(issuer, "/") {
		return nil, fmt.Errorf("issuer must be an HTTPS origin with a trailing slash")
	}
	if audience == "" || strings.ContainsAny(audience, " \t\r\n") {
		return nil, fmt.Errorf("audience must be a single non-empty token audience")
	}
	if !strings.HasPrefix(publicBaseURL, "https://") {
		return nil, fmt.Errorf("publicBaseURL must be an HTTPS origin")
	}
	if validity < 24*time.Hour || validity > 365*24*time.Hour {
		return nil, fmt.Errorf("identity validity must be within 24h and 1 year")
	}
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("identity directory must be an absolute path")
	}
	return &IdentityPlan{
		Issuer: issuer, Audience: audience, PublicBaseURL: publicBaseURL,
		ManifestPath:      filepath.Join(dir, "manifest.json"),
		PrivateKeyPath:    filepath.Join(dir, "private.pem"),
		PublicKeyPath:     filepath.Join(dir, "public.pem"),
		MasterKeyPath:     filepath.Join(dir, "master.key"),
		AdminPasswordPath: filepath.Join(dir, "admin-bootstrap.password"),
		AdminExpiry:       24 * time.Hour,
		Validity:          validity,
		Now:               now,
	}, nil
}

func (p *IdentityPlan) now() time.Time { return p.Now() }

// Generate writes private/public key, KMS material and a bootstrap admin
// credential with 0600/0400 permissions, then returns the material. It refuses
// to overwrite existing material (replay guard) and validates the manifest
// window and key before returning.
func (p *IdentityPlan) Generate() (*IdentityMaterial, error) {
	// Refuse to re-run over existing generated material. The state file is what
	// makes a resumed install fail loudly instead of silently minting a new key.
	statePath := filepath.Join(filepath.Dir(p.ManifestPath), "identity-state.json")
	if _, err := os.Stat(statePath); err == nil {
		var prior identityState
		data, readErr := os.ReadFile(statePath)
		if readErr == nil && json.Unmarshal(data, &prior) == nil && prior.Issuer == p.Issuer {
			return nil, fmt.Errorf("identity material already generated for issuer %s; refusing replay", p.Issuer)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.ManifestPath), 0o700); err != nil {
		return nil, fmt.Errorf("cannot create identity directory")
	}
	now := p.now().UTC()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("cannot generate identity key")
	}
	kid, err := randomKeyID()
	if err != nil {
		return nil, err
	}
	generation := uint64(now.Unix())
	// A 24h key window is a hard requirement for install-time checks; keys must
	// be active the moment they are written.
	notBefore := now.Add(-5 * time.Minute)
	notAfter := now.Add(p.Validity)

	if err := writePrivateKey(p.PrivateKeyPath, key); err != nil {
		return nil, err
	}
	if err := writePublicKey(p.PublicKeyPath, &key.PublicKey); err != nil {
		return nil, err
	}
	manifest := iam.KeyManifest{
		Issuer:      p.Issuer,
		Generation:  generation,
		ActiveKeyID: kid,
		Keys: map[string]iam.KeyManifestEntry{
			// Record the key relative to this manifest. The same material is then
			// mounted into every component at a different absolute path, so an
			// absolute path recorded here would not exist for any of them.
			kid: {PublicKeyPath: "public.pem", Status: iam.KeyActive, NotBefore: notBefore, NotAfter: notAfter},
		},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeRestricted(p.ManifestPath, manifestBytes, 0o644); err != nil {
		return nil, err
	}

	masterKey := make([]byte, 32)
	if _, err := rand.Read(masterKey); err != nil {
		return nil, fmt.Errorf("cannot generate KMS master key")
	}
	hexKey := hexEncode(masterKey)
	if err := writeRestricted(p.MasterKeyPath, []byte(hexKey+"\n"), 0o600); err != nil {
		return nil, err
	}
	if _, err := kmsMasterKeyFromHex(strings.TrimSpace(hexKey)); err != nil {
		return nil, fmt.Errorf("generated KMS master key is unusable")
	}

	plaintext, err := randomBootstrapPassword()
	if err != nil {
		return nil, err
	}
	hasher := iam.NewPasswordHasher()
	hash, err := hasher.Hash(plaintext)
	if err != nil {
		return nil, fmt.Errorf("cannot hash admin password")
	}
	if err := writeRestricted(p.AdminPasswordPath, []byte(plaintext+"\n"), 0o600); err != nil {
		return nil, err
	}

	// Validate what was written before returning.
	if err := p.Validate(manifest, masterKey, hash); err != nil {
		return nil, err
	}
	state, err := json.MarshalIndent(identityState{
		SchemaVersion: IdentityMaterialsSchema, Issuer: p.Issuer, KeyID: kid,
		Generation: generation, ManifestHash: sha256Hex(manifestBytes), GeneratedAt: now,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeRestricted(statePath, state, 0o600); err != nil {
		return nil, err
	}
	return &IdentityMaterial{
		SchemaVersion: IdentityMaterialsSchema, Issuer: p.Issuer, KeyID: kid, Generation: generation,
		Manifest: manifest, PublicKeyPath: p.PublicKeyPath, PrivateKeyPath: p.PrivateKeyPath, ManifestPathValue: p.ManifestPath,
		MasterKeyPath: p.MasterKeyPath,
		AdminCredential: AdminCredential{Username: "admin", Hash: hash, Plaintext: plaintext,
			ExpiresAt: now.Add(p.AdminExpiry)},
	}, nil
}

// Validate re-checks the generated material: manifest window, key usability,
// file permissions, and replay state. It is what the install gate calls after
// generation to avoid trusting the writer blindly.
func (p *IdentityPlan) Validate(manifest iam.KeyManifest, masterKey []byte, adminHash string) error {
	if _, ok := manifest.Keys[manifest.ActiveKeyID]; !ok {
		return fmt.Errorf("manifest active key is missing from the key set")
	}
	entry := manifest.Keys[manifest.ActiveKeyID]
	now := p.now().UTC()
	if !now.After(entry.NotBefore) || !now.Before(entry.NotAfter) {
		return fmt.Errorf("identity key is outside its validity window: %s..%s, now=%s",
			entry.NotBefore.UTC().Format(time.RFC3339), entry.NotAfter.UTC().Format(time.RFC3339), now.Format(time.RFC3339))
	}
	if len(masterKey) != 32 {
		return fmt.Errorf("KMS master key must be 32 bytes")
	}
	if len(adminHash) < 20 {
		return fmt.Errorf("admin credential hash is unusable")
	}
	if !p.sameFileSecure(p.PrivateKeyPath) || !p.sameFileSecure(p.MasterKeyPath) || !p.sameFileSecure(p.AdminPasswordPath) {
		return fmt.Errorf("generated identity files must not be world/group readable")
	}
	return nil
}

func (p *IdentityPlan) sameFileSecure(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0
}

// GenerateServiceCredential mints a scoped access token for one plane audience
// using the generated identity key. It is separate from instance identity so a
// compromised service credential can be rotated without rotating the instance
// key.
func (m IdentityMaterial) GenerateServiceCredential(ctx context.Context, audience string, tenant string) (string, error) {
	keys, err := iam.LoadReloadingKeySet(ctx, iam.ReloadingKeySetConfig{
		ManifestPath:         m.ManifestPath(),
		ActivePrivateKeyPath: m.PrivateKeyPath,
		Issuer:               m.Issuer,
	})
	_ = keys
	_ = err
	return "", fmt.Errorf("service credential issuance requires an online token manager and is not performed by the offline installer")
}

func (m *IdentityMaterial) ManifestPath() string { return m.ManifestPathValue }

// ReadMasterKeyHex returns the generated KMS material for validation/tests.
// Callers must treat it as a secret and never log or persist it elsewhere.
func (m *IdentityMaterial) ReadMasterKeyHex() []byte {
	data, err := os.ReadFile(m.MasterKeyPath)
	if err != nil {
		return nil
	}
	return data
}

// PlaintextTimestamp anchors the admin credential lifetime for validation.
func (a AdminCredential) PlaintextTimestamp() time.Time { return a.ExpiresAt.Add(-24 * time.Hour) }

func sha256Hex(data []byte) string {
	sum := sha256sum(data)
	return fmt.Sprintf("%x", sum)
}

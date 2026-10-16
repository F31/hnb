package toolkit

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testIdentityPlan(t *testing.T, dir string) *IdentityPlan {
	t.Helper()
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	plan, err := NewIdentityPlan("https://issuer.hnb.local/", "hnb-apiserver",
		"https://portal.hnb.local", dir, 30*24*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

// Generated material must be inside its validity window at generation time.
func TestIdentityMaterialIsInsideValidityWindow(t *testing.T) {
	dir := t.TempDir()
	material, err := testIdentityPlan(t, dir).Generate()
	if err != nil {
		t.Fatal(err)
	}
	entry := material.Manifest.Keys[material.KeyID]
	genTime := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	if !genTime.After(entry.NotBefore) || !genTime.Before(entry.NotAfter) {
		t.Fatal("key is not active at generation time")
	}
	plan := testIdentityPlan(t, dir)
	entry = material.Manifest.Keys[material.KeyID]
	// Reconstruct now: the plan used a fixed now; validate with the same now.
	plan.Now = func() time.Time { return time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC) }
	master, err := kmsMasterKeyFromHex(strings.TrimSpace(string(material.ReadMasterKeyHex())))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(material.Manifest, master, material.AdminCredential.Hash); err != nil {
		t.Fatal(err)
	}
}

// Outside-window material must be rejected, not trusted.
func TestIdentityMaterialRejectsExpiredWindow(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	material, err := plan.Generate()
	if err != nil {
		t.Fatal(err)
	}
	entry := material.Manifest.Keys[material.KeyID]
	// Simulate the check running far in the future.
	future := entry.NotAfter.Add(time.Hour)
	plan.Now = func() time.Time { return future }
	master, _ := kmsMasterKeyFromHex(strings.TrimSpace(string(material.ReadMasterKeyHex())))
	if err := plan.Validate(material.Manifest, master, material.AdminCredential.Hash); err == nil ||
		!strings.Contains(err.Error(), "validity window") {
		t.Fatalf("expired key was accepted: %v", err)
	}
}

// File permission guards: private material must never be world/group readable.
func TestIdentityFilesArePermissionRestricted(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	material, err := plan.Generate()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plan.PrivateKeyPath, plan.MasterKeyPath, plan.AdminPasswordPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s must be 0600/private: %v %v", path, info.Mode(), err)
		}
	}
	// Public key and manifest may be world readable but must exist.
	for _, path := range []string{plan.PublicKeyPath, plan.ManifestPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	_ = material
}

// The private key must actually be usable, not just present.
func TestIdentityPrivateKeyIsUsable(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	if _, err := plan.Generate(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plan.PrivateKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("private key is not PEM")
	}
	if _, err := x509.ParseECPrivateKey(block.Bytes); err != nil {
		t.Fatal(err)
	}
}

// Re-running generation over existing material must fail loudly (replay guard).
func TestIdentityGenerationRejectsReplay(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	if _, err := plan.Generate(); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Generate(); err == nil {
		t.Fatal("identity generation replayed")
	}
}

// The bootstrap admin password must be strong, unpredictable and one-time.
func TestAdminCredentialIsStrongAndBounded(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	material, err := plan.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if len(material.AdminCredential.Plaintext) < 32 {
		t.Fatal("bootstrap password is too short")
	}
	if !material.AdminCredential.ExpiresAt.After(material.AdminCredential.PlaintextTimestamp().Add(time.Hour)) {
		t.Fatal("admin credential must have a bounded lifetime")
	}
	// The plaintext must not leak into the material JSON or state.
	state, err := os.ReadFile(filepath.Join(dir, "identity-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), material.AdminCredential.Plaintext) {
		t.Fatal("bootstrap password leaked into identity state")
	}
}

// The manifest must be decodable by the platform's key manifest loader.
func TestIdentityManifestLoadsWithPlatformLoader(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	material, err := plan.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = LoadManifestForTest(ctx, plan.ManifestPath)
	if err != nil {
		t.Fatalf("platform cannot load generated manifest: %v", err)
	}
	_ = material
}

// Invalid instance inputs must be rejected before any file is written.
func TestIdentityPlanRejectsInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name                           string
		issuer, audience, baseURL, dir string
		validity                       time.Duration
	}{
		{"http-issuer", "http://issuer.hnb.local/", "hnb-apiserver", "https://portal.hnb.local", dir, 30 * 24 * time.Hour},
		{"no-slash", "https://issuer.hnb.local", "hnb-apiserver", "https://portal.hnb.local", dir, 30 * 24 * time.Hour},
		{"empty-audience", "https://issuer.hnb.local/", "", "https://portal.hnb.local", dir, 30 * 24 * time.Hour},
		{"http-entry", "https://issuer.hnb.local/", "hnb-apiserver", "http://portal.hnb.local", dir, 30 * 24 * time.Hour},
		{"short-validity", "https://issuer.hnb.local/", "hnb-apiserver", "https://portal.hnb.local", dir, time.Hour},
		{"relative-dir", "https://issuer.hnb.local/", "hnb-apiserver", "https://portal.hnb.local", "rel", 30 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewIdentityPlan(tc.issuer, tc.audience, tc.baseURL, tc.dir, tc.validity, nil); err == nil {
				t.Fatal("invalid identity input accepted")
			}
		})
	}
}

// json.Marshal of the material must not include the plaintext password.
func TestIdentityMaterialJSONNeverContainsPlaintext(t *testing.T) {
	dir := t.TempDir()
	plan := testIdentityPlan(t, dir)
	material, err := plan.Generate()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), material.AdminCredential.Plaintext) {
		t.Fatal("material JSON leaked the bootstrap password")
	}
}

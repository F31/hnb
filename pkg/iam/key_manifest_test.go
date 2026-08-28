package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type manifestFixture struct {
	dir          string
	manifestPath string
	privatePath  string
	keys         map[string]*ecdsa.PrivateKey
	publicPaths  map[string]string
}

func newManifestFixture(t *testing.T, kids ...string) *manifestFixture {
	t.Helper()
	fixture := &manifestFixture{dir: t.TempDir(), keys: make(map[string]*ecdsa.PrivateKey), publicPaths: make(map[string]string)}
	fixture.manifestPath = filepath.Join(fixture.dir, "manifest.json")
	fixture.privatePath = filepath.Join(fixture.dir, "active-private.pem")
	for _, kid := range kids {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		fixture.keys[kid] = key
		publicDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
		path := filepath.Join(fixture.dir, kid+"-public.pem")
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
			t.Fatal(err)
		}
		fixture.publicPaths[kid] = path
	}
	return fixture
}

func (f *manifestFixture) setPrivate(t *testing.T, kid string) {
	t.Helper()
	der, _ := x509.MarshalPKCS8PrivateKey(f.keys[kid])
	writeAtomic(t, f.privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
}

func (f *manifestFixture) writeManifest(t *testing.T, manifest KeyManifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeAtomic(t, f.manifestPath, data, 0o600)
}

func writeAtomic(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func testManifest(f *manifestFixture, generation uint64, active string, statuses map[string]KeyStatus, before, after time.Time) KeyManifest {
	keys := make(map[string]KeyManifestEntry, len(statuses))
	for kid, status := range statuses {
		keys[kid] = KeyManifestEntry{PublicKeyPath: f.publicPaths[kid], Status: status, NotBefore: before, NotAfter: after}
	}
	return KeyManifest{Issuer: "https://issuer.example", Generation: generation, ActiveKeyID: active, Keys: keys}
}

func TestReloadingKeySetOverlapSwitchAndRevocation(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	f := newManifestFixture(t, "k1", "k2")
	f.setPrivate(t, "k1")
	f.writeManifest(t, testManifest(f, 1, "k1", map[string]KeyStatus{"k1": KeyActive, "k2": KeyPending}, now.Add(-time.Hour), now.Add(time.Hour)))
	set, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, ActivePrivateKeyPath: f.privatePath, Issuer: "https://issuer.example", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.VerificationKey(context.Background(), "k2"); err == nil {
		t.Fatal("pending key was accepted")
	}

	f.setPrivate(t, "k2")
	f.writeManifest(t, testManifest(f, 2, "k2", map[string]KeyStatus{"k1": KeyRetiring, "k2": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := set.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := set.VerificationKey(context.Background(), "k1"); err != nil {
		t.Fatalf("retiring key rejected during overlap: %v", err)
	}
	kid, key, err := set.CurrentSigningKey(context.Background())
	if err != nil || kid != "k2" || key != f.keys["k2"] && key.D.Cmp(f.keys["k2"].D) != 0 {
		t.Fatalf("CurrentSigningKey() = %q, %v", kid, err)
	}

	f.writeManifest(t, testManifest(f, 3, "k2", map[string]KeyStatus{"k1": KeyRevoked, "k2": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := set.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := set.VerificationKey(context.Background(), "k1"); err == nil {
		t.Fatal("revoked key remained accepted")
	}
}

func TestTokenManagerHotSwitchesSigningKeyAndRevokesOldTokens(t *testing.T) {
	now := time.Unix(2_000_000_000, 0).UTC()
	f := newManifestFixture(t, "k1", "k2")
	f.setPrivate(t, "k1")
	f.writeManifest(t, testManifest(f, 1, "k1", map[string]KeyStatus{"k1": KeyActive, "k2": KeyPending}, now.Add(-time.Hour), now.Add(time.Hour)))
	set, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, ActivePrivateKeyPath: f.privatePath, Issuer: "https://issuer.example", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	identities := &testIdentities{identity: &Identity{UserID: "user-1", SubjectID: "subject-1", SubjectType: "user", MembershipID: "membership-1", TenantID: "tenant-1"}}
	refresh := &testRefreshStore{records: make(map[string]RefreshTokenRecord)}
	manager, err := NewTokenManager(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "hnb-apiserver", Audiences: []string{"hnb-apiserver"}, AccessTTL: MaxAccessTokenTTL, RefreshTTL: time.Hour, Now: func() time.Time { return now }}, set, set, identities, identities, refresh)
	if err != nil {
		t.Fatal(err)
	}
	k1Token, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := manager.Verify(context.Background(), k1Token.Token); err != nil || claims.KeyID != "k1" {
		t.Fatalf("initial token key = %v, err = %v", claims, err)
	}

	f.setPrivate(t, "k2")
	f.writeManifest(t, testManifest(f, 2, "k2", map[string]KeyStatus{"k1": KeyRetiring, "k2": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := set.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	k2Token, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := manager.Verify(context.Background(), k2Token.Token); err != nil || claims.KeyID != "k2" {
		t.Fatalf("rotated token key = %v, err = %v", claims, err)
	}
	if _, err := manager.Verify(context.Background(), k1Token.Token); err != nil {
		t.Fatalf("retiring K1 token failed during overlap: %v", err)
	}

	f.writeManifest(t, testManifest(f, 3, "k2", map[string]KeyStatus{"k1": KeyRevoked, "k2": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := set.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Verify(context.Background(), k1Token.Token); err == nil {
		t.Fatal("K1 token remained valid after emergency revocation")
	}
	if _, err := manager.Verify(context.Background(), k2Token.Token); err != nil {
		t.Fatalf("active K2 token failed after K1 revocation: %v", err)
	}
}

func TestReloadingKeySetExpiryNotBeforeAndFailClosedStatuses(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	f := newManifestFixture(t, "active", "expired", "revoked", "pending")
	f.setPrivate(t, "active")
	manifest := testManifest(f, 1, "active", map[string]KeyStatus{"active": KeyActive, "expired": KeyExpired, "revoked": KeyRevoked, "pending": KeyPending}, now.Add(-time.Hour), now.Add(time.Hour))
	expired := manifest.Keys["expired"]
	expired.NotBefore, expired.NotAfter = now.Add(-2*time.Hour), now.Add(-time.Hour)
	manifest.Keys["expired"] = expired
	pending := manifest.Keys["pending"]
	pending.NotBefore, pending.NotAfter = now.Add(time.Hour), now.Add(2*time.Hour)
	manifest.Keys["pending"] = pending
	f.writeManifest(t, manifest)
	set, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, Issuer: manifest.Issuer, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, kid := range []string{"expired", "revoked", "pending", "unknown"} {
		if _, err := set.VerificationKey(context.Background(), kid); err == nil {
			t.Fatalf("%s key was accepted", kid)
		}
	}
}

func TestReloadingKeySetRetainsSnapshotOnInvalidReloadAndRollback(t *testing.T) {
	now := time.Now().UTC()
	f := newManifestFixture(t, "k1", "k2")
	f.writeManifest(t, testManifest(f, 5, "k1", map[string]KeyStatus{"k1": KeyActive, "k2": KeyPending}, now.Add(-time.Hour), now.Add(time.Hour)))
	set, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, Issuer: "https://issuer.example", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	writeAtomic(t, f.manifestPath, []byte(`{"issuer":"https://issuer.example","generation":6,"unknown":true}`), 0o600)
	if err := set.Reload(context.Background()); err == nil {
		t.Fatal("invalid manifest reload succeeded")
	}
	if _, err := set.VerificationKey(context.Background(), "k1"); err != nil {
		t.Fatalf("last good snapshot was not retained: %v", err)
	}
	f.writeManifest(t, testManifest(f, 4, "k1", map[string]KeyStatus{"k1": KeyActive, "k2": KeyPending}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := set.Reload(context.Background()); err == nil {
		t.Fatal("generation rollback succeeded")
	}
	stats := set.Stats()
	if stats.Generation != 5 || stats.Successes != 1 || stats.Failures != 2 {
		t.Fatalf("Stats() = %#v", stats)
	}
}

func TestReloadingKeySetRejectsPrivatePublicMismatch(t *testing.T) {
	now := time.Now().UTC()
	f := newManifestFixture(t, "k1", "other")
	f.setPrivate(t, "other")
	f.writeManifest(t, testManifest(f, 1, "k1", map[string]KeyStatus{"k1": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if _, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, ActivePrivateKeyPath: f.privatePath, Issuer: "https://issuer.example", Now: func() time.Time { return now }}); err == nil {
		t.Fatal("mismatched active private key was accepted")
	}
}

func TestReloadingKeySetRejectsDuplicateFieldsAndUnsafePermissions(t *testing.T) {
	now := time.Now().UTC()
	f := newManifestFixture(t, "k1")
	duplicate := `{"issuer":"https://issuer.example","issuer":"https://other.example","generation":1,"activeKeyId":"k1","keys":{}}`
	writeAtomic(t, f.manifestPath, []byte(duplicate), 0o600)
	if _, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, Issuer: "https://issuer.example", Now: func() time.Time { return now }}); err == nil {
		t.Fatal("duplicate JSON field was accepted")
	}
	f.writeManifest(t, testManifest(f, 1, "k1", map[string]KeyStatus{"k1": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := os.Chmod(f.manifestPath, 0o622); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, Issuer: "https://issuer.example", Now: func() time.Time { return now }}); err == nil {
		t.Fatal("group/world-writable manifest was accepted")
	}
}

type countingManifestRecorder struct {
	calls atomic.Int64
	seen  atomic.Uint64
}

func (r *countingManifestRecorder) RecordKeyManifest(_ context.Context, manifest KeyManifest) error {
	r.calls.Add(1)
	r.seen.Store(manifest.Generation)
	return nil
}

func TestReloadingKeySetDuplicateGenerationIsIdempotentAndManifestHasNoPrivateField(t *testing.T) {
	now := time.Now().UTC()
	f := newManifestFixture(t, "k1")
	f.writeManifest(t, testManifest(f, 7, "k1", map[string]KeyStatus{"k1": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	recorder := &countingManifestRecorder{}
	set, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, Issuer: "https://issuer.example", Now: func() time.Time { return now }, Recorder: recorder})
	if err != nil {
		t.Fatal(err)
	}
	if err := set.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls.Load() != 1 || recorder.seen.Load() != 7 {
		t.Fatalf("recorder calls=%d generation=%d", recorder.calls.Load(), recorder.seen.Load())
	}
	data, _ := json.Marshal(testManifest(f, 7, "k1", map[string]KeyStatus{"k1": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if string(data) == "" || bytesContainsFold(data, []byte(`"private`)) {
		t.Fatalf("manifest exposed private material field: %s", data)
	}
}

func TestKeyReloadIntervalBounds(t *testing.T) {
	for _, interval := range []time.Duration{time.Second, 60 * time.Second} {
		if err := ValidateKeyReloadInterval(interval); err != nil {
			t.Fatalf("valid interval %s rejected: %v", interval, err)
		}
	}
	for _, interval := range []time.Duration{time.Second - 1, 60*time.Second + 1} {
		if err := ValidateKeyReloadInterval(interval); err == nil {
			t.Fatalf("invalid interval %s accepted", interval)
		}
	}
}

func TestReloadingKeySetConcurrentReloadAndVerify(t *testing.T) {
	now := time.Now().UTC()
	f := newManifestFixture(t, "k1", "k2")
	f.writeManifest(t, testManifest(f, 1, "k1", map[string]KeyStatus{"k1": KeyActive, "k2": KeyPending}, now.Add(-time.Hour), now.Add(time.Hour)))
	set, err := LoadReloadingKeySet(context.Background(), ReloadingKeySetConfig{ManifestPath: f.manifestPath, Issuer: "https://issuer.example", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = set.VerificationKey(context.Background(), "k1")
				_, _ = set.VerificationKey(context.Background(), "k2")
			}
		}()
	}
	f.writeManifest(t, testManifest(f, 2, "k2", map[string]KeyStatus{"k1": KeyRetiring, "k2": KeyActive}, now.Add(-time.Hour), now.Add(time.Hour)))
	if err := set.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

func bytesContainsFold(value, needle []byte) bool {
	if len(needle) == 0 || len(value) < len(needle) {
		return false
	}
	for i := 0; i <= len(value)-len(needle); i++ {
		match := true
		for j := range needle {
			a, b := value[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

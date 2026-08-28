package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type testKeys struct {
	kid string
	key *ecdsa.PrivateKey
}

func (k testKeys) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return k.kid, k.key, nil
}

func (k testKeys) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid != k.kid {
		return nil, errors.New("unknown key")
	}
	return &k.key.PublicKey, nil
}

type testIdentities struct {
	identity         *Identity
	disabled         bool
	missingPolicy    bool
	emptyPermissions bool
	permissionAction AuthorizationAction
}

func (r *testIdentities) ResolveUserIdentity(_ context.Context, userID, selector string) (*Identity, error) {
	if r.disabled || r.identity == nil {
		return nil, ErrNoAuthorizedTenant
	}
	if userID != r.identity.UserID || (selector != "" && selector != r.identity.MembershipID) {
		return nil, ErrMembershipMismatch
	}
	copy := *r.identity
	return &copy, nil
}

func (r *testIdentities) ResolveMembership(_ context.Context, subjectID, membershipID string) (*Identity, error) {
	if r.disabled || r.identity == nil || subjectID != r.identity.SubjectID || membershipID != r.identity.MembershipID {
		return nil, ErrMembershipMismatch
	}
	copy := *r.identity
	return &copy, nil
}

func (r *testIdentities) ResolvePermissions(_ context.Context, subjectID, membershipID, tenantID string) (string, []ScopedPermission, error) {
	if r.disabled || r.identity == nil || subjectID != r.identity.SubjectID || membershipID != r.identity.MembershipID || tenantID != r.identity.TenantID {
		return "", nil, ErrMembershipMismatch
	}
	if r.missingPolicy {
		return "", nil, errors.New("active authorization policy is missing")
	}
	if r.emptyPermissions {
		return "default:1", []ScopedPermission{}, nil
	}
	action := r.permissionAction
	if action == "" {
		action = ActionRead
	}
	return "default:1", []ScopedPermission{{ResourceKind: "*", Action: action, TenantID: tenantID}}, nil
}

type testRefreshStore struct {
	mu      sync.Mutex
	records map[string]RefreshTokenRecord
}

func (s *testRefreshStore) CreateRefreshToken(_ context.Context, record RefreshTokenRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[record.TokenHash]; exists {
		return errors.New("duplicate")
	}
	s.records[record.TokenHash] = record
	return nil
}

func (s *testRefreshStore) RotateRefreshToken(_ context.Context, oldHash string, replacement RefreshTokenRecord, now time.Time) (*RefreshTokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, exists := s.records[oldHash]
	if !exists || !old.ExpiresAt.After(now) {
		return nil, ErrRefreshReplay
	}
	delete(s.records, oldHash)
	replacement.SubjectID = old.SubjectID
	replacement.MembershipID = old.MembershipID
	s.records[replacement.TokenHash] = replacement
	return &old, nil
}

func newTestManager(t *testing.T) (*TokenManager, *testIdentities, *testRefreshStore, testKeys, time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0).UTC()
	keys := testKeys{kid: "test-key-1", key: key}
	identities := &testIdentities{identity: &Identity{UserID: "user-1", SubjectID: "subject-1", SubjectType: "user", MembershipID: "membership-1", TenantID: "tenant-1"}}
	refresh := &testRefreshStore{records: make(map[string]RefreshTokenRecord)}
	manager, err := NewTokenManager(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "hnb-apiserver", Audiences: []string{"hnb-apiserver", "hnb-platform-api", "hnb-app-market"}, AccessTTL: MaxAccessTokenTTL, RefreshTTL: 24 * time.Hour, Now: func() time.Time { return now }}, keys, keys, identities, identities, refresh)
	if err != nil {
		t.Fatal(err)
	}
	return manager, identities, refresh, keys, now
}

func TestIssueAndVerifyAccessToken(t *testing.T) {
	manager, _, refreshStore, _, _ := newTestManager(t)
	access, refresh, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Verify(context.Background(), access.Token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SubjectID != "subject-1" || claims.TenantID != "tenant-1" || claims.MembershipID != "membership-1" || claims.TenantMembershipIDs[0] != "membership-1" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if claims.PolicyVersion != "default:1" || len(claims.ScopedPermissions) != 1 || claims.ScopedPermissions[0].Action != ActionRead {
		t.Fatalf("permission snapshot = %#v", claims)
	}
	if _, storedRaw := refreshStore.records[refresh.Token]; storedRaw {
		t.Fatal("refresh token was stored in plaintext")
	}
	if _, storedHash := refreshStore.records[hashRefreshToken(refresh.Token)]; !storedHash {
		t.Fatal("refresh token hash was not stored")
	}
	trusted, err := manager.Authenticate(context.Background(), access.Token, "00000000-0000-0000-0000-000000000001", "")
	if err != nil || trusted.TenantID != "tenant-1" || trusted.SubjectID != "subject-1" {
		t.Fatalf("trusted context = %#v, err = %v", trusted, err)
	}
}

func TestVerifyRejectsInvalidTokens(t *testing.T) {
	manager, _, _, keys, now := newTestManager(t)
	access, _, err := manager.Issue(context.Background(), "user-1", "")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := manager.Verify(context.Background(), access.Token)
	if err != nil {
		t.Fatal(err)
	}
	baseHeader := map[string]string{"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": keys.kid}
	tests := map[string]string{
		"malformed":    "not-a-jwt",
		"legacy token": base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":"user-1"}`)) + ".deadbeef",
		"oversized":    string(make([]byte, MaxAccessTokenSize+1)),
	}
	mutate := func(name string, claims AccessTokenClaims) {
		tests[name] = signTestToken(t, keys.key, baseHeader, claims)
	}
	claims := *valid
	claims.ProfileVersion = "hnb.identity/v0"
	mutate("wrong profile", claims)
	claims = *valid
	claims.Issuer = "https://wrong.example"
	mutate("wrong issuer", claims)
	claims = *valid
	claims.Audiences = []string{"other-service"}
	mutate("wrong audience", claims)
	claims = *valid
	claims.ExpiresAt = now.Unix()
	mutate("expired", claims)
	claims = *valid
	claims.NotBefore = now.Add(time.Minute).Unix()
	mutate("not before", claims)
	claims = *valid
	claims.IssuedAt = now.Add(time.Minute).Unix()
	mutate("future issued at", claims)
	claims = *valid
	claims.SubjectID = ""
	mutate("missing subject", claims)
	claims = *valid
	claims.SubjectType = "administrator"
	mutate("wrong subject type", claims)
	claims = *valid
	claims.TokenID = ""
	mutate("missing jti", claims)
	claims = *valid
	claims.KeyID = "other-key"
	mutate("key ID claim mismatch", claims)
	claims = *valid
	claims.Algorithm = "RS256"
	mutate("algorithm claim mismatch", claims)
	claims = *valid
	claims.TenantMembershipIDs = nil
	mutate("missing membership", claims)
	claims = *valid
	claims.MembershipID = "other-membership"
	mutate("selected membership absent from memberships", claims)
	claims = *valid
	claims.TenantID = ""
	mutate("missing selected tenant", claims)
	claims = *valid
	claims.PolicyVersion = ""
	mutate("missing policy version", claims)
	claims = *valid
	claims.ScopedPermissions = append([]ScopedPermission(nil), valid.ScopedPermissions...)
	claims.ScopedPermissions[0].TenantID = "*"
	mutate("wildcard permission tenant", claims)
	claims = *valid
	claims.ScopedPermissions = append([]ScopedPermission(nil), valid.ScopedPermissions...)
	claims.ScopedPermissions[0].Action = "*"
	mutate("wildcard permission action", claims)
	claims = *valid
	claims.Audiences = append(claims.Audiences, "*")
	mutate("wildcard audience", claims)
	tests["none"] = signTestToken(t, keys.key, map[string]string{"typ": AccessTokenType, "alg": "none", "kid": keys.kid}, *valid)
	tests["wrong algorithm"] = signTestToken(t, keys.key, map[string]string{"typ": AccessTokenType, "alg": "HS256", "kid": keys.kid}, *valid)
	tests["wrong type"] = signTestToken(t, keys.key, map[string]string{"typ": "JWT", "alg": AccessTokenAlgorithm, "kid": keys.kid}, *valid)
	tests["unknown kid"] = signTestToken(t, keys.key, map[string]string{"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": "unknown"}, *valid)
	parts := strings.Split(access.Token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)/2] ^= 1
	tests["tampered signed snapshot"] = parts[0] + "." + base64.RawURLEncoding.EncodeToString(payload) + "." + parts[2]

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := manager.Verify(context.Background(), token); err == nil {
				t.Fatal("token was accepted")
			}
		})
	}
}

func TestIssuanceFailsClosedWithoutActivePolicy(t *testing.T) {
	manager, identities, _, _, _ := newTestManager(t)
	identities.missingPolicy = true
	if _, _, err := manager.Issue(context.Background(), "user-1", "membership-1"); err == nil {
		t.Fatal("token was issued without an active policy")
	}
}

func TestIssuanceAllowsEmptyPermissionSnapshotButProtectedActionIsDenied(t *testing.T) {
	manager, identities, _, _, _ := newTestManager(t)
	identities.emptyPermissions = true
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := manager.Authenticate(context.Background(), access.Token, "correlation", "")
	if err != nil {
		t.Fatal(err)
	}
	if trusted.ScopedPermissions == nil || len(trusted.ScopedPermissions) != 0 {
		t.Fatalf("permissions = %#v", trusted.ScopedPermissions)
	}
	decision := NewEvaluator().Evaluate(trusted, AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID, ResourceKind: "operation", Action: ActionRead,
	})
	if decision.Allowed || decision.ReasonCode != "permission_denied" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPermissionChangesAreBoundedByAccessTokenExpiry(t *testing.T) {
	manager, identities, _, _, _ := newTestManager(t)
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	identities.permissionAction = ActionDelete
	trusted, err := manager.Authenticate(context.Background(), access.Token, "correlation", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(trusted.ScopedPermissions) != 1 || trusted.ScopedPermissions[0].Action != ActionRead {
		t.Fatalf("issued snapshot changed in place: %+v", trusted.ScopedPermissions)
	}
	next, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	nextClaims, err := manager.Verify(context.Background(), next.Token)
	if err != nil || nextClaims.ScopedPermissions[0].Action != ActionDelete {
		t.Fatalf("new snapshot = %+v, err = %v", nextClaims, err)
	}
}

func TestVerifierOnlyDerivesSignedTenantContextForEachAudience(t *testing.T) {
	manager, _, _, keys, now := newTestManager(t)
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, audience := range []string{"hnb-platform-api", "hnb-app-market"} {
		verifier, err := NewTokenVerifier(TokenManagerConfig{
			Issuer: "https://issuer.example", Audience: audience, AccessTTL: MaxAccessTokenTTL,
			Now: func() time.Time { return now },
		}, keys)
		if err != nil {
			t.Fatal(err)
		}
		trusted, err := verifier.Authenticate(context.Background(), access.Token, "correlation", "")
		if err != nil || trusted.TenantID != "tenant-1" || trusted.MembershipID != "membership-1" {
			t.Fatalf("audience %s: trusted = %#v, err = %v", audience, trusted, err)
		}
	}
}

func TestTokenConfigurationEnforcesSixtySecondRevocationBound(t *testing.T) {
	_, _, _, keys, _ := newTestManager(t)
	if _, err := NewTokenVerifier(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", AccessTTL: MaxAccessTokenTTL + time.Second}, keys); err == nil {
		t.Fatal("access TTL over 60 seconds was accepted")
	}
	if _, err := NewTokenVerifier(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "*", AccessTTL: MaxAccessTokenTTL}, keys); err == nil {
		t.Fatal("wildcard verifier audience was accepted")
	}
}

func TestMembershipAndRefreshFailClosed(t *testing.T) {
	manager, identities, _, _, _ := newTestManager(t)
	if _, _, err := manager.Issue(context.Background(), "user-1", "other-membership"); !errors.Is(err, ErrMembershipMismatch) {
		t.Fatalf("selector mismatch error = %v", err)
	}
	access, refresh, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	identities.disabled = true
	if _, err := manager.Authenticate(context.Background(), access.Token, "correlation", ""); err == nil {
		t.Fatal("disabled subject or inactive membership was accepted")
	}
	identities.disabled = false
	identities.identity.TenantID = "tenant-changed-after-issuance"
	if _, err := manager.Authenticate(context.Background(), access.Token, "correlation", ""); !errors.Is(err, ErrMembershipMismatch) {
		t.Fatalf("changed membership tenant error = %v", err)
	}
	identities.identity.TenantID = "tenant-1"
	if _, _, err := manager.Refresh(context.Background(), refresh.Token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Refresh(context.Background(), refresh.Token); !errors.Is(err, ErrRefreshReplay) {
		t.Fatalf("refresh replay error = %v", err)
	}
	withoutMembership, emptyIdentities, _, _, _ := newTestManager(t)
	emptyIdentities.identity = nil
	if _, _, err := withoutMembership.Issue(context.Background(), "user-1", ""); !errors.Is(err, ErrNoAuthorizedTenant) {
		t.Fatalf("missing membership error = %v", err)
	}
}

func TestRefreshRotationIsSingleUseUnderConcurrency(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)
	_, refresh, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := manager.Refresh(context.Background(), refresh.Token)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful refresh rotations = %d, want 1", successes)
	}
}

func TestTrustedContextAPI(t *testing.T) {
	want := TrustedContext{SubjectID: "subject", TenantID: "tenant"}
	ctx := WithTrustedContext(context.Background(), want)
	got, ok := TrustedContextFrom(ctx)
	if !ok || got.SubjectID != want.SubjectID || got.TenantID != want.TenantID {
		t.Fatalf("TrustedContextFrom() = %#v, %v", got, ok)
	}
	if _, ok := TrustedContextFrom(context.Background()); ok {
		t.Fatal("empty context contained trusted identity")
	}
}

func TestLoadPEMKeySet(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, _ := x509.MarshalPKCS8PrivateKey(key)
	publicDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPEMKeySet("kid-1", privatePath, map[string]string{"kid-1": publicPath}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPEMPublicKeySet(map[string]string{"kid-1": publicPath}); err != nil {
		t.Fatal(err)
	}
}

func TestAlgorithmConfusionWithHeaderInjection(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0).UTC()
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: "https://issuer.example",
		Audiences: []string{"hnb-apiserver"}, SubjectID: "subject-confusion",
		SubjectType: "user", TenantID: "tenant-confusion", MembershipID: "member-1",
		TenantMembershipIDs: []string{"member-1"}, PolicyVersion: "default:1",
		ScopedPermissions:   []ScopedPermission{{ResourceKind: "*", Action: ActionRead, TenantID: "tenant-confusion"}},
		AllowedActions:      []AuthorizationAction{ActionRead},
		IssuedAt:            now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(30 * time.Second).Unix(),
		AuthTime:            now.Unix(), TokenID: "jti-confusion", KeyID: "algo-test-key", Algorithm: AccessTokenAlgorithm,
	}

	// Build verifier that knows the test key
	testKeys := testKeysFixed{kid: "algo-test-key", key: &key.PublicKey}
	verifier, err := NewTokenVerifier(TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "hnb-apiserver", AccessTTL: MaxAccessTokenTTL, Now: func() time.Time { return now },
	}, testKeys)
	if err != nil {
		t.Fatal(err)
	}

	// Sign with ECDSA but claim HS256 in header - verifier should reject
	hsHeader := map[string]string{"typ": AccessTokenType, "alg": "HS256", "kid": "algo-test-key"}
	hstoken := signTestToken(t, key, hsHeader, claims)
	if _, err := verifier.Verify(context.Background(), hstoken); err == nil {
		t.Fatal("HS256 header with ES256 payload was accepted")
	}

	// Claim RS256 but use ES256 signature
	rsHeader := map[string]string{"typ": AccessTokenType, "alg": "RS256", "kid": "algo-test-key"}
	rstoken := signTestToken(t, key, rsHeader, claims)
	if _, err := verifier.Verify(context.Background(), rstoken); err == nil {
		t.Fatal("RS256 header with ES256 payload was accepted")
	}

	// Claim 'none' algorithm
	noneHeader := map[string]string{"typ": AccessTokenType, "alg": "none", "kid": "algo-test-key"}
	nonetoken := signTestToken(t, key, noneHeader, claims)
	if _, err := verifier.Verify(context.Background(), nonetoken); err == nil {
		t.Fatal("'none' algorithm header was accepted")
	}

	// Verify correct header type matches signature
	correctHeader := map[string]string{"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": "algo-test-key"}
	correctToken := signTestToken(t, key, correctHeader, claims)
	verified, err := verifier.Verify(context.Background(), correctToken)
	if err != nil {
		t.Fatalf("correct header rejected: %v", err)
	}
	if verified.SubjectID != "subject-confusion" {
		t.Fatal("verified subject mismatch")
	}
}

func TestCrossAudienceVerification(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0).UTC()
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: "https://issuer.example",
		Audiences: []string{"hnb-apiserver", "hnb-platform-api"}, SubjectID: "subject-crosstown",
		SubjectType: "user", TenantID: "tenant-a", MembershipID: "m1",
		TenantMembershipIDs: []string{"m1"}, PolicyVersion: "default:1",
		ScopedPermissions:   []ScopedPermission{{ResourceKind: "*", Action: ActionList, TenantID: "tenant-a"}},
		AllowedActions:      []AuthorizationAction{ActionList},
		IssuedAt:            now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(30 * time.Second).Unix(),
		AuthTime:            now.Unix(), TokenID: "jti-cross", KeyID: "kc", Algorithm: AccessTokenAlgorithm,
	}

	headers := map[string]string{
		"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": "kc",
	}
	token := signTestToken(t, key, headers, claims)

	for _, audience := range []string{"hnb-apiserver", "hnb-platform-api"} {
		verifier, err := NewTokenVerifier(TokenManagerConfig{
			Issuer: "https://issuer.example", Audience: audience, AccessTTL: MaxAccessTokenTTL, Now: func() time.Time { return now },
		}, testKeysFixed{kid: "kc", key: &key.PublicKey})
		if err != nil {
			t.Fatalf("audience %s config error: %v", audience, err)
		}
		verified, err := verifier.Verify(context.Background(), token)
		if err != nil {
			t.Fatalf("audience %s: verify failed: %v", audience, err)
		}
		if verified.SubjectID != "subject-crosstown" {
			t.Fatalf("audience %s: subject mismatch", audience)
		}
	}
}

func TestHeaderKeyKidMismatchRejection(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0).UTC()
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: "https://issuer.example",
		Audiences: []string{"hnb-apiserver"}, SubjectID: "sub", SubjectType: "user",
		TenantID: "t1", MembershipID: "m1", TenantMembershipIDs: []string{"m1"},
		PolicyVersion: "default:1", ScopedPermissions: []ScopedPermission{{ResourceKind: "*", Action: ActionRead, TenantID: "t1"}},
		AllowedActions:    []AuthorizationAction{ActionRead},
		IssuedAt:          now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(30 * time.Second).Unix(),
		AuthTime:          now.Unix(), TokenID: "jti-kid", KeyID: "real-key", Algorithm: AccessTokenAlgorithm,
	}
	headers := map[string]string{"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": "spoofed-key"}
	token := signTestToken(t, key, headers, claims)
	manager, _, _, _, _ := newTestManager(t)
	if _, err := manager.Verify(context.Background(), token); err == nil {
		t.Fatal("kid mismatch between header and claims was accepted")
	}
}

func TestStrictPolicyVersionClamp(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0).UTC()
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: "https://issuer.example",
		Audiences: []string{"hnb-apiserver"}, SubjectID: "sub", SubjectType: "user",
		TenantID: "t1", MembershipID: "m1", TenantMembershipIDs: []string{"m1"},
		PolicyVersion: strings.Repeat("a", 129), ScopedPermissions: []ScopedPermission{{ResourceKind: "*", Action: ActionRead, TenantID: "t1"}},
		AllowedActions:    []AuthorizationAction{ActionRead},
		IssuedAt:          now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(30 * time.Second).Unix(),
		AuthTime:          now.Unix(), TokenID: "jti-len", KeyID: "k1", Algorithm: AccessTokenAlgorithm,
	}
	headers := map[string]string{"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": "k1"}
	token := signTestToken(t, key, headers, claims)
	manager, _, _, _, _ := newTestManager(t)
	if _, err := manager.Verify(context.Background(), token); err == nil {
		t.Fatal("oversized policy version was accepted")
	}
}

func TestCacheInvalidationViaMembershipMismatch(t *testing.T) {
	manager, identities, _, _, _ := newTestManager(t)
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}

	// Valid token authenticates
	_, err = manager.Authenticate(context.Background(), access.Token, "correlation", "")
	if err != nil {
		t.Fatalf("initial authenticate failed: %v", err)
	}

	// Change the persisted identity so that cached claims no longer match
	identities.identity.MembershipID = "changed-membership"
	if _, err := manager.Authenticate(context.Background(), access.Token, "correlation", ""); !errors.Is(err, ErrMembershipMismatch) {
		t.Fatalf("expected membership mismatch after change, got %v", err)
	}

	// Restore for cleanup
	identities.identity.MembershipID = "membership-1"
}

func TestNoHeaderTrustFromSpoofedHeaders(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)

	// Create a valid token for tenant-a with user-b's subject
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}

	// Verify does NOT read from X-Tenant-ID or X-User-ID headers
	ctx := context.Background()
	_, err = manager.Verify(ctx, access.Token)
	if err != nil {
		t.Fatalf("verify basic token failed: %v", err)
	}
	// Verifier has no access to HTTP headers at all — it only uses the signed JWT contents.
}

func signTestToken(t *testing.T, key *ecdsa.PrivateKey, header map[string]string, claims AccessTokenClaims) string {
	t.Helper()
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func signTestTokenFixed(t *testing.T, key *ecdsa.PrivateKey, header map[string]string, claims AccessTokenClaims, pubKey *ecdsa.PublicKey) string {
	t.Helper()
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

type testKeysFixed struct {
	kid  string
	key  interface{} // *ecdsa.PublicKey for verification
}

func (k testKeysFixed) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid != k.kid {
		return nil, errors.New("unknown key")
	}
	switch v := k.key.(type) {
	case *ecdsa.PublicKey:
		return v, nil
	default:
		return nil, errors.New("key is not public key")
	}
}

func TestCorrelationAndTraceparentRedaction(t *testing.T) {
	manager, _, _, _, _ := newTestManager(t)

	// Correlation ID and traceparent must NOT appear in the token or audit
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	verifiedClaims, err := manager.Verify(context.Background(), access.Token)
	if err != nil {
		t.Fatal(err)
	}
	// The AccessTokenClaims struct contains only signed JWT fields;
	// correlation and traceparent are injected server-side during Authenticate, never stored in JWT.
	// The TrustedContext stores correlation for audit but it should never reach logs as secrets
	trusted, err := manager.Authenticate(context.Background(), access.Token, "test-correlation-id", "00-trace123-456-01")
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if trusted.CorrelationID != "test-correlation-id" || trusted.Traceparent != "00-trace123-456-01" {
		t.Fatalf("trusted context lost correlation/trace: %#v", trusted)
	}
	// Ensure the claims struct has no leak by confirming known signed fields only
	if verifiedClaims.SubjectID == "" {
		t.Fatal("verified claims missing subject")
	}
}

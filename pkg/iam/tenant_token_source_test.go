package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type tenantTestIssuer struct {
	mu    sync.Mutex
	scope ServiceCredentialScope
	kid   string
	key   *ecdsa.PrivateKey
	now   time.Time
	calls int
}

func (i *tenantTestIssuer) IssueServiceCredential(ctx context.Context, scope ServiceCredentialScope) (string, error) {
	i.mu.Lock()
	i.calls++
	i.mu.Unlock()
	if scope != i.scope {
		return "", ErrServiceCredentialRejected
	}
	membership := "service:" + scope.SubjectID
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: "https://issuer.example",
		Audiences: []string{scope.Audience}, SubjectID: scope.SubjectID, SubjectType: "service",
		TenantID: scope.Permission.TenantID, MembershipID: membership,
		TenantMembershipIDs: []string{membership}, PolicyVersion: "serviceIdentity:1",
		ScopedPermissions: []ScopedPermission{scope.Permission},
		AllowedActions:    []AuthorizationAction{scope.Permission.Action},
		IssuedAt:          i.now.Unix(), NotBefore: i.now.Unix(), ExpiresAt: i.now.Add(30 * time.Second).Unix(),
		AuthTime: i.now.Unix(), TokenID: uuid.NewString(), KeyID: i.kid, Algorithm: AccessTokenAlgorithm,
	}
	return signAccessToken(i.key, i.kid, claims)
}

func (i *tenantTestIssuer) Calls() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.calls
}

func TestTenantRenewingTokenSourceIsolatesTenantsAndCloses(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := testKeys{kid: "test-key-1", key: key}
	now := time.Unix(2_000_000_000, 0).UTC()
	subjectID := uuid.NewString()
	var mu sync.Mutex
	issuers := make(map[string]*tenantTestIssuer)

	source, err := NewTenantRenewingTokenSource(context.Background(), func(tenantID string) (*TokenVerifier, ServiceCredentialIssuer, ServiceCredentialScope, error) {
		scope := ServiceCredentialScope{
			SubjectID: subjectID, Audience: "provider-audience",
			Permission: ScopedPermission{ResourceKind: "runtimeProvider", ResourceID: "provider-a", Action: ActionExecute, TenantID: tenantID},
		}
		issuer := &tenantTestIssuer{scope: scope, kid: keys.kid, key: keys.key, now: now}
		mu.Lock()
		issuers[tenantID] = issuer
		mu.Unlock()
		verifier, err := NewTokenVerifier(TokenManagerConfig{
			Issuer: "https://issuer.example", Audience: scope.Audience,
			AccessTTL: MaxAccessTokenTTL, Now: func() time.Time { return now },
		}, keys)
		return verifier, issuer, scope, err
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	tokens := make(chan string, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := source.TokenForTenant(context.Background(), "tenant-a")
			if err != nil {
				t.Errorf("tenant-a token: %v", err)
				return
			}
			tokens <- token
		}()
	}
	wg.Wait()
	close(tokens)
	if len(tokens) != 100 {
		t.Fatalf("expected 100 tokens, got %d", len(tokens))
	}
	mu.Lock()
	tenantAIssuer := issuers["tenant-a"]
	mu.Unlock()
	if tenantAIssuer == nil || tenantAIssuer.Calls() != 1 {
		t.Fatalf("cold-start issuance was not coalesced: %+v", tenantAIssuer)
	}

	tokenB, err := source.TokenForTenant(context.Background(), "tenant-b")
	if err != nil {
		t.Fatal(err)
	}
	if tokenB == "" {
		t.Fatal("tenant-b token is empty")
	}
	mu.Lock()
	tenantBIssuer := issuers["tenant-b"]
	mu.Unlock()
	if tenantBIssuer == nil || tenantBIssuer.scope.Permission.TenantID != "tenant-b" || tenantBIssuer.Calls() != 1 {
		t.Fatal("tenant-b did not receive an isolated source")
	}

	for _, invalid := range []string{"", "*"} {
		if _, err := source.TokenForTenant(context.Background(), invalid); !errors.Is(err, ErrServiceCredentialRejected) {
			t.Fatalf("invalid tenant %q was accepted: %v", invalid, err)
		}
	}

	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := source.TokenForTenant(context.Background(), "tenant-a"); !errors.Is(err, ErrServiceCredentialUnavailable) {
		t.Fatalf("closed source returned a token: %v", err)
	}
}

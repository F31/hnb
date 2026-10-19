package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeAgentSessionIssuer struct {
	mu              sync.Mutex
	signer          *AgentTunnelTokenSigner
	tenantID        string
	targetID        string
	sessionID       string
	generation      int64
	ttl             time.Duration
	exchangeCalls   int
	renewCalls      int
	renewErr        error
	observerVersion int
}

func (f *fakeAgentSessionIssuer) ExchangeAgentSession(ctx context.Context, secret, tenantID, targetID string) (AgentSessionCredential, error) {
	f.mu.Lock()
	f.exchangeCalls++
	f.mu.Unlock()
	if secret != "single-use-secret" || tenantID != f.tenantID || targetID != f.targetID {
		return AgentSessionCredential{}, ErrServiceCredentialRejected
	}
	return f.issue(ctx)
}

func (f *fakeAgentSessionIssuer) RenewAgentSession(ctx context.Context, _ string) (AgentSessionCredential, error) {
	f.mu.Lock()
	f.renewCalls++
	err := f.renewErr
	f.mu.Unlock()
	if err != nil {
		return AgentSessionCredential{}, err
	}
	return f.issue(ctx)
}

func (f *fakeAgentSessionIssuer) issue(ctx context.Context) (AgentSessionCredential, error) {
	token, expiry, err := f.signer.SignSession(ctx, f.tenantID, f.targetID, f.sessionID, f.generation, f.ttl)
	if err != nil {
		return AgentSessionCredential{}, err
	}
	f.observerVersion++
	return AgentSessionCredential{
		Credential: token, ExpiresAt: expiry, SessionID: f.sessionID, Generation: f.generation,
		TenantID: f.tenantID, TargetID: f.targetID, Subject: "cluster-agent",
		ObserverToken: fmt.Sprintf("observer-token-%d", f.observerVersion), ObserverTokenExpiresAt: expiry,
	}, nil
}

func newAgentSessionSourceFixture(t *testing.T) (*AgentSessionTokenSource, *fakeAgentSessionIssuer, *renewalTestClock, *AgentTunnelTokenVerifier) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := testKeys{kid: "test-key-1", key: key}
	now := time.Unix(2_000_000_000, 0).UTC()
	clock := &renewalTestClock{now: now, scheduled: make(chan time.Duration, 128)}
	config := AgentTunnelTokenConfig{
		Issuer: "https://identity.hnb.cloud", Audience: "hnb-apiserver-tunnel",
		TTL: MaxAgentTunnelTokenTTL, Now: clock.Now,
	}
	signer, err := NewAgentTunnelTokenSigner(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewAgentTunnelTokenVerifier(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	issuer := &fakeAgentSessionIssuer{
		signer: signer, tenantID: "tenant-a", targetID: "target-a",
		sessionID: uuid.NewString(), generation: 1, ttl: 30 * time.Second,
	}
	source, err := NewAgentSessionTokenSource(context.Background(), verifier, issuer, "tenant-a", "target-a", "single-use-secret")
	if err != nil {
		t.Fatal(err)
	}
	source.clock = clock
	return source, issuer, clock, verifier
}

func TestAgentSessionTokenSourceRotatesAtHalfLifeAndFailsClosed(t *testing.T) {
	source, issuer, clock, verifier := newAgentSessionSourceFixture(t)
	defer source.Close()

	first, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.VerifyClaims(context.Background(), first, "tenant-a", "target-a"); err != nil {
		t.Fatal(err)
	}
	if delay := nextRenewalDelay(t, clock); delay != 15*time.Second {
		t.Fatalf("renewal delay = %v, want 15s", delay)
	}
	clock.Advance(15 * time.Second)
	if delay := nextRenewalDelay(t, clock); delay != 15*time.Second {
		t.Fatalf("post-renewal delay = %v, want 15s", delay)
	}
	if _, err := source.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	issuer.mu.Lock()
	renewCalls := issuer.renewCalls
	issuer.mu.Unlock()
	if renewCalls != 1 {
		t.Fatalf("renew calls = %d, want 1", renewCalls)
	}

	issuer.mu.Lock()
	issuer.renewErr = ErrServiceIdentityRevoked
	issuer.mu.Unlock()
	clock.Advance(15 * time.Second)
	clock.Advance(30 * time.Second)
	if _, err := source.Token(context.Background()); !errors.Is(err, ErrServiceIdentityRevoked) {
		t.Fatalf("revoked session returned a token: %v", err)
	}
}

func TestAgentSessionTokenSourceStopsOnExchangeRejection(t *testing.T) {
	_, issuer, _, verifier := newAgentSessionSourceFixture(t)
	source, err := NewAgentSessionTokenSource(context.Background(), verifier, issuer, "tenant-a", "target-a", "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.Token(context.Background()); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatalf("rejected enrollment was accepted: %v", err)
	}
}

func TestHTTPAgentSessionIssuerExchangeAndRenew(t *testing.T) {
	now := time.Unix(2_000_000_000, 0).UTC()
	sessionID := uuid.NewString()
	var mu sync.Mutex
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		credential := "credential-1"
		if r.URL.Path == "/api/v1/agent-sessions:renew" {
			credential = "credential-2"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"credential": credential, "expiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
			"sessionId": sessionID, "generation": 1,
			"tenantId": "tenant-a", "targetId": "target-a", "subject": "cluster-agent",
			"observerToken": "observer-" + credential, "observerTokenExpiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
		})
	}))
	defer server.Close()
	issuer, err := NewHTTPAgentSessionIssuer(HTTPAgentSessionIssuerConfig{BaseURL: server.URL, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	exchanged, err := issuer.ExchangeAgentSession(context.Background(), "single-use-secret", "tenant-a", "target-a")
	if err != nil || exchanged.Credential != "credential-1" || exchanged.SessionID != sessionID || exchanged.Generation != 1 {
		t.Fatalf("exchange = %+v err=%v", exchanged, err)
	}
	if exchanged.ObserverToken != "observer-credential-1" || !exchanged.ObserverTokenExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("exchange observer token = %+v", exchanged)
	}
	renewed, err := issuer.RenewAgentSession(context.Background(), "credential-1")
	if err != nil || renewed.Credential != "credential-2" {
		t.Fatalf("renew = %+v err=%v", renewed, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || requests[1] != "/api/v1/agent-sessions:renew Bearer credential-1" {
		t.Fatalf("unexpected requests: %v", requests)
	}
}

func TestHTTPAgentSessionIssuerMapsPermanentAndTransientFailures(t *testing.T) {
	now := time.Unix(2_000_000_000, 0).UTC()
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: ErrServiceCredentialRejected},
		{name: "server failure", status: http.StatusInternalServerError, want: ErrServiceCredentialUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("sensitive-server-body"))
			}))
			defer server.Close()
			issuer, _ := NewHTTPAgentSessionIssuer(HTTPAgentSessionIssuerConfig{BaseURL: server.URL, Now: func() time.Time { return now }})
			_, err := issuer.ExchangeAgentSession(context.Background(), "single-use-secret", "tenant-a", "target-a")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "sensitive-server-body") || strings.Contains(err.Error(), "single-use-secret") {
				t.Fatalf("error leaked secret material: %v", err)
			}
		})
	}
}

func TestFileAgentSessionStoreRoundTripAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	credential := AgentSessionCredential{
		Credential: "credential", ExpiresAt: time.Now().Add(time.Minute).UTC(),
		SessionID: uuid.NewString(), Generation: 2, TenantID: "tenant-a", TargetID: "target-a", Subject: "cluster-agent",
	}
	store := FileAgentSessionStore{}
	if err := store.Save(path, credential); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session file mode = %o, want 600", info.Mode().Perm())
	}
	loaded, ok, err := store.Load(path)
	if err != nil || !ok || loaded.SessionID != credential.SessionID || loaded.Generation != 2 {
		t.Fatalf("load = %+v ok=%v err=%v", loaded, ok, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(path); err == nil {
		t.Fatal("group-readable session file was accepted")
	}
}

func TestAgentSessionTokenSourceResumesFromPersistedSession(t *testing.T) {
	source, issuer, clock, verifier := newAgentSessionSourceFixture(t)
	path := filepath.Join(t.TempDir(), "session.json")
	source.ConfigurePersistence(FileAgentSessionStore{}, path)
	first, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if delay := nextRenewalDelay(t, clock); delay <= 0 {
		t.Fatal("source did not reach its renewal wait")
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	resumed, err := NewAgentSessionTokenSource(context.Background(), verifier, issuer, "tenant-a", "target-a", "already-consumed-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	resumed.clock = clock
	resumed.ConfigurePersistence(FileAgentSessionStore{}, path)
	got, err := resumed.Token(context.Background())
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if got != first {
		t.Fatal("resumed source did not use the persisted session")
	}
	issuer.mu.Lock()
	exchanges := issuer.exchangeCalls
	issuer.mu.Unlock()
	if exchanges != 1 {
		t.Fatalf("resume performed another exchange: %d", exchanges)
	}
}

func TestAgentSessionTokenSourceWritesRotatedObserverToken(t *testing.T) {
	source, _, clock, _ := newAgentSessionSourceFixture(t)
	defer source.Close()
	dir := t.TempDir()
	source.ConfigureObserverTokenFile(filepath.Join(dir, "observer-token"))
	if _, err := source.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "observer-token"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "observer-token-1" {
		t.Fatalf("observer token = %q", first)
	}
	info, err := os.Stat(filepath.Join(dir, "observer-token"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("observer token mode = %v, err=%v", info.Mode().Perm(), err)
	}
	clock.Advance(15 * time.Second)
	if _, err := source.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		second, err := os.ReadFile(filepath.Join(dir, "observer-token"))
		if err != nil {
			t.Fatal(err)
		}
		if string(second) == "observer-token-2" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rotated observer token = %q", second)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

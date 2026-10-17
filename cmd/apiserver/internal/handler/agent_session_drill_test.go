//go:build drill

package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/tunnel"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// TestAgentSessionIdentityDrill is the 2.8 process-level drill: real HTTP,
// websocket, PostgreSQL, the actual tunnel server, and the Agent session
// source. It crosses more than two session TTLs, restarts the Agent source
// from the persisted session, revokes the identity, and measures propagation.
// Timings are reported by the test log and summarized in the acceptance report.
func TestAgentSessionIdentityDrill(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HNB_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	setupDrillSchema(t, db)

	store := iam.NewServiceIdentityDBStore(db)
	tenantID, targetID := "drill-tenant", "drill-target"
	targets := drillTargetResolver{tenantID: tenantID, targetID: targetID}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := handlerTestKeys{key: key}
	now := time.Now
	config := iam.AgentTunnelTokenConfig{
		Issuer: "https://identity.hnb.cloud", Audience: "hnb-apiserver-tunnel",
		TTL: iam.MaxAgentTunnelTokenTTL, Now: now,
	}
	signer, err := iam.NewAgentTunnelTokenSigner(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := iam.NewAgentTunnelTokenVerifier(config, keys)
	if err != nil {
		t.Fatal(err)
	}

	verifyToken := func(ctx context.Context, token, tenant, cluster string) (time.Time, error) {
		claims, err := verifier.VerifyClaims(ctx, token, tenant, cluster)
		if err != nil {
			return time.Time{}, err
		}
		session, err := store.GetAgentSession(ctx, tenant, cluster, claims.SessionID)
		if err != nil || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) || session.Generation != claims.Generation {
			return time.Time{}, fmt.Errorf("agent session unavailable")
		}
		return time.Unix(claims.ExpiresAt, 0).UTC(), nil
	}
	tunnelServer := tunnel.NewTunnelServer(verifyToken)
	tunnelHTTP := httptest.NewServer(tunnelServer)
	defer tunnelHTTP.Close()

	const sessionTTL = 4 * time.Second
	agentHandler := NewAgentEnrollmentHandler(
		store, store, targets, signer, verifier, sessionTTL, tunnelServer.Revoke, now, nil,
	)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/agent-enrollments:exchange", agentHandler.Exchange)
	mux.HandleFunc("POST /api/v1/agent-sessions:renew", agentHandler.Renew)
	mux.HandleFunc("DELETE /api/v1/resources/clusters/{id}/agent-session", func(w http.ResponseWriter, r *http.Request) {
		trusted := iam.TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: tenantID}
		agentHandler.RevokeSessions(w, r.WithContext(iam.WithTrustedContext(r.Context(), trusted)))
	})
	apiHTTP := httptest.NewServer(mux)
	defer apiHTTP.Close()

	enrollmentSecret, enrollmentHash, err := iam.NewEnrollmentSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEnrollmentCredential(context.Background(), iam.EnrollmentCredentialRecord{
		ID: uuid.NewString(), TokenHash: enrollmentHash, TenantID: tenantID, TargetID: targetID,
		ExpectedSubject: "cluster-agent", ExpiresAt: time.Now().Add(15 * time.Minute), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	issuer, err := iam.NewHTTPAgentSessionIssuer(iam.HTTPAgentSessionIssuerConfig{BaseURL: apiHTTP.URL})
	if err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	source, err := iam.NewAgentSessionTokenSource(context.Background(), verifier, issuer, tenantID, targetID, enrollmentSecret)
	if err != nil {
		t.Fatal(err)
	}
	source.ConfigurePersistence(iam.FileAgentSessionStore{}, sessionPath)
	defer source.Close()

	wsURL := "ws" + strings.TrimPrefix(tunnelHTTP.URL, "http")
	connect := func(src iam.TokenSource) (*tunnel.AgentClient, error) {
		client := tunnel.NewAgentClient(wsURL, src, tenantID, targetID)
		if err := client.Connect(); err != nil {
			return nil, err
		}
		return client, nil
	}
	client, err := connect(source)
	if err != nil {
		t.Fatalf("initial connect: %v", err)
	}
	start := time.Now()

	// Cross more than two session TTLs. The server closes each connection at
	// the token expiry; the source renews at half-life and the Agent reconnects
	// with the rotated token.
	reconnects := 0
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.ReadMessage(); err != nil {
			if errors.Is(err, tunnel.ErrSessionRevoked) {
				t.Fatalf("session revoked during rotation drill: %v", err)
			}
			client.Close()
			time.Sleep(100 * time.Millisecond)
			client, err = connect(source)
			if err != nil {
				t.Fatalf("reconnect after rotation: %v", err)
			}
			reconnects++
		}
	}
	client.Close()
	if reconnects < 2 {
		t.Fatalf("expected at least 2 reconnects across 2 TTLs, got %d", reconnects)
	}
	latest := latestDrillSession(t, db, tenantID, targetID)
	if latest.IssuedAt.Before(start) {
		t.Fatalf("session did not auto-renew: issued_at=%v start=%v", latest.IssuedAt, start)
	}
	if latest.Generation != 1 {
		t.Fatalf("rotation changed generation: %d", latest.Generation)
	}
	t.Logf("rotation: reconnects=%d generation=%d ttl=%s elapsed=%s", reconnects, latest.Generation, sessionTTL, time.Since(start))

	// Restart: the enrollment secret is single-use and already consumed. Only
	// the persisted session can restore the connection.
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := iam.NewAgentSessionTokenSource(context.Background(), verifier, issuer, tenantID, targetID, "already-consumed")
	if err != nil {
		t.Fatal(err)
	}
	restarted.ConfigurePersistence(iam.FileAgentSessionStore{}, sessionPath)
	defer restarted.Close()
	client2, err := connect(restarted)
	if err != nil {
		t.Fatalf("restart connect from persisted session: %v", err)
	}
	defer client2.Close()
	t.Logf("restart: resumed from persisted session without exchange")

	// Cross-tenant exchange is rejected.
	if _, err := issuer.ExchangeAgentSession(context.Background(), enrollmentSecret, "other-tenant", targetID); !errors.Is(err, iam.ErrServiceCredentialRejected) {
		t.Fatalf("cross-tenant exchange accepted: %v", err)
	}

	sessionToken, err := restarted.Token(context.Background())
	if err != nil {
		t.Fatalf("read resumed session token: %v", err)
	}

	// Revoke the identity and measure the persistent-connection close.
	revokeStart := time.Now()
	response, err := http.NewRequest(http.MethodDelete, apiHTTP.URL+"/api/v1/resources/clusters/"+targetID+"/agent-session", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(response)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d", resp.StatusCode)
	}
	closed := make(chan error, 1)
	go func() {
		_, err := client2.ReadMessage()
		closed <- err
	}()
	select {
	case err := <-closed:
		if !errors.Is(err, tunnel.ErrSessionRevoked) {
			t.Fatalf("revocation close error = %v, want ErrSessionRevoked", err)
		}
		t.Logf("revocation: persistent connection closed in %s", time.Since(revokeStart))
	case <-time.After(5 * time.Second):
		t.Fatal("revocation did not close the persistent connection within 5s")
	}

	// New renewal is rejected after revocation.
	if _, err := issuer.RenewAgentSession(context.Background(), sessionToken); !errors.Is(err, iam.ErrServiceCredentialRejected) {
		t.Fatalf("revoked session renewed: %v", err)
	}
	if time.Since(revokeStart) > 5*time.Second {
		t.Fatalf("new-call revocation exceeded 5s: %s", time.Since(revokeStart))
	}
	t.Logf("revocation: new renewal rejected; total=%s", time.Since(revokeStart))

}

type drillTargetResolver struct{ tenantID, targetID string }

func (d drillTargetResolver) ResolveAgentTarget(_ context.Context, tenantID, targetID string) error {
	if tenantID != d.tenantID || targetID != d.targetID {
		return ErrAgentTargetNotFound
	}
	return nil
}

func setupDrillSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`DROP TABLE IF EXISTS agent_sessions, service_enrollment_credentials, service_credentials, service_identities, identity_subjects, tenants, runtime_targets CASCADE`,
		`CREATE TABLE tenants (id TEXT PRIMARY KEY)`,
		`CREATE TABLE identity_subjects (id UUID PRIMARY KEY, issuer TEXT NOT NULL, external_subject TEXT NOT NULL, subject_type TEXT NOT NULL)`,
		`CREATE TABLE runtime_targets (id TEXT NOT NULL, tenant_id TEXT NOT NULL, target_type TEXT NOT NULL, PRIMARY KEY (id, tenant_id))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate drill test")
	}
	migration := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "database", "postgresql", "migrations", "105_service_identity_lifecycle.sql")
	data, err := os.ReadFile(migration)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(data)); err != nil {
			t.Fatalf("migration pass %d: %v", i+1, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO tenants (id) VALUES ('drill-tenant')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runtime_targets (id, tenant_id, target_type) VALUES ('drill-target', 'drill-tenant', 'kubernetes')`); err != nil {
		t.Fatal(err)
	}
}

func latestDrillSession(t *testing.T, db *sql.DB, tenantID, targetID string) iam.AgentSessionRecord {
	t.Helper()
	var record iam.AgentSessionRecord
	err := db.QueryRow(`
		SELECT id::text, tenant_id, target_id, subject, generation, issued_at, expires_at
		FROM agent_sessions WHERE tenant_id = $1 AND target_id = $2 ORDER BY created_at DESC LIMIT 1`,
		tenantID, targetID).Scan(&record.ID, &record.TenantID, &record.TargetID, &record.Subject,
		&record.Generation, &record.IssuedAt, &record.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

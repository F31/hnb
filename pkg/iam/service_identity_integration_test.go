//go:build integration

package iam

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("HNB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HNB_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	return db
}

func integrationMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "database", "postgresql", "migrations", "105_service_identity_lifecycle.sql")
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE IF EXISTS agent_sessions, service_enrollment_credentials, service_credentials, service_identities, identity_subjects, tenants CASCADE`,
		`CREATE TABLE identity_subjects (id UUID PRIMARY KEY, issuer TEXT NOT NULL, external_subject TEXT NOT NULL, subject_type TEXT NOT NULL)`,
		`CREATE TABLE tenants (id TEXT PRIMARY KEY)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	// The forward migration must be safe to run twice (repeatable owning-plane
	// convergence, not a one-shot manual step).
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("migration pass %d: %v", i+1, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO tenants (id) VALUES ('tenant-a'), ('tenant-b')`); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		if _, err := db.Exec(`INSERT INTO identity_subjects (id, issuer, external_subject, subject_type) VALUES ($1, $2, $2, 'service')`,
			uuid.NewString(), "service:"+tenant); err != nil {
			t.Fatal(err)
		}
	}
}

func insertServiceSubject(t *testing.T, db *sql.DB, tenant string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO identity_subjects (id, issuer, external_subject, subject_type) VALUES ($1, $2, $2, 'service')`,
		id, "service:"+tenant+":"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestServiceIdentityStoreAtomicConsumptionAndRevocation(t *testing.T) {
	db := integrationDB(t)
	integrationMigration(t, db)
	store := NewServiceIdentityDBStore(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	tenantA, tenantB := "tenant-a", "tenant-b"
	subjectA := insertServiceSubject(t, db, tenantA)
	identity, err := store.CreateServiceIdentity(ctx, ServiceIdentityRecord{
		ID: uuid.NewString(), TenantID: tenantA, SubjectID: subjectA, Audience: "provider-audience",
		Permission: ScopedPermission{ResourceKind: "runtimeProvider", ResourceID: "provider-a", Action: ActionExecute, TenantID: tenantA},
		Status:     ServiceIdentityStatusActive, Version: 1, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetServiceIdentity(ctx, tenantB, identity.ID); !errors.Is(err, ErrServiceIdentityNotFound) {
		t.Fatalf("cross-tenant read was accepted: %v", err)
	}

	if _, err := store.RevokeServiceIdentity(ctx, tenantA, identity.ID, 9, "", "x", now); !errors.Is(err, ErrServiceIdentityVersionConflict) {
		t.Fatalf("wrong version was accepted: %v", err)
	}
	if _, err := store.RevokeServiceIdentity(ctx, tenantB, identity.ID, 1, "", "x", now); !errors.Is(err, ErrServiceIdentityNotFound) {
		t.Fatalf("cross-tenant revoke was accepted: %v", err)
	}
	revoked, err := store.RevokeServiceIdentity(ctx, tenantA, identity.ID, 1, "", "retired", now)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != ServiceIdentityStatusRevoked || revoked.Version != 2 {
		t.Fatalf("unexpected revocation result: %+v", revoked)
	}
	if _, err := store.RevokeServiceIdentity(ctx, tenantA, identity.ID, 2, "", "again", now); !errors.Is(err, ErrServiceIdentityRevoked) {
		t.Fatalf("repeated revocation was accepted: %v", err)
	}

	credentialID := uuid.NewString()
	if err := store.RegisterServiceCredential(ctx, ServiceCredentialRecord{
		ID: credentialID, ServiceIdentityID: identity.ID, TenantID: tenantA, SubjectID: subjectA,
		Audience: "provider-audience", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeServiceCredential(ctx, tenantB, identity.ID, credentialID, "", "x", now); !errors.Is(err, ErrServiceCredentialNotFound) {
		t.Fatalf("cross-tenant credential revoke was accepted: %v", err)
	}
	if err := store.RevokeServiceCredential(ctx, tenantA, identity.ID, credentialID, "", "retired", now); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeServiceCredential(ctx, tenantA, identity.ID, credentialID, "", "retired", now); err != nil {
		t.Fatalf("idempotent revoke failed: %v", err)
	}

	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := store.CreateEnrollmentCredential(ctx, EnrollmentCredentialRecord{
		ID: uuid.NewString(), TokenHash: hash, TenantID: tenantA, TargetID: "target-a", ExpectedSubject: "cluster-agent",
		ExpiresAt: now.Add(DefaultEnrollmentTTL), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.ConsumeEnrollmentCredential(ctx, hash, tenantA, "target-a", "cluster-agent", "agent-a", now)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrEnrollmentUnavailable) {
			t.Fatalf("unexpected consumption error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful consumption, got %d", successes)
	}
	if _, err := store.ConsumeEnrollmentCredential(ctx, hash, tenantB, "target-a", "cluster-agent", "agent-b", now); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatalf("cross-tenant replay was accepted: %v", err)
	}
	if _, err := store.ConsumeEnrollmentCredential(ctx, hash, tenantA, "target-b", "cluster-agent", "agent-b", now); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatalf("cross-target replay was accepted: %v", err)
	}

	expiredHash := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	if err := store.CreateEnrollmentCredential(ctx, EnrollmentCredentialRecord{
		ID: uuid.NewString(), TokenHash: expiredHash, TenantID: tenantA, TargetID: "target-a", ExpectedSubject: "cluster-agent",
		ExpiresAt: now.Add(-time.Second), CreatedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConsumeEnrollmentCredential(ctx, expiredHash, tenantA, "target-a", "cluster-agent", "agent-a", now); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatalf("expired enrollment was accepted: %v", err)
	}

	revokedID := uuid.NewString()
	revokedHash := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	if err := store.CreateEnrollmentCredential(ctx, EnrollmentCredentialRecord{
		ID: revokedID, TokenHash: revokedHash, TenantID: tenantA, TargetID: "target-a", ExpectedSubject: "cluster-agent",
		ExpiresAt: now.Add(DefaultEnrollmentTTL), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeEnrollmentCredential(ctx, tenantB, "target-a", revokedID, "", now); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatalf("cross-tenant enrollment revoke was accepted: %v", err)
	}
	if err := store.RevokeEnrollmentCredential(ctx, tenantA, "target-a", revokedID, "", now); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeEnrollmentCredential(ctx, tenantA, "target-a", revokedID, "", now); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatalf("repeated enrollment revoke was accepted: %v", err)
	}
	if _, err := store.ConsumeEnrollmentCredential(ctx, revokedHash, tenantA, "target-a", "cluster-agent", "agent-a", now); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatalf("revoked enrollment was accepted: %v", err)
	}
}

func TestAgentSessionStoreLifecycle(t *testing.T) {
	db := integrationDB(t)
	integrationMigration(t, db)
	store := NewServiceIdentityDBStore(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	tenantA, tenantB := "tenant-a", "tenant-b"

	first, err := store.CreateAgentSession(ctx, AgentSessionRecord{
		ID: uuid.NewString(), TenantID: tenantA, TargetID: "target-a", Subject: "cluster-agent",
		IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation != 1 {
		t.Fatalf("first generation = %d, want 1", first.Generation)
	}
	second, err := store.CreateAgentSession(ctx, AgentSessionRecord{
		ID: uuid.NewString(), TenantID: tenantA, TargetID: "target-a", Subject: "cluster-agent",
		IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != 2 {
		t.Fatalf("second generation = %d, want 2", second.Generation)
	}
	rotated, err := store.RotateAgentSession(ctx, tenantA, "target-a", first.ID, now.Add(time.Minute), now.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != first.ID || rotated.Generation != 1 || !rotated.ExpiresAt.Equal(now.Add(6*time.Minute)) {
		t.Fatalf("unexpected rotation: %+v", rotated)
	}
	if _, err := store.GetAgentSession(ctx, tenantB, "target-a", first.ID); !errors.Is(err, ErrAgentSessionNotFound) {
		t.Fatalf("cross-tenant session read was accepted: %v", err)
	}
	if count, err := store.RevokeAgentSessions(ctx, tenantB, "target-a", "", now); err != nil || count != 0 {
		t.Fatalf("cross-tenant revoke = %d, %v", count, err)
	}
	if count, err := store.RevokeAgentSessions(ctx, tenantA, "target-a", "", now.Add(2*time.Minute)); err != nil || count != 2 {
		t.Fatalf("revoke = %d, %v", count, err)
	}
	if _, err := store.RotateAgentSession(ctx, tenantA, "target-a", first.ID, now, now.Add(time.Minute)); !errors.Is(err, ErrAgentSessionUnavailable) {
		t.Fatalf("revoked session rotated: %v", err)
	}
	if count, err := store.RevokeAgentSessions(ctx, tenantA, "target-a", "", now); err != nil || count != 0 {
		t.Fatalf("idempotent revoke = %d, %v", count, err)
	}
}

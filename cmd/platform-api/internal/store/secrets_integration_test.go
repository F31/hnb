package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestResolveSecretReferenceMetadata(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	tenantID := "secret-admission-" + uuid.NewString()
	providerID := uuid.NewString()
	secretID := uuid.NewString()

	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM secret_references WHERE id = $1`, secretID)
		_, _ = db.ExecContext(ctx, `DELETE FROM kms_providers WHERE id = $1`, providerID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO kms_providers (id, provider_type, name, is_active)
		VALUES ($1, 'vault', $2, true)`, providerID, "vault-"+tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO secret_references (
			id, tenant_id, name, secret_ref, encrypted_value, version, kms_provider_id,
			scope, purpose, allowed_lifecycle_provider_id, is_active
		) VALUES ($1,$2,'edge-credential','secret://edge','not-read-by-admission',3,$3,$4,
			'cloudcore-client','runtime-target.lifecycle.edge',true)`,
		secretID, tenantID, providerID, "tenant:"+tenantID); err != nil {
		t.Fatal(err)
	}

	repo := NewPGStore(db)
	metadata, err := repo.ResolveSecretReference(ctx, tenantID, "vault-"+tenantID, "tenant:"+tenantID, "edge-credential", "3")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Purpose != "cloudcore-client" || metadata.AllowedLifecycleProviderID != "runtime-target.lifecycle.edge" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	if _, err := repo.ResolveSecretReference(ctx, "other-tenant", "vault-"+tenantID, "tenant:"+tenantID, "edge-credential", "3"); !errors.Is(err, ErrSecretReferenceDenied) {
		t.Fatalf("cross-tenant lookup error = %v", err)
	}
	if _, err := repo.ResolveSecretReference(ctx, tenantID, "vault-"+tenantID, "tenant:"+tenantID, "edge-credential", "2"); !errors.Is(err, ErrSecretReferenceDenied) {
		t.Fatalf("wrong-version lookup error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE secret_references SET is_active = false WHERE id = $1`, secretID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveSecretReference(ctx, tenantID, "vault-"+tenantID, "tenant:"+tenantID, "edge-credential", "3"); !errors.Is(err, ErrSecretReferenceDenied) {
		t.Fatalf("inactive lookup error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE secret_references SET is_active = true, expires_at = now() - interval '1 minute' WHERE id = $1`, secretID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveSecretReference(ctx, tenantID, "vault-"+tenantID, "tenant:"+tenantID, "edge-credential", "3"); !errors.Is(err, ErrSecretReferenceDenied) {
		t.Fatalf("expired lookup error = %v", err)
	}
}

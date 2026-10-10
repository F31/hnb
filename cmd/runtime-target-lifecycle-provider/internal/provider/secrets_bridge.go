package provider

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provision"
)

// provisionStore bridges the provider's PostgreSQL + KMS cipher to the
// provision.SecretStore / provision.KeyResolver contracts so the provisioner
// can resolve an SSH key and persist the resulting admin kubeconfig.
type provisionStore struct {
	db     *sql.DB
	cipher secretCipher
}

// Execution-time gate; UI selection alone is not authorization to bootstrap.
func (s *provisionStore) ValidateVersion(ctx context.Context, version string) error {
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM platform_settings WHERE key='resource.cluster.kubernetesVersions'`).Scan(&raw); err != nil {
		return fmt.Errorf("version evidence unavailable: %w", err)
	}
	var catalog struct {
		Items []struct {
			Code   string `json:"code"`
			Status string `json:"integrationTestStatus"`
			Report string `json:"integrationReport"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return err
	}
	for _, item := range catalog.Items {
		if item.Code == version && item.Status == "passed" && item.Report != "" {
			return nil
		}
	}
	return fmt.Errorf("exact version %q has no accepted integration report", version)
}

// NewProvisionStore builds the bridge used by the self-hosted provisioner.
func NewProvisionStore(db *sql.DB, cipher secretCipher) *provisionStore {
	return &provisionStore{db: db, cipher: cipher}
}

// ResolveSSHKey returns the plaintext SSH private key for a SecretReference
// with purpose=ssh bound to the kubernetes lifecycle provider.
func (s *provisionStore) ResolveSSHKey(ctx context.Context, tenantID string, ref provision.SecretReference) ([]byte, error) {
	p := SecretReference{Provider: ref.Provider, Scope: ref.Scope, Name: ref.Name, Version: ref.Version}
	resolver := NewPGSecretResolver(s.db, s.cipher)
	return resolver.Resolve(ctx, tenantID, "runtime-target.lifecycle.kubernetes", "ssh", p)
}

// StoreKubeconfig persists the provisioned admin kubeconfig under
// purpose=kubeconfig, bound to the kubernetes lifecycle provider, and returns
// its SecretReference.
func (s *provisionStore) StoreKubeconfig(ctx context.Context, tenantID, scope, name string, kubeconfig []byte) (provision.SecretReference, error) {
	if tenantID == "" || scope == "" || name == "" {
		return provision.SecretReference{}, fmt.Errorf("tenant, scope, and name are required to store a kubeconfig")
	}
	if len(kubeconfig) == 0 {
		return provision.SecretReference{}, fmt.Errorf("empty kubeconfig")
	}

	sealed, err := s.cipher.Encrypt(kubeconfig)
	if err != nil {
		return provision.SecretReference{}, fmt.Errorf("encrypt kubeconfig: %w", err)
	}

	// Pick the tenant's active KMS provider (matches the register path).
	var providerID, providerName string
	err = s.db.QueryRowContext(ctx, `
		SELECT id::text, name FROM kms_providers
		WHERE is_active AND is_default = true
		ORDER BY created_at ASC LIMIT 1`).Scan(&providerID, &providerName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = s.db.QueryRowContext(ctx, `
				SELECT id::text, name FROM kms_providers
				WHERE is_active ORDER BY created_at ASC LIMIT 1`).Scan(&providerID, &providerName)
		}
		if err != nil {
			return provision.SecretReference{}, fmt.Errorf("resolve kms provider: %w", err)
		}
	}

	// Regenerate on conflict so the ref version always matches stored content.
	var version string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO secret_references (
			tenant_id, name, secret_ref, scope, purpose, encrypted_value,
			algorithm, kms_provider_id, allowed_lifecycle_provider_id, is_active
		) VALUES ($1, $2, $2, $3, 'kubeconfig', $4, 'AES-256-GCM', $5::uuid,
			'runtime-target.lifecycle.kubernetes', true)
		ON CONFLICT (tenant_id, name, scope, purpose, allowed_lifecycle_provider_id)
		DO UPDATE SET secret_ref = EXCLUDED.secret_ref,
			encrypted_value = EXCLUDED.encrypted_value,
			kms_provider_id = EXCLUDED.kms_provider_id,
			version = secret_references.version + 1,
			updated_at = now(),
			is_active = true
		RETURNING version::text`,
		tenantID, name, scope, sealed, providerID).Scan(&version)
	if err != nil {
		return provision.SecretReference{}, fmt.Errorf("store kubeconfig reference: %w", err)
	}

	return provision.SecretReference{
		Provider: providerName,
		Scope:    scope,
		Name:     name,
		Version:  version,
	}, nil
}

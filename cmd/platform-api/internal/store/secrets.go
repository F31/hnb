package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *PGStore) ResolveSecretReference(ctx context.Context, tenantID, provider, scope, name, version string) (*SecretReferenceMetadata, error) {
	metadata := &SecretReferenceMetadata{}
	err := s.db.QueryRowContext(ctx, `
		SELECT sr.tenant_id, kp.name, sr.scope, sr.name, sr.version::text,
		       sr.purpose, COALESCE(sr.allowed_lifecycle_provider_id, '')
		FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1 AND kp.name = $2 AND sr.scope = $3
		  AND sr.name = $4 AND ($5 = '' OR sr.version::text = $5)
		  AND sr.is_active AND (sr.expires_at IS NULL OR sr.expires_at > now())`,
		tenantID, provider, scope, name, version,
	).Scan(&metadata.TenantID, &metadata.Provider, &metadata.Scope, &metadata.Name,
		&metadata.Version, &metadata.Purpose, &metadata.AllowedLifecycleProviderID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSecretReferenceDenied
	}
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

// LatestKubeConfigEncrypted returns the most recently registered kubeconfig
// secret value for the tenant (still sealed). It is the fallback for legacy
// targets that were created before credential_ref was recorded; callers must
// enforce cluster authorization before calling.
func (s *PGStore) LatestKubeConfigEncrypted(ctx context.Context, tenantID string) (string, error) {
	var encrypted string
	err := s.db.QueryRowContext(ctx, `
		SELECT sr.encrypted_value
		FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1
		  AND sr.purpose = 'kubeconfig'
		  AND sr.allowed_lifecycle_provider_id = 'runtime-target.lifecycle.kubernetes'
		  AND sr.is_active
		ORDER BY sr.created_at DESC
		LIMIT 1`, tenantID).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSecretReferenceDenied
	}
	if err != nil {
		return "", err
	}
	return encrypted, nil
}

// KubeConfigEncryptedForRef returns the sealed kubeconfig value for the exact
// secret reference (scope + name) bound to a target at creation time. Unlike
// LatestKubeConfigEncrypted it never falls back to another secret: when the
// referenced secret is missing or deactivated, ErrSecretReferenceDenied is
// returned so a caller can fail closed instead of issuing the wrong cluster's
// credentials.
func (s *PGStore) KubeConfigEncryptedForRef(ctx context.Context, tenantID, scope, name string) (string, error) {
	var encrypted string
	err := s.db.QueryRowContext(ctx, `
		SELECT sr.encrypted_value
		FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1
		  AND sr.scope = $2
		  AND sr.name = $3
		  AND sr.purpose = 'kubeconfig'
		  AND sr.is_active
		ORDER BY sr.created_at DESC
		LIMIT 1`, tenantID, scope, name).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSecretReferenceDenied
	}
	if err != nil {
		return "", err
	}
	return encrypted, nil
}

// RegisterSecretReference creates a tenant-scoped secret reference encrypted at
// rest. It ensures the "local-aes" KMS provider row exists and records an
// initial version entry in secret_versions.
func (s *PGStore) RegisterSecretReference(ctx context.Context, req RegisterSecretReferenceRequest) (*SecretReferenceMetadata, error) {
	if req.Name == "" || req.Scope == "" || req.Purpose == "" || req.EncryptedValue == "" || req.TenantID == "" {
		return nil, fmt.Errorf("register secret: name, scope, purpose, encrypted value and tenant are required")
	}
	var kmsProviderID, secretID, versionString string
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("register secret: begin tx: %w", err)
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO kms_providers (provider_type, name, description, config, is_default, is_active)
		VALUES ('local_aes', 'local-aes', 'Platform AES-256-GCM local KMS', '{}', false, true)
		ON CONFLICT (name) DO UPDATE SET is_active = true
		RETURNING id`,
	).Scan(&kmsProviderID)
	if err != nil {
		return nil, fmt.Errorf("register secret: ensure kms provider: %w", err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO secret_references (
			tenant_id, name, secret_ref, description, encrypted_value, algorithm,
			version, kms_provider_id, scope, purpose, allowed_lifecycle_provider_id, is_active
		) VALUES (
			$1, $2, $2, NULL, $3, $4, 1, $5, $6, $7, NULLIF($8, ''), true
		)
		ON CONFLICT (tenant_id, name) DO UPDATE
			SET encrypted_value = EXCLUDED.encrypted_value,
			    algorithm = EXCLUDED.algorithm,
			    kms_provider_id = EXCLUDED.kms_provider_id,
			    scope = EXCLUDED.scope,
			    purpose = EXCLUDED.purpose,
			    allowed_lifecycle_provider_id = EXCLUDED.allowed_lifecycle_provider_id,
			    is_active = true,
			    updated_at = now()
		RETURNING id, version::text`,
		req.TenantID, req.Name, req.EncryptedValue, req.Algorithm,
		kmsProviderID, req.Scope, req.Purpose, req.AllowedLifecycleProviderID,
	).Scan(&secretID, &versionString)
	if err != nil {
		return nil, fmt.Errorf("register secret: insert reference: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO secret_versions (secret_id, version, encrypted_value, created_by)
		VALUES ($1, 1, $2, $3)
		ON CONFLICT (secret_id, version) DO UPDATE SET created_at = now()`,
		secretID, req.EncryptedValue, req.SubjectID,
	); err != nil {
		return nil, fmt.Errorf("register secret: insert version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("register secret: commit: %w", err)
	}
	return &SecretReferenceMetadata{
		TenantID: req.TenantID, Provider: "local-aes", Scope: req.Scope,
		Name: req.Name, Version: versionString, Purpose: req.Purpose,
		AllowedLifecycleProviderID: req.AllowedLifecycleProviderID,
	}, nil
}

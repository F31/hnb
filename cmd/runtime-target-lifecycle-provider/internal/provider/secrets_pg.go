package provider

import (
	"context"
	"database/sql"
	"errors"

	_ "github.com/lib/pq"
)

// secretCipher opens AES-256-GCM sealed payloads (base64 nonce||ciphertext).
type secretCipher interface {
	Decrypt(sealed string) ([]byte, error)
}

// PGSecretResolver resolves a SecretReference by reading the encrypted value
// from secret_references (joined to kms_providers) and decrypting it with the
// platform master key. It enforces tenant isolation, the KMS provider name,
// the secret purpose, and the allowed lifecycle provider as security boundaries.
type PGSecretResolver struct {
	db     *sql.DB
	cipher secretCipher
}

// NewPGSecretResolver builds a resolver backed by the platform database.
func NewPGSecretResolver(db *sql.DB, cipher secretCipher) *PGSecretResolver {
	return &PGSecretResolver{db: db, cipher: cipher}
}

func (r *PGSecretResolver) Resolve(ctx context.Context, tenantID, providerID, purpose string, ref SecretReference) ([]byte, error) {
	if tenantID == "" {
		return nil, invalid("tenant id is required to resolve a credential")
	}
	if ref.Provider == "" || ref.Scope == "" || ref.Name == "" {
		return nil, invalid("credentialSecretRef must include provider, scope, and name")
	}
	var encrypted string
	err := r.db.QueryRowContext(ctx, `
		SELECT sr.encrypted_value
		FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1
		  AND kp.name = $2
		  AND sr.scope = $3
		  AND sr.name = $4
		  AND sr.is_active
		  AND (sr.expires_at IS NULL OR sr.expires_at > now())
		  AND ($5 = '' OR sr.purpose = $5)
		  AND ($6 = '' OR sr.allowed_lifecycle_provider_id = $6)
		LIMIT 1`,
		tenantID, ref.Provider, ref.Scope, ref.Name, purpose, providerID).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fail(403, ErrorScopeDenied, false, "secret reference is not authorized for this lifecycle provider")
	}
	if err != nil {
		return nil, fail(503, ErrorTargetUnavailable, true, "resolve secret reference: %v", err)
	}
	plaintext, err := r.cipher.Decrypt(encrypted)
	if err != nil {
		return nil, fail(500, ErrorTargetUnavailable, true, "decrypt secret reference: %v", err)
	}
	return plaintext, nil
}

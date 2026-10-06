// Package credentials resolves runtime-target kubeconfigs from the platform
// secret store. It mirrors the semantics of cmd/plugin-worker's
// kubeconfigFor: the message stream carries only target IDs, never credential
// material; each worker resolves credentials itself and must fail closed
// rather than fall back to its local kube context.
package credentials

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/F31/hnb/pkg/kms"
)

// Resolver resolves the kubeconfig of a runtime target and returns the path
// of a local file containing the plaintext kubeconfig, suitable for passing
// to helm --kubeconfig.
type Resolver interface {
	KubeconfigFor(ctx context.Context, targetID string) (string, error)
}

// PGResolver resolves a target's kubeconfig via runtime_targets.credential_ref
// (or, for legacy targets without one, the tenant's most recent kubeconfig
// secret), decrypting the stored value with the platform master key.
type PGResolver struct {
	db     *sql.DB
	cipher kms.Decrypter
	dir    string
}

// NewPGResolver builds a resolver backed by the platform database. cipher may
// be nil when no master key is configured, in which case every resolution
// fails closed with a clear error.
func NewPGResolver(db *sql.DB, cipher kms.Decrypter, dir string) *PGResolver {
	return &PGResolver{db: db, cipher: cipher, dir: dir}
}

// KubeconfigFor resolves the target cluster's kubeconfig via its
// credential_ref (or the tenant's most recent kubeconfig secret) and returns
// a file path containing the plaintext kubeconfig.
func (r *PGResolver) KubeconfigFor(ctx context.Context, targetID string) (string, error) {
	if r.cipher == nil {
		return "", fmt.Errorf("master key not configured")
	}
	var tenantID, scope, name string
	var encrypted, candidate string
	// Exact credential_ref preferred (same semantics as platform-api
	// KubeConfigEncryptedForRef); legacy targets fall back to the tenant's
	// most recent kubeconfig secret.
	err := r.db.QueryRowContext(ctx, `
		SELECT rt.tenant_id,
		       COALESCE(rt.credential_ref->>'scope','') ,
		       COALESCE(rt.credential_ref->>'name','')
		FROM runtime_targets rt WHERE rt.id = $1::uuid`, targetID).Scan(&tenantID, &scope, &name)
	if err != nil {
		return "", fmt.Errorf("load target: %w", err)
	}
	if scope != "" && name != "" {
		err = r.db.QueryRowContext(ctx, `
			SELECT sr.encrypted_value FROM secret_references sr
			JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
			WHERE sr.tenant_id = $1 AND sr.scope = $2 AND sr.name = $3
			  AND sr.purpose = 'kubeconfig' AND sr.is_active
			ORDER BY sr.created_at DESC LIMIT 1`, tenantID, scope, name).Scan(&encrypted)
		if err == nil {
			return r.writeKubeconfig(targetID, encrypted)
		}
		log.Printf("[network-provider] credential_ref secret missing (%s/%s): %v", scope, name, err)
	}
	err = r.db.QueryRowContext(ctx, `
		SELECT sr.encrypted_value FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1 AND sr.purpose = 'kubeconfig'
		  AND sr.is_active
		ORDER BY sr.created_at DESC LIMIT 1`, tenantID).Scan(&candidate)
	if err != nil {
		return "", fmt.Errorf("no kubeconfig secret for tenant %s: %w", tenantID, err)
	}
	return r.writeKubeconfig(targetID, candidate)
}

func (r *PGResolver) writeKubeconfig(targetID, sealed string) (string, error) {
	decrypted, err := r.cipher.Decrypt(sealed)
	if err != nil {
		return "", fmt.Errorf("decrypt kubeconfig: %w", err)
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(r.dir, "kubeconfig-"+targetID+".yaml")
	if err := os.WriteFile(path, decrypted, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

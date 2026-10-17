package provider

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provision"
	"github.com/F31/hnb/pkg/kms"
)

// TestProvisionRealSSH is opt-in: it bootstraps the explicitly supplied nodes.
// Node reset is deliberately outside this test; the operator supplies clean nodes.
func TestProvisionRealSSH(t *testing.T) {
	raw := os.Getenv("HNB_PROVISION_E2E_SPEC")
	if raw == "" {
		t.Skip("set HNB_PROVISION_E2E_SPEC, HNB_PROVISION_E2E_KEY_FILE, DB_DSN and HNB_MASTER_KEY")
	}
	var spec provision.ProvisionSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(os.Getenv("HNB_PROVISION_E2E_KEY_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := kms.NewAESGCMFromHex(os.Getenv("HNB_MASTER_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", os.Getenv("DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	var providerID, providerName string
	if err := db.QueryRowContext(ctx, `SELECT id::text, name FROM kms_providers WHERE is_active ORDER BY is_default DESC, created_at ASC LIMIT 1`).Scan(&providerID, &providerName); err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.Encrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	keyName := "e2e-ssh-" + spec.TargetID
	var keyVersion string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO secret_references (tenant_id, name, secret_ref, scope, purpose, encrypted_value,
		algorithm, kms_provider_id, allowed_lifecycle_provider_id, is_active)
		VALUES ($1, $2, $2, 'tenant', 'ssh', $3, 'AES-256-GCM', $4::uuid, 'runtime-target.lifecycle.kubernetes', true)
		ON CONFLICT (tenant_id, name, scope, purpose, allowed_lifecycle_provider_id)
		DO UPDATE SET encrypted_value=EXCLUDED.encrypted_value, version=secret_references.version+1, is_active=true
		RETURNING version::text`, spec.TenantID, keyName, sealed, providerID).Scan(&keyVersion); err != nil {
		t.Fatal(err)
	}
	spec.SSHCredential = provision.SecretReference{Provider: providerName, Scope: "tenant", Name: keyName, Version: keyVersion}
	store := NewProvisionStore(db, cipher)
	runner := &loggedRealSSH{t: t}
	p := provision.New(provision.WithBootstrapper(&provision.NodeBootstrapper{SSH: runner, Key: store, Store: store}))
	result, err := p.Provision(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != len(spec.Nodes) || result.ControlPlaneEndpoint != spec.Endpoint() {
		t.Fatalf("unexpected result: %+v", result)
	}
	ref := result.KubeconfigRef
	resolved, err := NewPGSecretResolver(db, cipher).Resolve(ctx, spec.TenantID, "runtime-target.lifecycle.kubernetes", "kubeconfig",
		SecretReference{Provider: ref.Provider, Scope: ref.Scope, Name: ref.Name, Version: ref.Version})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(resolved, []byte("server: https://"+spec.Endpoint())) || !bytes.Contains(resolved, []byte("client-certificate-data:")) {
		t.Fatal("stored kubeconfig lacks expected endpoint or client certificate")
	}
	var ciphertext string
	if err := db.QueryRowContext(ctx, `SELECT encrypted_value FROM secret_references WHERE tenant_id=$1 AND name=$2 AND purpose='kubeconfig'`, spec.TenantID, ref.Name).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "client-certificate-data:") || ciphertext == string(resolved) {
		t.Fatal("kubeconfig is not encrypted at rest")
	}
	t.Logf("accepted nodes=%d endpoint=%s kubeconfig_ref=%s encrypted_at_rest=true", result.NodeCount, result.ControlPlaneEndpoint, ref.String())
}

type loggedRealSSH struct {
	t *testing.T
	n int
}

func (r *loggedRealSSH) Run(ctx context.Context, node provision.NodeSpec, key []byte, script string) (string, error) {
	r.n++
	r.t.Logf("SSH step %d host=%s started", r.n, node.Host)
	out, err := (&provision.RealSSH{}).Run(ctx, node, key, script)
	r.t.Logf("SSH step %d host=%s completed success=%t", r.n, node.Host, err == nil)
	return out, err
}

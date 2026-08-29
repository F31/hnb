package worker_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/F31/hnb/cmd/plugin-worker/internal/worker"
	"github.com/F31/hnb/pkg/kms"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// fakeHelm writes a deterministic fake helm script to a temp dir and returns
// its path. The script appends its args to logPath for assertions.
func fakeHelm(t *testing.T, logPath string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "helm")
	content := `#!/bin/sh
echo "$0 $@" >> "` + logPath + `"
case "$1" in
  --kubeconfig) shift; shift;;
esac
case "$1" in
  upgrade|install) echo "Release \"demo\" has been installed. Happy Helming!" ;;
  status) echo "NAME: demo"; echo "STATUS: deployed" ;;
  uninstall) echo "release \"demo\" uninstalled" ;;
esac
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWorkerInstallWritesKubeconfigAndSucceeds(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tenantID := "plugin-worker-" + uuid.NewString()
	targetID := uuid.NewString()
	resourcesDir := t.TempDir()
	logPath := filepath.Join(resourcesDir, "helm.log")

	// Build a real AES-GCM cipher from a fixed master key and seal a fake kubeconfig.
	cipher, err := kms.NewAESGCMFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.Encrypt([]byte("apiVersion: v1\nkind: Config\n"))
	if err != nil {
		t.Fatal(err)
	}
	seeded := sealForRef(t, db, ctx, tenantID, sealed, targetID)

	w := worker.New(db, cipher, nil, fakeHelm(t, logPath), resourcesDir)
	req := worker.Request{
		Name: "cilium", Version: "v1.20.1", Provider: "cni", Action: "install", TargetID: targetID,
	}
	resp := w.Install(ctx, &req)
	if resp.Status != "succeeded" {
		t.Fatalf("install = %s (%s), want succeeded", resp.Status, resp.Message)
	}
	if _, err := os.Stat(filepath.Join(resourcesDir, "kubeconfig-"+targetID+".yaml")); err != nil {
		t.Fatalf("kubeconfig file not written: %v", err)
	}
	// The helm script must have received --kubeconfig and the chart name.
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(logData), "cilium/cilium") {
		t.Fatalf("helm log missing chart: %s", logData)
	}
	cleanupWorkerResources(t, db, ctx, tenantID, seeded, targetID)
}

func TestWorkerHealthAndUninstall(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tenantID := "plugin-worker-" + uuid.NewString()
	targetID := uuid.NewString()
	resourcesDir := t.TempDir()

	cipher, _ := kms.NewAESGCMFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	sealed, _ := cipher.Encrypt([]byte("apiVersion: v1\nkind: Config\n"))
	seeded := sealForRef(t, db, ctx, tenantID, sealed, targetID)

	w := worker.New(db, cipher, nil, fakeHelm(t, filepath.Join(resourcesDir, "helm.log")), resourcesDir)

	health := w.Health(ctx, &worker.Request{Name: "cilium", TargetID: targetID, Action: "health"})
	if health.Status != "succeeded" {
		t.Fatalf("health = %s (%s)", health.Status, health.Message)
	}
	un := w.Uninstall(ctx, &worker.Request{Name: "cilium", TargetID: targetID, Action: "uninstall"})
	if un.Status != "succeeded" {
		t.Fatalf("uninstall = %s (%s)", un.Status, un.Message)
	}
	cleanupWorkerResources(t, db, ctx, tenantID, seeded, targetID)
}

func TestWorkerInstallFailsClosedWithoutMasterKey(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tenantID := "plugin-worker-" + uuid.NewString()
	targetID := uuid.NewString()
	cipher, _ := kms.NewAESGCMFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	sealed, _ := cipher.Encrypt([]byte("apiVersion: v1\nkind: Config\n"))
	seeded := sealForRef(t, db, ctx, tenantID, sealed, targetID)

	w := worker.New(db, nil /* no master key */, nil, "helm", t.TempDir())
	resp := w.Install(ctx, &worker.Request{Name: "cilium", TargetID: targetID, Action: "install"})
	if resp.Status != "failed" || !contains(resp.Message, "master key") {
		t.Fatalf("install without key = %+v, want failed/master key", resp)
	}
	cleanupWorkerResources(t, db, ctx, tenantID, seeded, targetID)
}

// sealForRef seeds a tenant, a runtime target with credential_ref, and the
// referenced kubeconfig secret sealed with the in-memory cipher, mirroring what
// platform-api's SecretReference registration produces.
func sealForRef(t *testing.T, db *sql.DB, ctx context.Context, tenantID, sealed, targetID string) string {
	t.Helper()
	var secretID string
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	// kms_provider
	var providerID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO kms_providers (provider_type, name, description, config, is_default, is_active)
		VALUES ('local_aes','local-aes','','{}',false,true) ON CONFLICT (name) DO UPDATE SET is_active=true RETURNING id`,
	).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	// secret_references entry acting as the kubeconfig secret.
	if err := db.QueryRowContext(ctx, `
		INSERT INTO secret_references (tenant_id, name, scope, secret_ref, encrypted_value, kms_provider_id, purpose, allowed_lifecycle_provider_id, is_active)
		VALUES ($1,'kube-demo','tenant:'||$1,'ref://secrets/kube-demo',$2,$3,'kubeconfig','runtime-target.lifecycle.kubernetes',true)
		RETURNING id`, tenantID, sealed, providerID).Scan(&secretID); err != nil {
		t.Fatal(err)
	}
	// runtime target with credential_ref pointing at the secret.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status, credential_ref)
		VALUES ($1,$2,'demo','Demo','kubernetes','agent','online',
		        jsonb_build_object('provider','platform-secrets','scope','tenant:'||$2,'name','kube-demo'))`,
		targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	return secretID
}

func cleanupWorkerResources(t *testing.T, db *sql.DB, ctx context.Context, tenantID, secretID, targetID string) {
	t.Helper()
	_, _ = db.ExecContext(ctx, `DELETE FROM extensions WHERE target_id=$1`, targetID)
	_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, targetID)
	_, _ = db.ExecContext(ctx, `DELETE FROM secret_references WHERE id=$1`, secretID)
	_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
}

func contains(hay, needle string) bool {
	return len(needle) == 0 || (len(hay) >= len(needle) && indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestWorkerInstallWithRealHelm exercises the full path against a live cluster
// using the real helm binary: it seeds a kubeconfig secret backed by the
// HNB_TEST_REAL_HELM_KUBECONFIG file, installs a minimal local chart through
// the worker, asserts the release is deployed, then uninstalls it.
//
// This test is opt-in: set HNB_TEST_REAL_HELM=1 and
// HNB_TEST_REAL_HELM_KUBECONFIG to a kubeconfig for a reachable cluster, and
// HNB_TEST_REAL_HELM_HELM to the helm binary path. A minimal chart is created
// under the temp dir.
func TestWorkerInstallWithRealHelm(t *testing.T) {
	if os.Getenv("HNB_TEST_REAL_HELM") != "1" {
		t.Skip("set HNB_TEST_REAL_HELM=1 to run the real-helm integration test")
	}
	kubeconfigPath := os.Getenv("HNB_TEST_REAL_HELM_KUBECONFIG")
	helmPath := os.Getenv("HNB_TEST_REAL_HELM_HELM")
	if kubeconfigPath == "" || helmPath == "" {
		t.Skip("set HNB_TEST_REAL_HELM_KUBECONFIG and HNB_TEST_REAL_HELM_HELM")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// Minimal local chart (no remote repository / image pulls needed).
	chartDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	chartYAML := "apiVersion: v2\nname: worker-e2e\ndescription: minimal\nversion: 0.1.0\n"
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "templates", "configmap.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: worker-e2e-cm\ndata:\n  from: worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tenantID := "plugin-worker-" + uuid.NewString()
	targetID := uuid.NewString()
	resourcesDir := t.TempDir()

	cipher, _ := kms.NewAESGCMFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	kcRaw, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.Encrypt(kcRaw)
	if err != nil {
		t.Fatal(err)
	}
	seeded := sealForRef(t, db, ctx, tenantID, sealed, targetID)

	w := worker.New(db, cipher, nil, helmPath, resourcesDir)
	req := worker.Request{
		Name: "worker-e2e", Version: "0.1.0", Provider: "plugin", Action: "install",
		TargetID: targetID, Chart: chartDir,
	}
	if resp := w.Install(ctx, &req); resp.Status != "succeeded" {
		t.Fatalf("real install = %s (%s)", resp.Status, resp.Message)
	}
	if resp := w.Health(ctx, &worker.Request{Name: "worker-e2e", TargetID: targetID, Action: "health"}); resp.Status != "succeeded" {
		t.Fatalf("real health = %s (%s)", resp.Status, resp.Message)
	}
	if resp := w.Uninstall(ctx, &worker.Request{Name: "worker-e2e", TargetID: targetID, Action: "uninstall"}); resp.Status != "succeeded" {
		t.Fatalf("real uninstall = %s (%s)", resp.Status, resp.Message)
	}
	cleanupWorkerResources(t, db, ctx, tenantID, seeded, targetID)
}

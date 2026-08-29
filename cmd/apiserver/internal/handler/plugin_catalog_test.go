package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func pluginCatalogTestContext(subjectID, tenantID string) context.Context {
	ctx := iam.WithTrustedContext(context.Background(), iam.TrustedContext{
		SubjectID: subjectID, SubjectType: "user", TenantID: tenantID,
		MembershipID: "membership-1", PolicyVersion: "default:1",
		CorrelationID: "corr-1",
	})
	return iam.WithRawAccessToken(ctx, "raw-token")
}

func TestPluginCatalogListMergeInstalled(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	tenantID := "plugin-catalog-" + uuid.NewString()
	clusterID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM extensions WHERE target_id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status)
		VALUES ($1,$2,'kind-demo','Kind Demo','kubernetes','agent','online')`, clusterID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO extensions (id, name, version, provider_type, target_id, phase, manifest, created_at, updated_at)
		VALUES ($1,'calico','v3.32.1','cni',$2,'ready',$3,NOW(),NOW())`,
		uuid.NewString(), clusterID, `{"name":"calico"}`); err != nil {
		t.Fatal(err)
	}

	// Fake app-market returns a paginated public plugin catalog.
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"name": "calico", "display_name": "Calico", "description": "Calico CNI", "category": "tool", "labels": map[string]string{"plugin": "true", "plugin.category": "网络", "plugin.version": "v3.32.1"}},
				{"name": "hami", "display_name": "HAMi", "description": "HAMi", "category": "tool", "labels": map[string]string{"plugin": "true", "plugin.category": "GPU", "plugin.version": "v2.10.0"}},
				{"name": "mysql", "display_name": "MySQL", "description": "DB", "category": "database", "labels": map[string]string{}},
			},
			"total":    3,
			"page":     1,
			"pageSize": 100,
		})
	}))
	defer market.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugin-catalog?clusterId="+clusterID, nil)
	req = req.WithContext(pluginCatalogTestContext("subject-1", tenantID))
	rec := httptest.NewRecorder()

	NewPluginCatalogHandler(db, market.URL).List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var items []pluginCatalogItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (plugin=true only): %+v", len(items), items)
	}
	byName := map[string]pluginCatalogItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if calico := byName["calico"]; !calico.Installed || calico.Version != "v3.32.1" || calico.Category != "网络" {
		t.Fatalf("calico entry = %+v, want installed=true, v3.32.1, 网络", calico)
	}
	if hami := byName["hami"]; hami.Installed {
		t.Fatalf("hami should not be installed: %+v", hami)
	}
}

func TestPluginCatalogInstallUninstall(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	tenantID := "plugin-catalog-" + uuid.NewString()
	clusterID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM extensions WHERE target_id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status)
		VALUES ($1,$2,'kind-demo','Kind Demo','kubernetes','agent','online')`, clusterID, tenantID); err != nil {
		t.Fatal(err)
	}

	h := NewPluginCatalogHandler(db, "")

	// Install
	installReq := httptest.NewRequest(http.MethodPost, "/api/v1/plugin-catalog/installs",
		strings.NewReader(`{"name":"cilium","version":"v1.20.1","clusterId":"`+clusterID+`"}`))
	installReq = installReq.WithContext(pluginCatalogTestContext("subject-1", tenantID))
	installRec := httptest.NewRecorder()
	h.Install(installRec, installReq)
	if installRec.Code != http.StatusCreated {
		t.Fatalf("install status = %d body=%s", installRec.Code, installRec.Body.String())
	}

	// Extension exists and is pending.
	var extID, phase, providerType string
	if err := db.QueryRowContext(ctx, `SELECT id, phase, provider_type FROM extensions WHERE name='cilium' AND target_id=$1`, clusterID).
		Scan(&extID, &phase, &providerType); err != nil {
		t.Fatalf("extension lookup: %v", err)
	}
	if phase != "pending" || providerType != "plugin" {
		t.Fatalf("extension = (phase=%s, provider=%s), want pending/plugin (no market configured -> default)", phase, providerType)
	}

	// Uninstall (routed through a mux so {name} PathValue resolves)
	uninstallReq := httptest.NewRequest(http.MethodDelete, "/api/v1/plugin-catalog/installs/cilium?clusterId="+clusterID, nil)
	uninstallReq = uninstallReq.WithContext(pluginCatalogTestContext("subject-1", tenantID))
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/plugin-catalog/installs/{name}", h.Uninstall)
	uninstallRec := httptest.NewRecorder()
	mux.ServeHTTP(uninstallRec, uninstallReq)
	if uninstallRec.Code != http.StatusOK {
		t.Fatalf("uninstall status = %d body=%s", uninstallRec.Code, uninstallRec.Body.String())
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM extensions WHERE id=$1`, extID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("extension still present after uninstall: %d", count)
	}
}

func TestPluginCatalogInstallRoutesProviderFromCatalog(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	tenantID := "plugin-catalog-" + uuid.NewString()
	clusterID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM extensions WHERE target_id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status)
		VALUES ($1,$2,'kind-demo','Kind Demo','kubernetes','agent','online')`, clusterID, tenantID); err != nil {
		t.Fatal(err)
	}

	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"name": "cilium", "display_name": "Cilium", "category": "tool", "labels": map[string]string{"plugin": "true", "plugin.provider": "cni", "plugin.version": "v1.20.1"}},
			},
			"total": 1, "page": 1, "pageSize": 100,
		})
	}))
	defer market.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugin-catalog/installs",
		strings.NewReader(`{"name":"cilium","clusterId":"`+clusterID+`"}`))
	req = req.WithContext(pluginCatalogTestContext("subject-1", tenantID))
	rec := httptest.NewRecorder()
	NewPluginCatalogHandler(db, market.URL).Install(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("install status = %d body=%s", rec.Code, rec.Body.String())
	}

	var version, provider string
	if err := db.QueryRowContext(ctx, `SELECT version, provider_type FROM extensions WHERE name='cilium' AND target_id=$1`, clusterID).
		Scan(&version, &provider); err != nil {
		t.Fatalf("extension lookup: %v", err)
	}
	if provider != "cni" || version != "v1.20.1" {
		t.Fatalf("extension = (version=%s provider=%s), want v1.20.1/cni", version, provider)
	}
}

func TestPluginCatalogInstallForeignTenantDenied(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	tenantA := "plugin-catalog-" + uuid.NewString()
	tenantB := "plugin-catalog-" + uuid.NewString()
	clusterID := uuid.NewString()
	for _, tenantID := range []string{tenantA, tenantB} {
		if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM extensions WHERE target_id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, clusterID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id IN ($1,$2)`, tenantA, tenantB)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status)
		VALUES ($1,$2,'kind-demo','Kind Demo','kubernetes','agent','online')`, clusterID, tenantA); err != nil {
		t.Fatal(err)
	}

	// tenantB cannot install onto tenantA's cluster.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugin-catalog/installs",
		strings.NewReader(`{"name":"falco","version":"0.44.1","clusterId":"`+clusterID+`"}`))
	req = req.WithContext(pluginCatalogTestContext("subject-b", tenantB))
	rec := httptest.NewRecorder()
	NewPluginCatalogHandler(db, "").Install(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (cluster not in tenant) body=%s", rec.Code, rec.Body.String())
	}

	// A foreign uninstall should also fail or be a no-op.
	ureq := httptest.NewRequest(http.MethodDelete, "/api/v1/plugin-catalog/installs/falco?clusterId="+clusterID, nil)
	ureq = ureq.WithContext(pluginCatalogTestContext("subject-b", tenantB))
	umux := http.NewServeMux()
	umux.HandleFunc("DELETE /api/v1/plugin-catalog/installs/{name}", NewPluginCatalogHandler(db, "").Uninstall)
	urec := httptest.NewRecorder()
	umux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusNotFound {
		t.Fatalf("uninstall status = %d, want 404", urec.Code)
	}
}

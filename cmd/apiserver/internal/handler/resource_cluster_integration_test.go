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

func TestResourceClusterReadModel(t *testing.T) {
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
	tenantID := "resource-readmodel-" + uuid.NewString()
	targetID := uuid.NewString()
	nodeID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, targetID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status,
		                               lifecycle_state, health_state, connectivity_state, freshness_state,
		                               labels, observed_at, stale_threshold_seconds)
		VALUES ($1,$2,$3,$4,'kubernetes','agent','online',
		        'ACTIVE','HEALTHY','CONNECTED','FRESH',
		        $5, now(), 300)`,
		targetID, tenantID, "prod-k8s", "生产 K8s", `{"hnb.source":"imported"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO capability_snapshots (target_id, kube_version, cpu_cores, memory_mb, observed_at)
		VALUES ($1,'v1.30.2',32,131072, now())`, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_target_nodes (id, target_id, tenant_id, name, role, node_status,
		                                   health_state, connectivity_state,
		                                   ip_address, os, arch, cpu_allocatable, memory_allocatable, kubelet_version)
		VALUES ($1,$2,$3,'node-1','worker','Ready','HEALTHY','CONNECTED','10.0.0.1','Linux','amd64','16','64Gi','v1.30.2')`,
		nodeID, targetID, tenantID); err != nil {
		t.Fatal(err)
	}

	handler := NewResourceClusterHandler(db, "")
	trusted := iam.WithTrustedContext(ctx, iam.TrustedContext{
		SubjectID: "subject-a", TenantID: tenantID,
		ScopedPermissions: []iam.ScopedPermission{{TenantID: tenantID, ResourceKind: "cluster", Action: iam.ActionRead}},
	})

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters", nil).WithContext(trusted)
	rec := httptest.NewRecorder()
	handler.ListClusters(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var list clusterListPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("list total=%d items=%d", list.Total, len(list.Items))
	}
	item := list.Items[0]
	if item.ClusterID != targetID || item.Kind != "kubernetes" || item.Status != "RUNNING" || item.NodeCount != 1 {
		t.Fatalf("unexpected item: %+v", item)
	}
	if !strings.Contains(item.MemoryTotal, "GiB") || item.RuntimeVersion != "v1.30.2" {
		t.Fatalf("unexpected capability: %+v", item)
	}

	// Detail
	dreq := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters/"+targetID, nil).WithContext(trusted)
	dreq.SetPathValue("id", targetID)
	drec := httptest.NewRecorder()
	handler.GetCluster(drec, dreq)
	if drec.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", drec.Code, drec.Body.String())
	}
	var detail clusterSummary
	if err := json.Unmarshal(drec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ClusterID != targetID || detail.Source != "imported" {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	// Nodes
	nreq := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters/"+targetID+"/nodes", nil).WithContext(trusted)
	nreq.SetPathValue("id", targetID)
	nrec := httptest.NewRecorder()
	handler.ListClusterNodes(nrec, nreq)
	if nrec.Code != http.StatusOK {
		t.Fatalf("nodes status = %d body=%s", nrec.Code, nrec.Body.String())
	}
	var nodes clusterNodeListPayload
	if err := json.Unmarshal(nrec.Body.Bytes(), &nodes); err != nil {
		t.Fatal(err)
	}
	if nodes.Total != 1 || nodes.Items[0].Name != "node-1" || nodes.Items[0].Status != "Ready" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}

	// Cross-tenant isolation: another tenant must not see this target's nodes
	otherTrusted := iam.WithTrustedContext(ctx, iam.TrustedContext{
		SubjectID: "subject-b", TenantID: "tenant-other-" + uuid.NewString(),
		ScopedPermissions: []iam.ScopedPermission{{TenantID: "tenant-other-" + uuid.NewString(), ResourceKind: "cluster", Action: iam.ActionRead}},
	})
	oreq := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters/"+targetID+"/nodes", nil).WithContext(otherTrusted)
	oreq.SetPathValue("id", targetID)
	orec := httptest.NewRecorder()
	handler.ListClusterNodes(orec, oreq)
	var onodes clusterNodeListPayload
	if err := json.Unmarshal(orec.Body.Bytes(), &onodes); err != nil {
		t.Fatal(err)
	}
	if onodes.Total != 0 {
		t.Fatalf("cross-tenant nodes leak: %+v", onodes)
	}
}

func TestResourceClusterPluginStatusReadModel(t *testing.T) {
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
	tenantID := "resource-plugins-" + uuid.NewString()
	targetID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, targetID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, display_name, target_type, connection_type, status, labels, observed_at, stale_threshold_seconds)
		VALUES ($1,$2,$3,$4,'kubernetes','agent','online',$5, now(), 300)`,
		targetID, tenantID, "plugin-k8s", "插件 K8s", `{"hnb.source":"created"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO capability_snapshots (target_id, tenant_id, cni_plugins, csi_drivers, snapshot_json, observed_at)
		VALUES ($1,$2, ARRAY['ovnk8s','calico'], ARRAY['hostpath'], $3::jsonb, now())`,
		targetID, tenantID, `{"plugins":[{"name":"metallb","displayName":"metallb","status":"running"},{"name":"gpu-agent","status":"not-installed"}],"features":{"ipv6DualStack":true,"rdma":"absent"}}`); err != nil {
		t.Fatal(err)
	}

	handler := NewResourceClusterHandler(db, "")
	trusted := iam.WithTrustedContext(ctx, iam.TrustedContext{
		SubjectID: "subject-a", TenantID: tenantID,
		ScopedPermissions: []iam.ScopedPermission{{TenantID: tenantID, ResourceKind: "cluster", Action: iam.ActionRead}},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters/"+targetID+"/plugins", nil).WithContext(trusted)
	req.SetPathValue("id", targetID)
	rec := httptest.NewRecorder()
	handler.ListClusterPluginStatuses(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("plugins status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload clusterPluginStatusPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]pluginStatus{}
	for _, p := range payload.Items {
		byKey[p.Key] = p
	}
	if byKey["cni/ovnk8s"].Status != "installed" || byKey["cni/ovnk8s"].DisplayName == "" {
		t.Errorf("cni/ovnk8s = %+v", byKey["cni/ovnk8s"])
	}
	if byKey["cni/calico"].Status != "installed" {
		t.Errorf("cni/calico = %+v", byKey["cni/calico"])
	}
	if byKey["csi/hostpath"].Status != "installed" {
		t.Errorf("csi/hostpath = %+v", byKey["csi/hostpath"])
	}
	if byKey["plugin/metallb"].Status != "running" {
		t.Errorf("plugin/metallb = %+v", byKey["plugin/metallb"])
	}
	if byKey["plugin/gpu-agent"].Status != "not-installed" {
		t.Errorf("plugin/gpu-agent = %+v", byKey["plugin/gpu-agent"])
	}
	if byKey["feature/ipv6DualStack"].Status != "running" {
		t.Errorf("feature/ipv6DualStack = %+v", byKey["feature/ipv6DualStack"])
	}
	if byKey["feature/rdma"].Status != "not-installed" {
		t.Errorf("feature/rdma = %+v", byKey["feature/rdma"])
	}
	if payload.ObservedAt == nil {
		t.Errorf("expected observedAt to be set")
	}

	// Cross-tenant isolation: another tenant must not read this target's plugins
	otherTenantID := "tenant-other-" + uuid.NewString()
	otherTrusted := iam.WithTrustedContext(ctx, iam.TrustedContext{
		SubjectID: "subject-b", TenantID: otherTenantID,
		ScopedPermissions: []iam.ScopedPermission{{TenantID: otherTenantID, ResourceKind: "cluster", Action: iam.ActionRead}},
	})
	oreq := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters/"+targetID+"/plugins", nil).WithContext(otherTrusted)
	oreq.SetPathValue("id", targetID)
	orec := httptest.NewRecorder()
	handler.ListClusterPluginStatuses(orec, oreq)
	if orec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant plugins status = %d body=%s", orec.Code, orec.Body.String())
	}
}

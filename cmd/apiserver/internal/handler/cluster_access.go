package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

func clusterAccessibleTo(db *sql.DB, clusterID, tenantID, workspaceID string) bool {
	if db == nil || clusterID == "" || tenantID == "" {
		return false
	}
	var exists bool
	err := db.QueryRowContext(context.Background(),
		`SELECT EXISTS (
			SELECT 1 FROM runtime_targets WHERE id=$1 AND tenant_id=$2
			UNION ALL
			SELECT 1 FROM tenant_cluster_allocations WHERE cluster_id=$1 AND tenant_id=$2 AND status='active'
			UNION ALL
			SELECT 1 FROM cluster_shares WHERE cluster_id=$1 AND grantee_tenant_id=$2 AND (grantee_workspace_id IS NULL OR grantee_workspace_id::text=$3)
		)`, clusterID, tenantID, workspaceID).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func requireClusterAccess(db *sql.DB, w http.ResponseWriter, r *http.Request, clusterID, workspaceID string) bool {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok || !clusterAccessibleTo(db, clusterID, trusted.TenantID, workspaceID) {
		response.NotFound(w, "cluster not found")
		return false
	}
	return true
}

func trustedTenantID(r *http.Request) string {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		return ""
	}
	return trusted.TenantID
}

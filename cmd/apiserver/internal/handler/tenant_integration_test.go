package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestHandlersHideForeignTenantResources(t *testing.T) {
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
	tenantA, tenantB := "apiserver-a-"+uuid.NewString(), "apiserver-b-"+uuid.NewString()
	workspaceID, targetID := uuid.NewString(), uuid.NewString()
	for _, tenantID := range []string{tenantA, tenantB} {
		if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM runtime_targets WHERE id=$1`, targetID)
		_, _ = db.ExecContext(ctx, `DELETE FROM workspaces WHERE id=$1`, workspaceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id IN ($1,$2)`, tenantA, tenantB)
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, tenant_id, name) VALUES ($1,$2,'workspace')`, workspaceID, tenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime_targets (id, tenant_id, workspace_id, name, target_type) VALUES ($1,$2,$3,'target','kubernetes')`, targetID, tenantA, workspaceID); err != nil {
		t.Fatal(err)
	}

	trusted := iam.TrustedContext{SubjectID: "subject-b", TenantID: tenantB}
	clusterHandler := NewClusterHandler(db)
	for _, test := range []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{name: "get cluster", method: http.MethodGet, handler: clusterHandler.Get},
		{name: "delete cluster", method: http.MethodDelete, handler: clusterHandler.Delete},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/v1/clusters/"+targetID, nil)
			request.SetPathValue("id", targetID)
			request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
			recorder := httptest.NewRecorder()
			test.handler(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
		})
	}
	var targetExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM runtime_targets WHERE id=$1 AND tenant_id=$2)`, targetID, tenantA).Scan(&targetExists); err != nil || !targetExists {
		t.Fatalf("foreign delete changed target: exists=%v, err=%v", targetExists, err)
	}

	tenantHandler := NewTenantHandler(db)
	for _, test := range []struct {
		name    string
		path    string
		param   string
		value   string
		handler http.HandlerFunc
	}{
		{name: "namespaces", path: "/api/v1/workspaces/" + workspaceID + "/namespaces", param: "workspace_id", value: workspaceID, handler: tenantHandler.ListNamespaces},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.SetPathValue(test.param, test.value)
			request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
			recorder := httptest.NewRecorder()
			test.handler(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.String() != `{"code":0,"message":"success","data":[]}`+"\n" {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
		})
	}
}

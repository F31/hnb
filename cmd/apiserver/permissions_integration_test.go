package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

func TestIAMDBStoreResolvesCanonicalScopedPermissions(t *testing.T) {
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
	tenantID := "iam-permissions-" + uuid.NewString()
	subjectID, membershipID, policyID, roleID, bindingID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM scoped_role_bindings WHERE id=$1`, bindingID)
		_, _ = db.ExecContext(ctx, `DELETE FROM scoped_roles WHERE id=$1`, roleID)
		_, _ = db.ExecContext(ctx, `DELETE FROM authorization_policy_versions WHERE id=$1`, policyID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenant_memberships WHERE id=$1`, membershipID)
		_, _ = db.ExecContext(ctx, `DELETE FROM identity_subjects WHERE id=$1`, subjectID)
		_, _ = db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, []any{tenantID}},
		{`INSERT INTO identity_subjects (id, issuer, external_subject, subject_type) VALUES ($1,'https://issuer.example','external','user')`, []any{subjectID}},
		{`INSERT INTO tenant_memberships (id, tenant_id, subject_id) VALUES ($1,$2,$3)`, []any{membershipID, tenantID, subjectID}},
		{`INSERT INTO authorization_policy_versions (id, tenant_id, policy_key, version, policy_document, policy_digest, status) VALUES ($1,$2,'default',7,'{}','digest','active')`, []any{policyID, tenantID}},
		{`INSERT INTO scoped_roles (id, tenant_id, name, permissions) VALUES ($1,$2,'reader',$3)`, []any{roleID, tenantID, fmt.Sprintf(`[{"resourceKind":"operation","action":"read","tenantId":%q}]`, tenantID)}},
		{`INSERT INTO scoped_role_bindings (id, tenant_id, subject_id, role_id, scope_kind, actions) VALUES ($1,$2,$3,$4,'tenant',ARRAY['delete'])`, []any{bindingID, tenantID, subjectID, roleID}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	store := iam.NewIAMDBStore(db, "https://issuer.example")
	policyVersion, permissions, err := store.ResolvePermissions(ctx, subjectID, membershipID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if policyVersion != "default:7" || len(permissions) != 1 {
		t.Fatalf("snapshot = %q %+v", policyVersion, permissions)
	}
	if permissions[0].TenantID != tenantID || permissions[0].ResourceKind != "operation" || permissions[0].Action != iam.ActionDelete {
		t.Fatalf("permission = %+v", permissions[0])
	}

	if _, err := db.ExecContext(ctx, `UPDATE scoped_roles SET permissions='[{"resourceKind":"operation","action":"read","tenantId":"`+tenantID+`","unknown":true}]' WHERE id=$1`, roleID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ResolvePermissions(ctx, subjectID, membershipID, tenantID); err == nil {
		t.Fatal("unknown scoped_roles.permissions field was accepted")
	}
}

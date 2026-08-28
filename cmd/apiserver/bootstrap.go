package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

func bootstrapAdmin(ctx context.Context, db *sql.DB, password, issuer string) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("check users: %w", err)
	}
	if count > 0 {
		log.Println("[bootstrap] users table is not empty, skipping bootstrap")
		return nil
	}
	log.Println("[bootstrap] creating initial admin user, tenant, and permissions")

	tenantID := "default"
	userID := uuid.NewString()
	subjectID := uuid.NewString()
	membershipID := uuid.NewString()
	policyID := uuid.NewString()
	roleID := uuid.NewString()
	bindingID := uuid.NewString()
	rbBindingID := uuid.NewString()
	now := time.Now()

	hasher := iam.NewPasswordHasher()
	passwordHash, err := hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	actions := []string{"read", "list", "create", "update", "delete", "execute", "approve", "reject", "cancel", "switchTenant"}
	perms := make([]map[string]any, len(actions))
	for i, action := range actions {
		perms[i] = map[string]any{
			"resourceKind": "*",
			"action":       action,
			"tenantId":     tenantID,
		}
	}
	permBytes, _ := json.Marshal(perms)
	policyDoc := map[string]string{}
	policyDocBytes, _ := json.Marshal(policyDoc)
	policyDigest := sha256Hex(policyDocBytes)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenants (id, name, display_name, status, created_at, updated_at) VALUES ($1,$2,$3,'active',$4,$4)`,
			[]any{tenantID, "default", "Default Tenant", now}},
		{`INSERT INTO users (id, username, email, display_name, password_hash, source, is_active, created_at, updated_at) VALUES ($1,$2,'','admin',$3,'local',true,$4,$4)`,
			[]any{userID, "admin", passwordHash, now}},
		{`INSERT INTO identity_subjects (id, issuer, external_subject, subject_type, display_name, created_at, updated_at) VALUES ($1,$2,$3,'user','admin',$4,$4)`,
			[]any{subjectID, issuer, userID, now}},
		{`INSERT INTO tenant_memberships (id, tenant_id, subject_id, status, is_default, valid_from, created_at, updated_at) VALUES ($1,$2,$3,'active',true,$4,$4,$4)`,
			[]any{membershipID, tenantID, subjectID, now}},
		{`INSERT INTO authorization_policy_versions (id, tenant_id, policy_key, version, policy_document, policy_digest, status, activated_at, created_at) VALUES ($1,$2,'default',1,$3,$4,'active',$5,$5)`,
			[]any{policyID, tenantID, string(policyDocBytes), policyDigest, now}},
		{`INSERT INTO scoped_roles (id, tenant_id, name, permissions, is_active, created_at, updated_at) VALUES ($1,$2,'admin',$3,true,$4,$4)`,
			[]any{roleID, tenantID, string(permBytes), now}},
		{`INSERT INTO scoped_role_bindings (id, tenant_id, subject_id, role_id, scope_kind, actions, granted_at) VALUES ($1,$2,$3,$4,'tenant',ARRAY['read','list','create','update','delete','execute'],$5)`,
			[]any{bindingID, tenantID, subjectID, roleID, now}},
		{`INSERT INTO role_bindings (id, role_id, user_id, scope, created_at) VALUES ($1,'admin',$2,'global',$3)`,
			[]any{rbBindingID, userID, now}},
	}

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			return fmt.Errorf("bootstrap: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bootstrap: %w", err)
	}

	log.Printf("[bootstrap] admin user created: username=admin, tenant=default, password=<%s>", password)
	return nil
}

func sha256Hex(data []byte) string {
	d := sha256.Sum256(data)
	return hex.EncodeToString(d[:])
}
package iam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type IAMDBStore struct {
	db             *sql.DB
	identityIssuer string
}

func NewIAMDBStore(db *sql.DB, identityIssuer ...string) *IAMDBStore {
	store := &IAMDBStore{db: db}
	if len(identityIssuer) > 0 {
		store.identityIssuer = identityIssuer[0]
	}
	return store
}

// UserStore implementation

func (s *IAMDBStore) GetByUsername(username string) (*User, error) {
	var user User
	var email, phone, displayName, sourceID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, username, email, phone, display_name, password_hash, source, source_id, is_active, created_at, updated_at
		FROM users WHERE username = $1`, username).
		Scan(&user.ID, &user.Username, &email, &phone, &displayName, &user.PasswordHash,
			&user.Source, &sourceID, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if email.Valid {
		user.Email = email.String
	}
	if phone.Valid {
		user.Phone = phone.String
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	if sourceID.Valid {
		user.SourceID = sourceID.String
	}
	return &user, nil
}

func (s *IAMDBStore) GetByID(id string) (*User, error) {
	var user User
	var email, phone, displayName, sourceID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, username, email, phone, display_name, password_hash, source, source_id, is_active, created_at, updated_at
		FROM users WHERE id = $1`, id).
		Scan(&user.ID, &user.Username, &email, &phone, &displayName, &user.PasswordHash,
			&user.Source, &sourceID, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if email.Valid {
		user.Email = email.String
	}
	if phone.Valid {
		user.Phone = phone.String
	}
	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	if sourceID.Valid {
		user.SourceID = sourceID.String
	}
	return &user, nil
}

func (s *IAMDBStore) CreateUser(user *User) error {
	_, err := s.db.Exec(`
		INSERT INTO users (id, username, email, phone, display_name, password_hash, source, source_id, is_active, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7, NULLIF($8, ''), $9, $10, $11)`,
		user.ID, user.Username, user.Email, user.Phone, user.DisplayName, user.PasswordHash,
		user.Source, user.SourceID, user.IsActive, time.Now(), time.Now())
	return err
}

func (s *IAMDBStore) UpdateUser(user *User) error {
	_, err := s.db.Exec(`
		UPDATE users SET email = NULLIF($2, ''), phone = NULLIF($3, ''), display_name = NULLIF($4, ''), is_active = $5, updated_at = NOW()
		WHERE id = $1`, user.ID, user.Email, user.Phone, user.DisplayName, user.IsActive)
	return err
}

func (s *IAMDBStore) UpdatePasswordHash(id, hash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, hash)
	return err
}

func (s *IAMDBStore) CountUsers() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *IAMDBStore) DeleteUser(id string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = $1`, id)
	return err
}

func (s *IAMDBStore) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, username, email, phone, display_name, source, source_id, is_active, created_at, updated_at
		FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var email, phone, displayName, sourceID sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &email, &phone, &displayName,
			&u.Source, &sourceID, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		if email.Valid {
			u.Email = email.String
		}
		if phone.Valid {
			u.Phone = phone.String
		}
		if displayName.Valid {
			u.DisplayName = displayName.String
		}
		if sourceID.Valid {
			u.SourceID = sourceID.String
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *IAMDBStore) ResolveUserIdentity(ctx context.Context, userID, membershipSelector string) (*Identity, error) {
	if s.identityIssuer == "" {
		return nil, errors.New("identity issuer is not configured")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id::text, s.subject_type, m.id::text, m.tenant_id, m.is_default
		FROM identity_subjects s
		JOIN users u ON u.id = s.external_subject
		JOIN tenant_memberships m ON m.subject_id = s.id
		WHERE s.issuer = $1 AND s.external_subject = $2
		  AND NOT s.is_disabled AND u.is_active
		  AND m.status = 'active' AND m.valid_from <= NOW()
		  AND (m.valid_until IS NULL OR m.valid_until > NOW())
		ORDER BY m.is_default DESC, m.id`, s.identityIssuer, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		identity  Identity
		isDefault bool
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		item.identity.UserID = userID
		if err := rows.Scan(&item.identity.SubjectID, &item.identity.SubjectType, &item.identity.MembershipID, &item.identity.TenantID, &item.isDefault); err != nil {
			return nil, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if membershipSelector != "" {
		for i := range candidates {
			if candidates[i].identity.MembershipID == membershipSelector {
				return &candidates[i].identity, nil
			}
		}
		return nil, ErrMembershipMismatch
	}
	if len(candidates) == 1 {
		return &candidates[0].identity, nil
	}
	var selected *Identity
	for i := range candidates {
		if candidates[i].isDefault {
			if selected != nil {
				return nil, errors.New("multiple default tenant memberships")
			}
			identity := candidates[i].identity
			selected = &identity
		}
	}
	if selected != nil {
		return selected, nil
	}
	return nil, ErrNoAuthorizedTenant
}

func (s *IAMDBStore) ResolveMembership(ctx context.Context, subjectID, membershipID string) (*Identity, error) {
	var identity Identity
	err := s.db.QueryRowContext(ctx, `
		SELECT s.external_subject, s.id::text, s.subject_type, m.id::text, m.tenant_id
		FROM identity_subjects s
		JOIN tenant_memberships m ON m.subject_id = s.id
		LEFT JOIN users u ON s.subject_type = 'user' AND u.id = s.external_subject
		WHERE s.id = $1 AND m.id = $2 AND s.issuer = $3 AND NOT s.is_disabled
		  AND (s.subject_type <> 'user' OR u.is_active)
		  AND m.status = 'active' AND m.valid_from <= NOW()
		  AND (m.valid_until IS NULL OR m.valid_until > NOW())`, subjectID, membershipID, s.identityIssuer).
		Scan(&identity.UserID, &identity.SubjectID, &identity.SubjectType, &identity.MembershipID, &identity.TenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMembershipMismatch
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

func (s *IAMDBStore) CreateRefreshToken(ctx context.Context, token RefreshTokenRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_refresh_tokens (token_hash, purpose, subject_id, membership_id, expires_at)
		VALUES ($1, 'refresh', $2, $3, $4)`, token.TokenHash, token.SubjectID, token.MembershipID, token.ExpiresAt)
	return err
}

func (s *IAMDBStore) RotateRefreshToken(ctx context.Context, oldHash string, replacement RefreshTokenRecord, now time.Time) (*RefreshTokenRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var old RefreshTokenRecord
	err = tx.QueryRowContext(ctx, `
		SELECT token_hash, subject_id::text, membership_id::text, expires_at
		FROM auth_refresh_tokens
		WHERE token_hash = $1 AND purpose = 'refresh' AND consumed_at IS NULL AND expires_at > $2
		FOR UPDATE`, oldHash, now).
		Scan(&old.TokenHash, &old.SubjectID, &old.MembershipID, &old.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRefreshReplay
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET consumed_at = $2 WHERE token_hash = $1`, oldHash, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO auth_refresh_tokens (token_hash, purpose, subject_id, membership_id, expires_at)
		VALUES ($1, 'refresh', $2, $3, $4)`, replacement.TokenHash, old.SubjectID, old.MembershipID, replacement.ExpiresAt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &old, nil
}

// RoleBinding store

func (s *IAMDBStore) SaveRoleBinding(binding *RoleBinding) error {
	_, err := s.db.Exec(`
		INSERT INTO role_bindings (id, role_id, user_id, scope, scope_id, created_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		ON CONFLICT (user_id, scope, COALESCE(scope_id, '')) DO UPDATE SET
			role_id = EXCLUDED.role_id, updated_at = NOW()`,
		binding.ID, binding.RoleID, binding.UserID, string(binding.Scope),
		binding.ScopeID, time.Now())
	return err
}

func (s *IAMDBStore) DeleteRoleBinding(userID string, scope RoleScope, scopeID string) error {
	_, err := s.db.Exec(`
		DELETE FROM role_bindings
		WHERE user_id = $1 AND scope = $2 AND (scope_id = $3 OR (scope_id IS NULL AND $3 = ''))`,
		userID, string(scope), scopeID)
	return err
}

func (s *IAMDBStore) ListRoleBindings() ([]RoleBinding, error) {
	return s.listRoleBindings("")
}

func (s *IAMDBStore) ListRoleBindingsByUser(userID string) ([]RoleBinding, error) {
	return s.listRoleBindings(userID)
}

func (s *IAMDBStore) listRoleBindings(userID string) ([]RoleBinding, error) {
	var rows *sql.Rows
	var err error
	if userID != "" {
		rows, err = s.db.Query(`
			SELECT id, role_id, user_id, scope, scope_id, created_at
			FROM role_bindings WHERE user_id = $1 ORDER BY user_id, scope`, userID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, role_id, user_id, scope, scope_id, created_at
			FROM role_bindings ORDER BY user_id, scope`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []RoleBinding
	for rows.Next() {
		var b RoleBinding
		var scopeID sql.NullString
		if err := rows.Scan(&b.ID, &b.RoleID, &b.UserID, &b.Scope, &scopeID, &b.CreatedAt); err != nil {
			continue
		}
		if scopeID.Valid {
			b.ScopeID = scopeID.String
		}
		bindings = append(bindings, b)
	}
	return bindings, nil
}

// Ensure interface satisfaction
var _ UserStore = (*IAMDBStore)(nil)
var _ IdentityResolver = (*IAMDBStore)(nil)
var _ PermissionResolver = (*IAMDBStore)(nil)
var _ RefreshTokenStore = (*IAMDBStore)(nil)

// Migration SQL
var MigrationCreateUsers = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT,
    phone TEXT,
    display_name TEXT,
    password_hash TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'local',
    source_id TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    labels JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

var MigrationAddUserPhone = `ALTER TABLE users ADD COLUMN IF NOT EXISTS phone TEXT;`

var MigrationCreateRefreshTokens = `
CREATE TABLE IF NOT EXISTS auth_refresh_tokens (
    token_hash TEXT PRIMARY KEY CHECK (length(token_hash) = 64),
    purpose TEXT NOT NULL CHECK (purpose = 'refresh'),
    subject_id UUID NOT NULL REFERENCES identity_subjects(id) ON DELETE RESTRICT,
    membership_id UUID NOT NULL REFERENCES tenant_memberships(id) ON DELETE RESTRICT,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

var MigrationCreateRoleBindings = `
CREATE TABLE IF NOT EXISTS role_bindings (
    id TEXT PRIMARY KEY,
    role_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('global', 'workspace', 'cluster', 'project')),
    scope_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

var MigrationCreateRoleBindingsUniqueIndex = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_bindings_user_scope
ON role_bindings (user_id, scope, COALESCE(scope_id, ''));`

func (s *IAMDBStore) Migrate() error {
	for _, m := range []string{MigrationCreateUsers, MigrationAddUserPhone, MigrationCreateRefreshTokens, MigrationCreateRoleBindings, MigrationCreateRoleBindingsUniqueIndex} {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if s.identityIssuer == "" {
		return errors.New("migrate: identity issuer is required")
	}
	if _, err := s.db.Exec(`
		INSERT INTO identity_subjects (issuer, external_subject, subject_type, display_name, is_disabled, disabled_at)
		SELECT $1, u.id, 'user', COALESCE(NULLIF(u.display_name, ''), u.username), NOT u.is_active,
		       CASE WHEN u.is_active THEN NULL ELSE NOW() END
		FROM users u
		ON CONFLICT (issuer, external_subject) DO NOTHING`, s.identityIssuer); err != nil {
		return fmt.Errorf("migrate identity user bridge: %w", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO tenant_memberships (tenant_id, subject_id, status, is_default)
		SELECT ur.tenant_id, s.id, 'active', COUNT(*) OVER (PARTITION BY ur.user_id) = 1
		FROM (SELECT DISTINCT user_id, tenant_id FROM user_roles WHERE revoked_at IS NULL) ur
		JOIN identity_subjects s ON s.issuer = $1 AND s.external_subject = ur.user_id
		ON CONFLICT (tenant_id, subject_id) DO NOTHING`, s.identityIssuer); err != nil {
		return fmt.Errorf("migrate tenant memberships: %w", err)
	}
	return nil
}

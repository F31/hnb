package iam

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func (s *IAMDBStore) ResolvePermissions(ctx context.Context, subjectID, membershipID, tenantID string) (string, []ScopedPermission, error) {
	var membershipExists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM tenant_memberships
			WHERE id = $1 AND subject_id = $2 AND tenant_id = $3
			  AND status = 'active' AND valid_from <= NOW()
			  AND (valid_until IS NULL OR valid_until > NOW())
		)`, membershipID, subjectID, tenantID).Scan(&membershipExists)
	if err != nil {
		return "", nil, err
	}
	if !membershipExists {
		return "", nil, ErrMembershipMismatch
	}

	policyRows, err := s.db.QueryContext(ctx, `
		SELECT policy_key, version
		FROM authorization_policy_versions
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY policy_key`, tenantID)
	if err != nil {
		return "", nil, err
	}
	defer policyRows.Close()
	var policyVersion string
	for policyRows.Next() {
		var key string
		var version int64
		if err := policyRows.Scan(&key, &version); err != nil {
			return "", nil, err
		}
		if policyVersion != "" {
			policyVersion += ","
		}
		policyVersion += fmt.Sprintf("%s:%d", key, version)
	}
	if err := policyRows.Err(); err != nil {
		return "", nil, err
	}
	if policyVersion == "" {
		return "", nil, errors.New("active authorization policy is missing")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT r.permissions, b.scope_kind, b.workspace_id::text,
		       b.namespace_id, b.resource_kind, b.resource_id,
		       array_to_json(b.actions)::text
		FROM scoped_role_bindings b
		JOIN scoped_roles r ON r.id = b.role_id AND r.tenant_id = b.tenant_id
		WHERE b.tenant_id = $1 AND b.subject_id = $2 AND b.revoked_at IS NULL AND r.is_active
		ORDER BY b.id`, tenantID, subjectID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	permissions := make([]ScopedPermission, 0)
	for rows.Next() {
		var rawPermissions []byte
		var scopeKind string
		var workspaceID, namespaceID, resourceKind, resourceID sql.NullString
		var rawActions string
		if err := rows.Scan(&rawPermissions, &scopeKind, &workspaceID, &namespaceID, &resourceKind, &resourceID, &rawActions); err != nil {
			return "", nil, err
		}
		rolePermissions, err := decodeRolePermissions(rawPermissions)
		if err != nil {
			return "", nil, fmt.Errorf("invalid scoped_roles.permissions: %w", err)
		}
		var actions []AuthorizationAction
		if err := json.Unmarshal([]byte(rawActions), &actions); err != nil {
			return "", nil, fmt.Errorf("invalid scoped_role_bindings.actions: %w", err)
		}
		for _, permission := range rolePermissions {
			if permission.TenantID != tenantID {
				return "", nil, errors.New("role permission tenant does not match role binding")
			}
		}
		if len(actions) > 0 && len(rolePermissions) == 0 {
			if !resourceKind.Valid || !resourceID.Valid {
				return "", nil, errors.New("binding actions without role permissions require an explicit resource scope")
			}
			rolePermissions = []ScopedPermission{{ResourceKind: resourceKind.String, ResourceID: resourceID.String, TenantID: tenantID}}
		}
		for _, base := range rolePermissions {
			bindingActions := actions
			if len(bindingActions) == 0 {
				bindingActions = []AuthorizationAction{base.Action}
			}
			for _, action := range bindingActions {
				base.Action = action
				permission, err := applyBindingScope(base, scopeKind, workspaceID.String, namespaceID.String, resourceKind.String, resourceID.String)
				if err != nil {
					return "", nil, err
				}
				permissions = append(permissions, permission)
			}
		}
		if len(permissions) > MaxScopedPermissions {
			return "", nil, errors.New("permission snapshot exceeds maximum entries")
		}
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	permissions = deduplicatePermissions(permissions)
	if err := ValidatePermissionSnapshot(policyVersion, permissions, tenantID); err != nil {
		return "", nil, err
	}
	return policyVersion, permissions, nil
}

func decodeRolePermissions(raw []byte) ([]ScopedPermission, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var permissions []ScopedPermission
	if err := decoder.Decode(&permissions); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("permissions must contain one JSON array")
	}
	if permissions == nil {
		return nil, errors.New("permissions must be a JSON array")
	}
	return permissions, nil
}

func applyBindingScope(permission ScopedPermission, scopeKind, workspaceID, namespaceID, resourceKind, resourceID string) (ScopedPermission, error) {
	switch scopeKind {
	case "tenant":
	case "workspace":
		if workspaceID == "" {
			return ScopedPermission{}, errors.New("workspace binding is missing workspace_id")
		}
		permission.ResourceKind, permission.ResourceID = "workspace", workspaceID
	case "namespace":
		if namespaceID == "" {
			return ScopedPermission{}, errors.New("namespace binding is missing namespace_id")
		}
		permission.ResourceKind, permission.ResourceID = "namespace", namespaceID
	case "resource":
		permission.ResourceKind, permission.ResourceID = resourceKind, resourceID
	default:
		return ScopedPermission{}, errors.New("unknown binding scope")
	}
	return permission, nil
}

func deduplicatePermissions(permissions []ScopedPermission) []ScopedPermission {
	seen := make(map[ScopedPermission]struct{}, len(permissions))
	result := make([]ScopedPermission, 0, len(permissions))
	for _, permission := range permissions {
		if _, exists := seen[permission]; exists {
			continue
		}
		seen[permission] = struct{}{}
		result = append(result, permission)
	}
	return result
}

var _ PermissionResolver = (*IAMDBStore)(nil)

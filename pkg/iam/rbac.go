package iam

import (
	"fmt"
	"sync"
)

type RBACEngine struct {
	mu           sync.RWMutex
	roles        map[string]*Role
	roleBindings map[string]*RoleBinding
}

func NewRBACEngine() *RBACEngine {
	e := &RBACEngine{
		roles:        make(map[string]*Role),
		roleBindings: make(map[string]*RoleBinding),
	}
	e.initBuiltInRoles()
	return e
}

func (e *RBACEngine) initBuiltInRoles() {
	builtInRoles := []Role{
		{
			ID: "admin", Name: "admin", DisplayName: "Administrator",
			Scope: ScopeGlobal, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"*"}, Resources: []string{"*"}}},
		},
		{
			ID: "workspace-admin", Name: "workspace-admin", DisplayName: "Workspace Admin",
			Scope: ScopeWorkspace, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"*"}, Resources: []string{"workspaces", "projects", "clusters", "extensions"}}},
		},
		{
			ID: "workspace-viewer", Name: "workspace-viewer", DisplayName: "Workspace Viewer",
			Scope: ScopeWorkspace, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"get", "list"}, Resources: []string{"workspaces", "projects", "clusters", "extensions"}}},
		},
		{
			ID: "cluster-admin", Name: "cluster-admin", DisplayName: "Cluster Admin",
			Scope: ScopeCluster, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"*"}, Resources: []string{"*"}}},
		},
		{
			ID: "cluster-viewer", Name: "cluster-viewer", DisplayName: "Cluster Viewer",
			Scope: ScopeCluster, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"get", "list"}, Resources: []string{"*"}}},
		},
		{
			ID: "project-admin", Name: "project-admin", DisplayName: "Project Admin",
			Scope: ScopeProject, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"*"}, Resources: []string{"*"}}},
		},
		{
			ID: "project-viewer", Name: "project-viewer", DisplayName: "Project Viewer",
			Scope: ScopeProject, BuiltIn: true,
			Rules: []PolicyRule{{Verbs: []string{"get", "list"}, Resources: []string{"*"}}},
		},
	}

	for i := range builtInRoles {
		r := builtInRoles[i]
		e.roles[r.ID] = &r
	}
}

func (e *RBACEngine) CreateRole(role *Role) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.roles[role.ID]; exists {
		return fmt.Errorf("role %q already exists", role.ID)
	}
	e.roles[role.ID] = role
	return nil
}

func (e *RBACEngine) GetRole(id string) (*Role, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	role, exists := e.roles[id]
	if !exists {
		return nil, fmt.Errorf("role %q not found", id)
	}
	return role, nil
}

func (e *RBACEngine) DeleteRole(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.roles[id]; !exists {
		return fmt.Errorf("role %q not found", id)
	}
	if e.roles[id].BuiltIn {
		return fmt.Errorf("cannot delete built-in role %q", id)
	}
	delete(e.roles, id)
	return nil
}

func (e *RBACEngine) ListRoles() []*Role {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*Role, 0, len(e.roles))
	for _, role := range e.roles {
		result = append(result, role)
	}
	return result
}

func (e *RBACEngine) BindRole(binding *RoleBinding) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.roles[binding.RoleID]; !exists {
		return fmt.Errorf("role %q not found", binding.RoleID)
	}

	key := bindingKey(binding.UserID, binding.Scope, binding.ScopeID)
	if _, exists := e.roleBindings[key]; exists {
		return fmt.Errorf("binding already exists for user %s in scope %s/%s", binding.UserID, binding.Scope, binding.ScopeID)
	}

	e.roleBindings[key] = binding
	return nil
}

func (e *RBACEngine) UnbindRole(userID string, scope RoleScope, scopeID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := bindingKey(userID, scope, scopeID)
	if _, exists := e.roleBindings[key]; !exists {
		return fmt.Errorf("binding not found")
	}
	delete(e.roleBindings, key)
	return nil
}

func (e *RBACEngine) GetUserRoles(userID string) []*RoleBinding {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*RoleBinding
	for _, binding := range e.roleBindings {
		if binding.UserID == userID {
			result = append(result, binding)
		}
	}
	return result
}

func (e *RBACEngine) HasPermission(userID, verb, resource string, scope RoleScope, scopeID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check direct binding
	key := bindingKey(userID, scope, scopeID)
	if binding, exists := e.roleBindings[key]; exists {
		if role, ok := e.roles[binding.RoleID]; ok {
			if e.ruleAllows(role.Rules, verb, resource) {
				return true
			}
		}
	}

	// Check global role
	globalKey := bindingKey(userID, ScopeGlobal, "")
	if binding, exists := e.roleBindings[globalKey]; exists {
		if role, ok := e.roles[binding.RoleID]; ok {
			if e.ruleAllows(role.Rules, verb, resource) {
				return true
			}
		}
	}

	return false
}

func (e *RBACEngine) ruleAllows(rules []PolicyRule, verb, resource string) bool {
	for _, rule := range rules {
		if !matchesVerb(rule.Verbs, verb) {
			continue
		}
		if !matchesResource(rule.Resources, resource) {
			continue
		}
		return true
	}
	return false
}

func matchesVerb(verbs []string, verb string) bool {
	for _, v := range verbs {
		if v == "*" || v == verb {
			return true
		}
	}
	return false
}

func matchesResource(resources []string, resource string) bool {
	for _, r := range resources {
		if r == "*" || r == resource {
			return true
		}
	}
	return false
}

func bindingKey(userID string, scope RoleScope, scopeID string) string {
	return fmt.Sprintf("%s:%s:%s", userID, scope, scopeID)
}

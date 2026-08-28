package iam

import "time"

type User struct {
	ID           string            `json:"id"`
	Username     string            `json:"username"`
	Email        string            `json:"email,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	DisplayName  string            `json:"display_name,omitempty"`
	PasswordHash string            `json:"-"`                   // never expose
	Source       string            `json:"source"`              // local, ldap, oidc, github
	SourceID     string            `json:"source_id,omitempty"` // external ID from IdP
	IsActive     bool              `json:"is_active"`
	Labels       map[string]string `json:"labels,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type RoleScope string

const (
	ScopeGlobal    RoleScope = "global"
	ScopeWorkspace RoleScope = "workspace"
	ScopeCluster   RoleScope = "cluster"
	ScopeProject   RoleScope = "project"
)

type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name,omitempty"`
	Scope       RoleScope    `json:"scope"`
	Rules       []PolicyRule `json:"rules"`
	BuiltIn     bool         `json:"built_in"`
	CreatedAt   time.Time    `json:"created_at"`
}

type PolicyRule struct {
	Verbs           []string `json:"verbs"`
	Resources       []string `json:"resources"`
	ResourceNames   []string `json:"resource_names,omitempty"`
	NonResourceURLs []string `json:"non_resource_urls,omitempty"`
}

type RoleBinding struct {
	ID        string    `json:"id"`
	RoleID    string    `json:"role_id"`
	UserID    string    `json:"user_id"`
	Scope     RoleScope `json:"scope"`
	ScopeID   string    `json:"scope_id,omitempty"` // workspace/cluster/project ID
	CreatedAt time.Time `json:"created_at"`
}

type AuthProviderType string

const (
	AuthLocal AuthProviderType = "local"
	AuthLDAP  AuthProviderType = "ldap"
	AuthOIDC  AuthProviderType = "oidc"
	AuthOAuth AuthProviderType = "oauth"
)

type AuthProvider struct {
	Type      AuthProviderType `json:"type"`
	Name      string           `json:"name"`
	Config    map[string]any   `json:"config"`
	IsEnabled bool             `json:"is_enabled"`
}

type Token struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

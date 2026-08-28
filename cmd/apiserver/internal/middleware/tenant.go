package middleware

import (
	"database/sql"
	"net/http"

	"github.com/F31/hnb/pkg/iam"
)

type TenantContextMW struct {
	db          *sql.DB
	bypassPaths []string
}

func NewTenantContext(db *sql.DB, bypassPaths []string) *TenantContextMW {
	return &TenantContextMW{
		db:          db,
		bypassPaths: bypassPaths,
	}
}

func (m *TenantContextMW) Name() string { return "tenant_context" }

func (m *TenantContextMW) Handle(ctx *Context, next func()) {
	path := ctx.Request.URL.Path
	for _, bp := range m.bypassPaths {
		if path == bp {
			next()
			return
		}
	}

	trusted, ok := iam.TrustedContextFrom(ctx.Request.Context())
	if !ok {
		ctx.Abort(http.StatusUnauthorized, []byte(`{"code":40100,"message":"tenant context required"}`))
		return
	}
	tenantID := trusted.TenantID
	ctx.TenantID = trusted.TenantID
	ctx.UserID = trusted.SubjectID

	// Optionally resolve workspace membership from DB
	workspaceID := resolveWorkspace(m.db, ctx.UserID, tenantID)
	if workspaceID != "" {
		ctx.WorkspaceID = workspaceID
	}

	next()
}

func resolveWorkspace(db *sql.DB, userID, tenantID string) string {
	if db == nil || userID == "" {
		return ""
	}
	var wsID sql.NullString
	err := db.QueryRow(`
		SELECT w.id FROM workspaces w
		JOIN role_bindings rb ON rb.scope_id = w.id
		WHERE rb.user_id = $1 AND w.tenant_id = $2 AND rb.scope = 'workspace'
		LIMIT 1`, userID, tenantID).Scan(&wsID)
	if err != nil || !wsID.Valid {
		return ""
	}
	return wsID.String
}

func (c *Context) SetWorkspaceID(id string) { c.WorkspaceID = id }

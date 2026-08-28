package mapper

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

var platformRoleToClusterRole = map[string]string{
	"platform_admin": "cluster-admin",
	"tenant_admin":   "hnb:tenant-admin",
	"project_admin":  "hnb:project-admin",
	"operator":       "hnb:operator",
	"publisher":      "hnb:publisher",
	"readonly":       "view",
}

func RoleNameToClusterRole(roleName string) (string, error) {
	cr, ok := platformRoleToClusterRole[roleName]
	if !ok {
		return "", fmt.Errorf("unknown platform role: %s", roleName)
	}
	return cr, nil
}

func IsManagedRoleBinding(name string) bool {
	prefixes := []string{"hnb:"}
	for _, p := range prefixes {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}

type NamespaceFinder struct {
	db    *sql.DB
	cache sync.Map
	ttl   time.Duration
}

func NewNamespaceFinder(db *sql.DB) *NamespaceFinder {
	return &NamespaceFinder{
		db:  db,
		ttl: 5 * time.Minute,
	}
}

type nsCacheEntry struct {
	namespaces []string
	expiresAt  time.Time
}

func (f *NamespaceFinder) FindNamespaces(ctx context.Context, tenantID string, projectID *string) ([]string, error) {
	if projectID != nil {
		return f.findByProject(ctx, tenantID, *projectID)
	}
	return f.findByTenant(ctx, tenantID)
}

func (f *NamespaceFinder) findByTenant(ctx context.Context, tenantID string) ([]string, error) {
	cacheKey := "tenant:" + tenantID
	if cached, ok := f.cache.Load(cacheKey); ok {
		entry := cached.(nsCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.namespaces, nil
		}
	}

	query := `SELECT name FROM namespaces WHERE tenant_id = $1 AND status = 'active'`
	rows, err := f.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query namespaces by tenant: %w", err)
	}
	defer rows.Close()

	var namespaces []string
	for rows.Next() {
		var ns string
		if err := rows.Scan(&ns); err != nil {
			return nil, err
		}
		namespaces = append(namespaces, ns)
	}

	f.cache.Store(cacheKey, nsCacheEntry{
		namespaces: namespaces,
		expiresAt:  time.Now().Add(f.ttl),
	})

	return namespaces, nil
}

func (f *NamespaceFinder) findByProject(ctx context.Context, tenantID, projectID string) ([]string, error) {
	cacheKey := "project:" + tenantID + ":" + projectID
	if cached, ok := f.cache.Load(cacheKey); ok {
		entry := cached.(nsCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.namespaces, nil
		}
	}

	query := `SELECT name FROM namespaces WHERE tenant_id = $1 AND project_id = $2 AND status = 'active'`
	rows, err := f.db.QueryContext(ctx, query, tenantID, projectID)
	if err != nil {
		return nil, fmt.Errorf("query namespaces by project: %w", err)
	}
	defer rows.Close()

	var namespaces []string
	for rows.Next() {
		var ns string
		if err := rows.Scan(&ns); err != nil {
			return nil, err
		}
		namespaces = append(namespaces, ns)
	}

	f.cache.Store(cacheKey, nsCacheEntry{
		namespaces: namespaces,
		expiresAt:  time.Now().Add(f.ttl),
	})

	return namespaces, nil
}

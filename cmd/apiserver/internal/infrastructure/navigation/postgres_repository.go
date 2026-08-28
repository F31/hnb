package navigation

import (
	"context"
	"database/sql"
	"strings"

	navapp "github.com/F31/hnb/cmd/apiserver/internal/application/navigation"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

const defaultLocale = "zh-CN"

func (r *PostgresRepository) Snapshot(ctx context.Context, tenantID string, locale string) (navapp.Snapshot, error) {
	plugins, capabilities, err := r.loadPlugins(ctx)
	if err != nil {
		return navapp.Snapshot{}, err
	}
	routes, routesByName, err := r.loadRoutes(ctx)
	if err != nil {
		return navapp.Snapshot{}, err
	}
	menus, err := r.loadMenus(ctx, routesByName, normalizeLocale(locale))
	if err != nil {
		return navapp.Snapshot{}, err
	}
	versions, err := r.loadVersions(ctx)
	if err != nil {
		return navapp.Snapshot{}, err
	}
	_ = tenantID
	return navapp.Snapshot{Versions: versions, Capabilities: capabilities, Plugins: plugins, Menus: menus, Routes: routes}, nil
}

func (r *PostgresRepository) loadPlugins(ctx context.Context) ([]navapp.PluginManifestRef, map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT plugin_id, version, display_name, tier, mode, enabled
FROM console_plugins
WHERE enabled = true
ORDER BY sort_order, plugin_id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	plugins := make([]navapp.PluginManifestRef, 0)
	capabilities := make(map[string]bool)
	for rows.Next() {
		var plugin navapp.PluginManifestRef
		if err := rows.Scan(&plugin.Name, &plugin.Version, &plugin.DisplayName, &plugin.Tier, &plugin.Mode, &plugin.Enabled); err != nil {
			return nil, nil, err
		}
		plugin.ID = plugin.Name
		plugin.Permissions = navapp.PluginPermissions{Required: []string{}}
		plugin.Capabilities = navapp.PluginCapabilities{Required: []string{}}
		plugin.Dependencies = navapp.PluginDependencies{Backend: []string{}}
		plugin.Menu = navapp.PluginManifestMenu{Group: plugin.DisplayName, Items: []navapp.Item{}}
		plugins = append(plugins, plugin)
		capabilities[plugin.Name] = true
	}
	return plugins, capabilities, rows.Err()
}

func (r *PostgresRepository) loadRoutes(ctx context.Context) ([]navapp.Route, map[string]navapp.Route, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT route_name, path, plugin_id, COALESCE(component_key, ''), COALESCE(schema_id, ''), COALESCE(redirect_to, ''), permission, capability
FROM console_routes
WHERE enabled = true
ORDER BY sort_order, route_name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	routes := make([]navapp.Route, 0)
	routesByName := make(map[string]navapp.Route)
	for rows.Next() {
		var route navapp.Route
		if err := rows.Scan(&route.Name, &route.Path, &route.PluginID, &route.ComponentKey, &route.SchemaID, &route.Redirect, &route.Permission, &route.Capability); err != nil {
			return nil, nil, err
		}
		routes = append(routes, route)
		routesByName[route.Name] = route
	}
	return routes, routesByName, rows.Err()
}

type sqlNavigationItem struct {
	Key        string
	ParentKey  string
	Title      string
	Icon       string
	RouteName  string
	Permission string
	Capability string
	Level      int
	SortOrder  int
	Item       navapp.Item
	Children   []*sqlNavigationItem
}

func (r *PostgresRepository) loadMenus(ctx context.Context, routesByName map[string]navapp.Route, locale string) ([]navapp.Menu, error) {
	menus, err := r.loadMenusForLocale(ctx, routesByName, locale)
	if err != nil || len(menus) > 0 || locale == defaultLocale {
		return menus, err
	}
	return r.loadMenusForLocale(ctx, routesByName, defaultLocale)
}

func (r *PostgresRepository) loadMenusForLocale(ctx context.Context, routesByName map[string]navapp.Route, locale string) ([]navapp.Menu, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT item_key, COALESCE(parent_key, ''), title, icon, COALESCE(route_name, ''), permission, capability, level, sort_order
FROM console_navigation_items
WHERE enabled = true AND locale = $1
ORDER BY level, sort_order, item_key`, locale)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	itemsByID := make(map[string]*sqlNavigationItem)
	ordered := make([]*sqlNavigationItem, 0)
	for rows.Next() {
		var item sqlNavigationItem
		if err := rows.Scan(&item.Key, &item.ParentKey, &item.Title, &item.Icon, &item.RouteName, &item.Permission, &item.Capability, &item.Level, &item.SortOrder); err != nil {
			return nil, err
		}
		item.Item = navapp.Item{Title: item.Title, Icon: item.Icon, Permission: item.Permission, Capability: item.Capability}
		if route, ok := routesByName[item.RouteName]; ok {
			item.Item.Path = route.Path
			if item.Item.Permission == "" {
				item.Item.Permission = route.Permission
			}
			if item.Item.Capability == "" {
				item.Item.Capability = route.Capability
			}
		}
		itemsByID[item.Key] = &item
		ordered = append(ordered, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	menus := make([]navapp.Menu, 0)
	for _, item := range ordered {
		if item.ParentKey == "" {
			continue
		}
		if parent, ok := itemsByID[item.ParentKey]; ok {
			parent.Children = append(parent.Children, item)
		}
	}
	for _, item := range ordered {
		if item.ParentKey != "" {
			continue
		}
		children := sqlItemsToItems(item.Children)
		if len(children) == 0 && item.Item.Path != "" {
			children = []navapp.Item{item.Item}
		}
		menus = append(menus, navapp.Menu{Group: item.Title, Items: children})
	}
	return menus, nil
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return defaultLocale
	}
	return locale
}

func sqlItemsToItems(items []*sqlNavigationItem) []navapp.Item {
	result := make([]navapp.Item, 0, len(items))
	for _, item := range items {
		next := item.Item
		next.Children = sqlItemsToItems(item.Children)
		result = append(result, next)
	}
	return result
}

func (r *PostgresRepository) loadVersions(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version_key, version_value FROM console_navigation_versions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := map[string]string{"navigation": "db", "pluginCatalog": "db", "license": "mvp"}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		versions[key] = value
	}
	return versions, rows.Err()
}

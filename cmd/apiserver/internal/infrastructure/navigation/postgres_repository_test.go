package navigation

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresRepositorySnapshotLoadsOrderedLocalizedNavigation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	pluginRows := sqlmock.NewRows([]string{"plugin_id", "version", "display_name", "tier", "mode", "enabled"})
	pluginRows.AddRow("dashboard", "1.0.0", "Dashboard", "T0", "local", true)
	pluginRows.AddRow("container", "1.0.0", "Containers", "T1", "local", true)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plugin_id, version, display_name, tier, mode, enabled
FROM console_plugins
WHERE enabled = true
ORDER BY sort_order, plugin_id`)).WillReturnRows(pluginRows)

	routeRows := sqlmock.NewRows([]string{"route_name", "path", "plugin_id", "component_key", "schema_id", "redirect_to", "permission", "capability"})
	routeRows.AddRow("dashboard.overview", "/dashboard", "dashboard", "Dashboard", "", "", "dashboard:read", "")
	routeRows.AddRow("container.workloads", "/container/workloads", "container", "Workloads", "", "", "cluster:read", "kubernetes")
	routeRows.AddRow("container.storage.legacy", "/container/instances/storage", "container", "Storage", "", "/container/storage", "cluster:read", "")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT route_name, path, plugin_id, COALESCE(component_key, ''), COALESCE(schema_id, ''), COALESCE(redirect_to, ''), permission, capability
FROM console_routes
WHERE enabled = true
ORDER BY sort_order, route_name`)).WillReturnRows(routeRows)

	menuRows := sqlmock.NewRows([]string{"item_key", "parent_key", "title", "icon", "route_name", "permission", "capability", "level", "sort_order"})
	menuRows.AddRow("nav.dashboard", "", "Home", "dashboard", "dashboard.overview", "", "", 1, 10)
	menuRows.AddRow("nav.container", "", "Containers", "container", "", "", "", 1, 20)
	menuRows.AddRow("nav.container.workloads", "nav.container", "Workloads", "workload", "container.workloads", "", "", 2, 10)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT item_key, COALESCE(parent_key, ''), title, icon, COALESCE(route_name, ''), permission, capability, level, sort_order
FROM console_navigation_items
WHERE enabled = true AND locale = $1
ORDER BY level, sort_order, item_key`)).WithArgs("en-US").WillReturnRows(menuRows)

	versionRows := sqlmock.NewRows([]string{"version_key", "version_value"})
	versionRows.AddRow("navigation", "n1")
	versionRows.AddRow("pluginCatalog", "p1")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT version_key, version_value FROM console_navigation_versions`)).WillReturnRows(versionRows)

	snapshot, err := NewPostgresRepository(db).Snapshot(context.Background(), "tenant-a", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Plugins) != 2 || snapshot.Plugins[0].Name != "dashboard" || snapshot.Plugins[1].Name != "container" {
		t.Fatalf("unexpected plugins: %#v", snapshot.Plugins)
	}
	if len(snapshot.Menus) != 2 || snapshot.Menus[0].Group != "Home" || snapshot.Menus[1].Group != "Containers" {
		t.Fatalf("unexpected menus: %#v", snapshot.Menus)
	}
	workloads := snapshot.Menus[1].Items[0]
	if workloads.Path != "/container/workloads" || workloads.Permission != "cluster:read" || workloads.Capability != "kubernetes" {
		t.Fatalf("menu item did not inherit route metadata: %#v", workloads)
	}
	if len(snapshot.Routes) != 3 || snapshot.Routes[2].Redirect != "/container/storage" {
		t.Fatalf("compatibility redirect not loaded: %#v", snapshot.Routes)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresRepositoryFallsBackToDefaultLocale(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	pluginRows := sqlmock.NewRows([]string{"plugin_id", "version", "display_name", "tier", "mode", "enabled"})
	pluginRows.AddRow("dashboard", "1.0.0", "首页", "T0", "local", true)
	mock.ExpectQuery("SELECT plugin_id").WillReturnRows(pluginRows)

	routeRows := sqlmock.NewRows([]string{"route_name", "path", "plugin_id", "component_key", "schema_id", "redirect_to", "permission", "capability"})
	routeRows.AddRow("dashboard.overview", "/dashboard", "dashboard", "Dashboard", "", "", "", "")
	mock.ExpectQuery("SELECT route_name").WillReturnRows(routeRows)

	mock.ExpectQuery("SELECT item_key").WithArgs("fr-FR").WillReturnRows(sqlmock.NewRows([]string{"item_key", "parent_key", "title", "icon", "route_name", "permission", "capability", "level", "sort_order"}))
	fallbackRows := sqlmock.NewRows([]string{"item_key", "parent_key", "title", "icon", "route_name", "permission", "capability", "level", "sort_order"})
	fallbackRows.AddRow("nav.dashboard", "", "首页", "dashboard", "dashboard.overview", "", "", 1, 10)
	mock.ExpectQuery("SELECT item_key").WithArgs(defaultLocale).WillReturnRows(fallbackRows)
	mock.ExpectQuery("SELECT version_key").WillReturnRows(sqlmock.NewRows([]string{"version_key", "version_value"}))

	snapshot, err := NewPostgresRepository(db).Snapshot(context.Background(), "tenant-a", "fr-FR")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Menus) != 1 || snapshot.Menus[0].Group != "首页" {
		t.Fatalf("fallback menu not loaded: %#v", snapshot.Menus)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

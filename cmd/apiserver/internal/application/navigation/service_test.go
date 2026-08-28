package navigation

import (
	"context"
	"testing"

	"github.com/F31/hnb/pkg/iam"
)

func TestServiceFiltersMissingPermissionAndCapability(t *testing.T) {
	repo := stubRepo{snapshot: Snapshot{Versions: map[string]string{"permission": "p1", "pluginCatalog": "n1", "navigation": "n1"}, Capabilities: map[string]bool{"enabled": true}, Menus: []Menu{{Group: "g", Items: []Item{{Title: "Allowed", Path: "/allowed", Permission: "cluster:read"}, {Title: "Denied", Path: "/denied", Permission: "cluster:update"}, {Title: "No Cap", Path: "/no-cap", Permission: "cluster:read"}}}}, Routes: []Route{{Path: "/allowed", PluginID: "enabled", Permission: "cluster:read"}, {Path: "/denied", PluginID: "enabled", Permission: "cluster:update"}, {Path: "/no-cap", PluginID: "missing", Permission: "cluster:read"}}}}
	service := NewService(repo)
	resp, err := service.Build(Request{TenantID: "tenant-a", Trusted: iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", ScopedPermissions: []iam.ScopedPermission{{TenantID: "tenant-a", ResourceKind: "cluster", Action: iam.ActionRead}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Routes) != 1 || resp.Routes[0].Path != "/allowed" {
		t.Fatalf("unexpected routes: %#v", resp.Routes)
	}
	if len(resp.Menus) != 1 || len(resp.Menus[0].Items) != 1 || resp.Menus[0].Items[0].Path != "/allowed" {
		t.Fatalf("unexpected menus: %#v", resp.Menus)
	}
}

func TestNavigationDoesNotAliasClusterViewToRead(t *testing.T) {
	trusted := iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", ScopedPermissions: []iam.ScopedPermission{{TenantID: "tenant-a", ResourceKind: "cluster", Action: iam.ActionRead}}}
	if hasPermission(trusted, "cluster:view") {
		t.Fatal("legacy cluster:view alias was accepted")
	}
	if !hasPermission(trusted, iam.PermissionClusterRead) {
		t.Fatal("cluster:read was denied")
	}
}

func TestServiceETagChangesByTenantAndPermissionVersion(t *testing.T) {
	repo := stubRepo{snapshot: Snapshot{Versions: map[string]string{"permission": "p1", "pluginCatalog": "n1", "navigation": "n1"}, Capabilities: map[string]bool{}, Menus: []Menu{}, Routes: []Route{}}}
	service := NewService(repo)
	trusted := iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", PolicyVersion: "p1"}
	a, _ := service.Build(Request{TenantID: "tenant-a", Trusted: trusted})
	b, _ := service.Build(Request{TenantID: "tenant-b", Trusted: trusted})
	trusted.PolicyVersion = "p2"
	service.Invalidate()
	c, _ := service.Build(Request{TenantID: "tenant-a", Trusted: trusted})
	if a.ETag == b.ETag || a.ETag == c.ETag {
		t.Fatalf("etag did not vary: a=%s b=%s c=%s", a.ETag, b.ETag, c.ETag)
	}
}

func TestServiceUsesRepositoryLocalizedTitles(t *testing.T) {
	repo := stubRepo{snapshot: Snapshot{Versions: map[string]string{"permission": "p1", "pluginCatalog": "n1", "navigation": "n1"}, Capabilities: map[string]bool{}, Menus: []Menu{{Group: "g", Items: []Item{{Title: "平台总览", Path: "/dashboard"}}}}, Routes: []Route{{Path: "/dashboard", PluginID: "shell"}}}}
	resp, err := NewService(repo).Build(Request{TenantID: "tenant-a", Locale: "en-US", Trusted: iam.TrustedContext{SubjectID: "s", TenantID: "tenant-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Menus[0].Items[0].Title != "平台总览" {
		t.Fatalf("title = %q", resp.Menus[0].Items[0].Title)
	}
}

func TestServicePrunesEmptyParents(t *testing.T) {
	repo := stubRepo{snapshot: Snapshot{
		Versions:     map[string]string{"permission": "p1", "pluginCatalog": "n1", "navigation": "n1"},
		Capabilities: map[string]bool{"enabled": true},
		Menus: []Menu{{Group: "Container", Items: []Item{{
			Title:    "Instances",
			Children: []Item{{Title: "Denied", Path: "/denied", Permission: "cluster:update"}},
		}}}},
		Routes: []Route{{Path: "/denied", PluginID: "enabled", Permission: "cluster:update"}},
	}}
	resp, err := NewService(repo).Build(Request{TenantID: "tenant-a", Trusted: iam.TrustedContext{SubjectID: "s", TenantID: "tenant-a", ScopedPermissions: []iam.ScopedPermission{{TenantID: "tenant-a", ResourceKind: "cluster", Action: iam.ActionRead}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Menus) != 0 {
		t.Fatalf("expected empty parent to be pruned, got %#v", resp.Menus)
	}
}

func TestServiceRejectsParentItemPermission(t *testing.T) {
	repo := stubRepo{snapshot: Snapshot{
		Versions:     map[string]string{"permission": "p1", "pluginCatalog": "n1", "navigation": "n1"},
		Capabilities: map[string]bool{"enabled": true},
		Menus: []Menu{{Group: "System", Items: []Item{{
			Title:      "Admin",
			Permission: "system:admin",
			Children:   []Item{{Title: "Users", Path: "/users", Permission: "user:read"}},
		}}}},
		Routes: []Route{{Path: "/users", PluginID: "enabled", Permission: "user:read"}},
	}}
	resp, err := NewService(repo).Build(Request{TenantID: "tenant-a", Trusted: iam.TrustedContext{SubjectID: "s", TenantID: "tenant-a", ScopedPermissions: []iam.ScopedPermission{{TenantID: "tenant-a", ResourceKind: "user", Action: iam.ActionRead}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Menus) != 0 {
		t.Fatalf("expected parent permission to hide subtree, got %#v", resp.Menus)
	}
}

func TestServiceRejectsRedirectWhenCanonicalTargetIsNotAllowed(t *testing.T) {
	repo := stubRepo{snapshot: Snapshot{
		Versions:     map[string]string{"permission": "p1", "pluginCatalog": "n1", "navigation": "n1"},
		Capabilities: map[string]bool{"container": true},
		Routes: []Route{
			{Name: "storage", Path: "/container/storage", PluginID: "container", Permission: "storage:read"},
			{Name: "storage-legacy", Path: "/container/instances/storage", PluginID: "container", Redirect: "/container/storage"},
		},
	}}
	resp, err := NewService(repo).Build(Request{TenantID: "tenant-a", Trusted: iam.TrustedContext{SubjectID: "s", TenantID: "tenant-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Routes) != 0 {
		t.Fatalf("redirect bypassed canonical route authorization: %#v", resp.Routes)
	}
}

type stubRepo struct{ snapshot Snapshot }

func (s stubRepo) Snapshot(context.Context, string, string) (Snapshot, error) { return s.snapshot, nil }

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	navapp "github.com/F31/hnb/cmd/apiserver/internal/application/navigation"
	"github.com/F31/hnb/pkg/iam"
)

func TestNavigationMenusReturnsShellContract(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/navigation/menus?tenant=tenant-a", nil)
	req = req.WithContext(iam.WithTrustedContext(req.Context(), navigationTrustedContext()))
	recorder := httptest.NewRecorder()

	NewNavigationHandler(testNavigationRepo()).Menus(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if recorder.Header().Get("ETag") == "" {
		t.Fatal("missing ETag header")
	}
	var got navapp.Response
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.APIVersion != "navigation.hnb.io/v1" || got.Context.TenantID != "tenant-a" {
		t.Fatalf("unexpected navigation response: %+v", got)
	}
	if len(got.Menus) == 0 || len(got.Routes) == 0 {
		t.Fatalf("navigation response should contain menus and routes: %+v", got)
	}
}

func TestNavigationMenusSupportsETagNotModified(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/navigation/menus?tenant=tenant-a", nil)
	req = req.WithContext(iam.WithTrustedContext(req.Context(), navigationTrustedContext()))
	first := httptest.NewRecorder()
	NewNavigationHandler(testNavigationRepo()).Menus(first, req)

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/navigation/menus?tenant=tenant-a", nil)
	secondReq.Header.Set("If-None-Match", first.Header().Get("ETag"))
	secondReq = secondReq.WithContext(iam.WithTrustedContext(secondReq.Context(), navigationTrustedContext()))
	second := httptest.NewRecorder()
	NewNavigationHandler(testNavigationRepo()).Menus(second, secondReq)

	if second.Code != http.StatusNotModified {
		t.Fatalf("status = %d", second.Code)
	}
}

func navigationTrustedContext() iam.TrustedContext {
	return iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", ScopedPermissions: []iam.ScopedPermission{
		{TenantID: "tenant-a", ResourceKind: "dashboard", Action: iam.ActionRead},
		{TenantID: "tenant-a", ResourceKind: "cluster", Action: iam.ActionRead},
	}}
}

type navigationRepoStub struct{}

func testNavigationRepo() navapp.MetadataRepository { return navigationRepoStub{} }

func (navigationRepoStub) Snapshot(_ context.Context, _ string, _ string) (navapp.Snapshot, error) {
	return navapp.Snapshot{
		Versions:     map[string]string{"permission": "test", "pluginCatalog": "test", "navigation": "test"},
		Capabilities: map[string]bool{"dashboard": true},
		Menus:        []navapp.Menu{{Group: "首页", Items: []navapp.Item{{Title: "首页", Path: "/dashboard", Icon: "dashboard"}}}},
		Routes:       []navapp.Route{{Name: "dashboard.overview", Path: "/dashboard", PluginID: "dashboard", ComponentKey: "Dashboard"}},
	}, nil
}

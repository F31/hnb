package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlatformClusterHandlerForwardsListToPlatformAPI(t *testing.T) {
	var gotPath, gotAuth, gotTenant string
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("X-Tenant-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"cluster-a"}]`))
	}))
	defer platform.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	recorder := httptest.NewRecorder()

	NewPlatformClusterHandler(platform.URL).List(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if gotPath != "/v1/clusters" || gotAuth != "Bearer token" || gotTenant != "tenant-a" {
		t.Fatalf("unexpected forwarded request path=%q auth=%q tenant=%q", gotPath, gotAuth, gotTenant)
	}
}

func TestPlatformClusterHandlerMapsPlatformUnavailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	recorder := httptest.NewRecorder()

	handler := NewPlatformClusterHandler("http://127.0.0.1:1")
	handler.client = &http.Client{Timeout: 50 * time.Millisecond}
	handler.List(recorder, req)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", recorder.Code)
	}
}

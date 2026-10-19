package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/F31/hnb/cmd/apiserver/internal/middleware"
)

func TestServiceIdentityRoutesRegisterAndMatchRevoke(t *testing.T) {
	handlers := map[string]http.HandlerFunc{}
	for _, name := range []string{"create", "list", "get", "issue", "renew", "revoke", "revoke-credential"} {
		handlers["service-identities."+name] = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux := http.NewServeMux()
	registerServiceIdentityRoutes(mux, handlers)

	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/service-identities/4e69ee9e-5a18-46e3-b330-1349e0d74166/credentials/a69a8f5f-0872-4c8e-8df9-d9e6179050af/revoke", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("credential revoke route did not match: %d", recorder.Code)
	}
}

func TestPublicIdentityExchangePathsBypassTenantContext(t *testing.T) {
	for _, path := range []string{
		"/api/v1/service-credentials:renew",
		"/api/v1/agent-enrollments:exchange",
		"/api/v1/agent-sessions:renew",
	} {
		mw := middleware.NewTenantContext(nil, bypassPaths)
		recorder := httptest.NewRecorder()
		ctx := &middleware.Context{
			Request:  httptest.NewRequest(http.MethodPost, path, nil),
			Response: recorder,
			Params:   map[string]string{},
		}
		called := false
		mw.Handle(ctx, func() { called = true })
		if !called || recorder.Code == http.StatusUnauthorized {
			t.Fatalf("public identity path %s did not bypass tenant context", path)
		}
	}
}

package iam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type httpTestAuthenticator struct{}

func (httpTestAuthenticator) Authenticate(_ context.Context, token, correlationID, traceparent string) (TrustedContext, error) {
	return TrustedContext{SubjectID: token, TenantID: "tenant-a", MembershipID: "membership-a", CorrelationID: correlationID, Traceparent: traceparent}, nil
}

func TestTrustedHTTPMiddlewareSanitizesAndInjectsContext(t *testing.T) {
	handler := TrustedHTTPMiddleware(httpTestAuthenticator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted, ok := TrustedContextFrom(r.Context())
		if !ok || trusted.SubjectID != "signed-subject" || trusted.TenantID != "tenant-a" {
			t.Fatalf("trusted context = %#v, %v", trusted, ok)
		}
		for _, name := range []string{"Authorization", "X-Tenant-ID", "X-User-ID", "X-Actor-ID"} {
			if r.Header.Get(name) != "" {
				t.Fatalf("header %s survived", name)
			}
		}
		if r.Header.Get("X-Correlation-ID") == "" || r.Header.Get("traceparent") != "" {
			t.Fatalf("correlation/trace headers = %#v", r.Header)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/protected?token=query-token", nil)
	req.Header.Set("Authorization", "Bearer signed-subject")
	req.Header.Set("X-Tenant-ID", "spoofed")
	req.Header.Set("X-User-ID", "spoofed")
	req.Header.Set("X-Actor-ID", "spoofed")
	req.Header.Set("traceparent", "invalid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTrustedHTTPMiddlewareStrictBearerAndExactBypass(t *testing.T) {
	handler := TrustedHTTPMiddleware(httpTestAuthenticator{}, "/health")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, target := range []string{"/protected?token=query-token", "/health/private"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d", target, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("health status = %d", rec.Code)
	}
}

func TestHeaderSanitizationBlocksImpersonationHeaders(t *testing.T) {
	handler := TrustedHTTPMiddleware(httpTestAuthenticator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted, ok := TrustedContextFrom(r.Context())
		if !ok || trusted.SubjectID != "signed" {
			t.Fatalf("trusted context missing or wrong: %#v, %v", trusted, ok)
		}
		// Verify impersonation headers were stripped — handler should only see tenant from JWT, not HTTP headers
		for _, h := range []string{"X-Tenant-ID", "X-User-ID", "X-Subject-ID", "X-Actor-ID", "X-Workspace-ID", "X-Role", "X-Permission", "X-Membership-ID"} {
			if r.Header.Get(h) != "" {
				t.Fatalf("impersonation header %s was not sanitized", h)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/operations", nil)
	req.Header.Set("Authorization", "Bearer signed")
	req.Header.Set("X-Tenant-ID", "attacker-tenant")
	req.Header.Set("X-User-ID", "attacker-user")
	req.Header.Set("X-Subject-ID", "attacker-subject")
	req.Header.Set("X-Actor-ID", "attacker-actor")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestInvalidCorrelationIDIsRegenerated(t *testing.T) {
	handler := TrustedHTTPMiddleware(httpTestAuthenticator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get("X-Correlation-ID")
		if cid == "" {
			t.Fatal("correlation ID was dropped")
		}
		// Should be valid UUID format (newly generated)
		if len(cid) < 36 {
			t.Fatalf("correlation ID too short: %s", cid)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer signed")
	req.Header.Set("X-Correlation-ID", "not-a-valid-uuid!!!")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInvalidTraceparentIsStripped(t *testing.T) {
	handler := TrustedHTTPMiddleware(httpTestAuthenticator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("traceparent") != "" {
			t.Fatal("invalid traceparent should have been stripped")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer signed")
	req.Header.Set("traceparent", "invalid-format-here")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestBearerOnlyOneValueAndNoWhitespace(t *testing.T) {
	handler := TrustedHTTPMiddleware(httpTestAuthenticator{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	tests := []struct {
		name   string
		values []string
		wantOK bool
	}{
		{"no bearer", []string{}, false},
		{"two bearer values", []string{"Bearer tok1", "Bearer tok2"}, false},
		{"bearer with whitespace", []string{"Bearer to ken"}, false},
		{"bearer lowercase", []string{"bearer token"}, false},
		{"valid bearer", []string{"Bearer single-token-value"}, true},
		{"empty bearer", []string{"Bearer "}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, v := range tt.values {
				req.Header.Add("Authorization", v)
			}
			handler.ServeHTTP(rec, req)
			want := http.StatusUnauthorized
			if tt.wantOK {
				want = http.StatusNoContent
			}
			if rec.Code != want {
				t.Fatalf("status = %d, want %d", rec.Code, want)
			}
		})
	}
}

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/F31/hnb/pkg/iam"
)

type fakeAccessAuthenticator struct {
	trusted iam.TrustedContext
	err     error
	called  bool
}

func (f *fakeAccessAuthenticator) Authenticate(_ context.Context, token, correlationID, _ string) (iam.TrustedContext, error) {
	f.called = true
	if token != "valid-token" {
		return iam.TrustedContext{}, errors.New("invalid token")
	}
	trusted := f.trusted
	trusted.CorrelationID = correlationID
	return trusted, f.err
}

func TestAuthSanitizesHeadersAndInjectsTrustedContext(t *testing.T) {
	authenticator := &fakeAccessAuthenticator{trusted: iam.TrustedContext{SubjectID: "subject-a", SubjectType: "user", TenantID: "tenant-a", MembershipID: "membership-a"}}
	chain := NewChain(NewRequestID(), NewAuth(authenticator, nil))
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	request.Header.Set("X-Tenant-ID", "tenant-b")
	request.Header.Set("X-User-ID", "attacker")
	request.Header.Set("X-Roles", "admin")
	request.Header.Set("X-Permissions", "*")
	request.Header.Set("X-Approval", "approved")
	request.Header.Set("X-Actor-ID", "attacker")
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: request, Response: recorder}
	handled := false
	chain.Then(func(ctx *Context) {
		handled = true
		for _, name := range []string{"Authorization", "X-Tenant-ID", "X-User-ID", "X-Roles", "X-Permissions", "X-Approval", "X-Actor-ID"} {
			if ctx.Request.Header.Get(name) != "" {
				t.Fatalf("spoofed header %s survived sanitization", name)
			}
		}
		trusted, ok := iam.TrustedContextFrom(ctx.Request.Context())
		if !ok || trusted.SubjectID != "subject-a" || trusted.TenantID != "tenant-a" {
			t.Fatalf("trusted context = %#v, %v", trusted, ok)
		}
	})(ctx)
	if !handled || recorder.Code != http.StatusOK {
		t.Fatalf("handled = %v, status = %d", handled, recorder.Code)
	}
}

func TestAuthRejectsQueryTokenAndInactiveIdentity(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		url           string
		authError     error
	}{
		{name: "query token", url: "/protected?token=valid-token"},
		{name: "lowercase bearer", authorization: "bearer valid-token", url: "/protected"},
		{name: "disabled subject", authorization: "Bearer valid-token", url: "/protected", authError: iam.ErrMembershipMismatch},
		{name: "inactive membership", authorization: "Bearer valid-token", url: "/protected", authError: iam.ErrMembershipMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authenticator := &fakeAccessAuthenticator{trusted: iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a"}, err: test.authError}
			chain := NewChain(NewRequestID(), NewAuth(authenticator, nil))
			request := httptest.NewRequest(http.MethodGet, test.url, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			recorder := httptest.NewRecorder()
			ctx := &Context{Request: request, Response: recorder}
			handled := false
			chain.Then(func(*Context) { handled = true })(ctx)
			if handled || recorder.Code != http.StatusUnauthorized {
				t.Fatalf("handled = %v, status = %d", handled, recorder.Code)
			}
		})
	}
}

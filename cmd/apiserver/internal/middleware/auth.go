package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/F31/hnb/pkg/iam"
)

type AccessAuthenticator interface {
	Authenticate(context.Context, string, string, string) (iam.TrustedContext, error)
}

var traceparentPattern = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

type AuthMiddleware struct {
	authenticator AccessAuthenticator
	bypassPaths   []string
}

func NewAuth(authenticator AccessAuthenticator, bypassPaths []string) *AuthMiddleware {
	return &AuthMiddleware{authenticator: authenticator, bypassPaths: bypassPaths}
}

func (a *AuthMiddleware) Name() string { return "auth" }

func (a *AuthMiddleware) Handle(ctx *Context, next func()) {
	iam.SanitizeIdentityHeaders(ctx.Request.Header)
	if ctx.Request.Method == http.MethodOptions {
		writePreflight(ctx)
		return
	}
	for _, path := range a.bypassPaths {
		if matchBypass(ctx.Request.URL.Path, path) {
			next()
			return
		}
	}

	values := ctx.Request.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		ctx.Abort(http.StatusUnauthorized, []byte(`{"code":40100,"message":"missing authorization"}`))
		return
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		ctx.Abort(http.StatusUnauthorized, []byte(`{"code":40100,"message":"invalid token"}`))
		return
	}
	traceparent := strings.ToLower(ctx.Request.Header.Get("traceparent"))
	if traceparent != "" && !traceparentPattern.MatchString(traceparent) {
		ctx.Request.Header.Del("traceparent")
		traceparent = ""
	}
	trusted, err := a.authenticator.Authenticate(ctx.Request.Context(), token, ctx.RequestID, traceparent)
	if err != nil {
		ctx.Abort(http.StatusUnauthorized, []byte(`{"code":40100,"message":"invalid token"}`))
		return
	}

	ctx.TenantID = trusted.TenantID
	ctx.UserID = trusted.SubjectID
	ctx.Request.Header.Del("Authorization")
	reqCtx := iam.WithTrustedContext(ctx.Request.Context(), trusted)
	reqCtx = iam.WithRawAccessToken(reqCtx, token)
	ctx.Request = ctx.Request.WithContext(reqCtx)
	next()
}

func writePreflight(ctx *Context) {
	header := ctx.Response.Header()
	origin := ctx.Request.Header.Get("Origin")
	if origin != "" {
		header.Set("Access-Control-Allow-Origin", origin)
		header.Set("Vary", "Origin")
	}
	header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept,X-Tenant-ID,X-Space-ID,X-Environment-ID,X-Cluster-ID,X-Trace-Id,X-Correlation-ID")
	header.Set("Access-Control-Max-Age", "600")
	ctx.Abort(http.StatusNoContent, nil)
}

func matchBypass(path, bypass string) bool {
	if bypass == path {
		return true
	}
	if strings.HasSuffix(bypass, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(bypass, "*"))
	}
	return false
}

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func WriteJSONBytes(w http.ResponseWriter, code int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

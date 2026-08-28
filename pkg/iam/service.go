package iam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type TokenSource interface {
	Token(context.Context) (string, error)
}

type FileTokenSource struct {
	Path string
}

func (s FileTokenSource) Token(_ context.Context) (string, error) {
	if strings.TrimSpace(s.Path) == "" {
		return "", errors.New("token file path is required")
	}
	file, err := os.Open(s.Path)
	if err != nil {
		return "", fmt.Errorf("open token file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat token file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("token file must be regular and inaccessible to group and other users")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxAccessTokenSize+2))
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	value := strings.TrimSuffix(string(data), "\n")
	if len(value) == 0 || len(value) > MaxAccessTokenSize || strings.ContainsAny(value, " \t\r\n") {
		return "", errors.New("token file contains an invalid token")
	}
	return value, nil
}

type ServiceAuthenticator struct {
	verifier *TokenVerifier
}

func NewServiceAuthenticator(verifier *TokenVerifier) (*ServiceAuthenticator, error) {
	if verifier == nil {
		return nil, errors.New("token verifier is required")
	}
	return &ServiceAuthenticator{verifier: verifier}, nil
}

func (a *ServiceAuthenticator) Authenticate(ctx context.Context, token string, action AuthorizationAction, tenantID, resourceKind, resourceID string) (TrustedContext, error) {
	if !validAction(action) || action == "*" || !boundedClaim(tenantID, 128) || tenantID == "*" {
		return TrustedContext{}, errors.New("explicit action and tenant are required")
	}
	claims, err := a.verifier.Verify(ctx, token)
	if err != nil {
		return TrustedContext{}, err
	}
	if claims.SubjectType != "service" && claims.SubjectType != "workload" {
		return TrustedContext{}, errors.New("user tokens are not service credentials")
	}
	if len(claims.Audiences) != 1 || claims.Audiences[0] != a.verifier.config.Audience {
		return TrustedContext{}, errors.New("service token must have one exact target audience")
	}
	if !containsAction(claims.AllowedActions, action) {
		return TrustedContext{}, errors.New("service token does not allow requested action")
	}
	trusted, err := a.verifier.Authenticate(ctx, token, "", "")
	if err != nil {
		return TrustedContext{}, err
	}
	decision := NewEvaluator().Evaluate(trusted, AuthorizationRequest{
		SubjectID: claims.SubjectID, TenantID: tenantID, ResourceKind: resourceKind,
		ResourceID: resourceID, Action: action,
	})
	if !decision.Allowed {
		return TrustedContext{}, errors.New("service token does not authorize requested scope")
	}
	return trusted, nil
}

func containsAction(actions []AuthorizationAction, expected AuthorizationAction) bool {
	for _, action := range actions {
		if action == expected {
			return true
		}
	}
	return false
}

type ServiceRequestScope struct {
	TenantID     string
	ResourceKind string
	ResourceID   string
}

type ServiceScopeResolver func(*http.Request) (ServiceRequestScope, error)

func RequireServiceIdentity(authenticator *ServiceAuthenticator, action AuthorizationAction, resolve ServiceScopeResolver, bypassPaths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			SanitizeIdentityHeaders(r.Header)
			if contains(bypassPaths, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			values := r.Header.Values("Authorization")
			if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(values[0], "Bearer ")
			if token == "" || strings.ContainsAny(token, " \t\r\n") || resolve == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			scope, err := resolve(r)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			trusted, err := authenticator.Authenticate(r.Context(), token, action, scope.TenantID, scope.ResourceKind, scope.ResourceID)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r.Header.Del("Authorization")
			next.ServeHTTP(w, r.WithContext(WithTrustedContext(r.Context(), trusted)))
		})
	}
}

func RuntimeExecutionScope(maxBytes int64) ServiceScopeResolver {
	return func(r *http.Request) (ServiceRequestScope, error) {
		if maxBytes <= 0 {
			return ServiceRequestScope{}, errors.New("positive request limit is required")
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
		if err != nil || int64(len(data)) > maxBytes {
			return ServiceRequestScope{}, errors.New("invalid execution request size")
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		var envelope struct {
			Execution struct {
				TenantID   string `json:"tenant_id"`
				ProviderID string `json:"provider_id"`
			} `json:"execution"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.Execution.TenantID == "" || envelope.Execution.ProviderID == "" {
			return ServiceRequestScope{}, errors.New("execution scope is required")
		}
		return ServiceRequestScope{TenantID: envelope.Execution.TenantID, ResourceKind: "runtimeProvider", ResourceID: envelope.Execution.ProviderID}, nil
	}
}

func ParseVerificationKeyPaths(value string) (map[string]string, error) {
	paths := make(map[string]string)
	for _, entry := range strings.Split(value, ",") {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, errors.New("verification keys must contain kid=path entries")
		}
		kid := strings.TrimSpace(parts[0])
		if _, duplicate := paths[kid]; duplicate {
			return nil, errors.New("verification key IDs must be unique")
		}
		paths[kid] = strings.TrimSpace(parts[1])
	}
	if len(paths) == 0 {
		return nil, errors.New("verification keys are required")
	}
	return paths, nil
}

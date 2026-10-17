package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const MaxServiceCredentialResponseBytes = 64 * 1024

// HTTPCredentialIssuerConfig configures the worker-side adapter for the
// apiserver renewal endpoint. Bootstrap is only the first credential; after a
// successful renewal the adapter keeps the newest credential in memory. When
// renewal is configured, a static-file fallback is never used on failure.
type HTTPCredentialIssuerConfig struct {
	BaseURL          string
	Bootstrap        TokenSource
	Client           *http.Client
	MaxResponseBytes int64
	Now              func() time.Time
}

type HTTPCredentialIssuer struct {
	baseURL          string
	bootstrap        TokenSource
	client           *http.Client
	maxResponseBytes int64
	now              func() time.Time
	mu               sync.Mutex
	current          string
}

func NewHTTPCredentialIssuer(config HTTPCredentialIssuerConfig) (*HTTPCredentialIssuer, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("service credential issuer URL must be an absolute HTTP(S) URL without credentials")
	}
	if config.Bootstrap == nil {
		return nil, errors.New("service credential bootstrap token source is required")
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: ServiceCredentialRPCTimeout + time.Second}
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = MaxServiceCredentialResponseBytes
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &HTTPCredentialIssuer{
		baseURL: strings.TrimRight(parsed.String(), "/"), bootstrap: config.Bootstrap,
		client: config.Client, maxResponseBytes: config.MaxResponseBytes, now: config.Now,
	}, nil
}

func (i *HTTPCredentialIssuer) IssueServiceCredential(ctx context.Context, scope ServiceCredentialScope) (string, error) {
	if err := validateServiceCredentialScope(scope); err != nil {
		return "", ErrServiceCredentialRejected
	}
	current, err := i.currentToken(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: bootstrap credential unavailable", ErrServiceCredentialUnavailable)
	}

	rpcCtx, cancel := context.WithTimeout(ctx, ServiceCredentialRPCTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rpcCtx, http.MethodPost, i.baseURL+"/api/v1/service-credentials:renew", nil)
	if err != nil {
		return "", fmt.Errorf("%w: create renewal request", ErrServiceCredentialUnavailable)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+current)

	resp, err := i.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: renewal request failed", ErrServiceCredentialUnavailable)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, i.maxResponseBytes+1))
	if err != nil || int64(len(body)) > i.maxResponseBytes {
		return "", fmt.Errorf("%w: invalid renewal response size", ErrServiceCredentialUnavailable)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		i.forgetOnPermanentRejection(resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return "", ErrServiceCredentialRejected
		case http.StatusConflict:
			return "", ErrServiceIdentityRevoked
		default:
			return "", fmt.Errorf("%w: renewal endpoint returned HTTP %d", ErrServiceCredentialUnavailable, resp.StatusCode)
		}
	}

	var payload struct {
		Credential string `json:"credential"`
		ExpiresAt  string `json:"expiresAt"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("%w: invalid renewal payload", ErrServiceCredentialUnavailable)
	}
	if len(payload.Credential) == 0 || len(payload.Credential) > MaxAccessTokenSize || payload.ExpiresAt == "" {
		return "", fmt.Errorf("%w: incomplete renewal payload", ErrServiceCredentialUnavailable)
	}
	expiry, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil || !expiry.After(i.now().UTC()) {
		return "", fmt.Errorf("%w: renewal credential is already expired", ErrServiceCredentialUnavailable)
	}
	if err := validateRenewedCredential(payload.Credential, scope); err != nil {
		return "", ErrServiceCredentialRejected
	}

	i.mu.Lock()
	i.current = payload.Credential
	i.mu.Unlock()
	return payload.Credential, nil
}

func (i *HTTPCredentialIssuer) currentToken(ctx context.Context) (string, error) {
	i.mu.Lock()
	current := i.current
	i.mu.Unlock()
	if current != "" {
		return current, nil
	}
	value, err := i.bootstrap.Token(ctx)
	if err != nil || value == "" {
		return "", errors.New("bootstrap credential unavailable")
	}
	i.mu.Lock()
	if i.current == "" {
		i.current = value
	}
	current = i.current
	i.mu.Unlock()
	return current, nil
}

func (i *HTTPCredentialIssuer) forgetOnPermanentRejection(status int) {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
		i.mu.Lock()
		i.current = ""
		i.mu.Unlock()
	}
}

func validateServiceCredentialScope(scope ServiceCredentialScope) error {
	if _, err := uuid.Parse(scope.SubjectID); err != nil {
		return errors.New("invalid subject")
	}
	if !boundedClaim(scope.Audience, 256) || scope.Audience == "*" {
		return errors.New("invalid audience")
	}
	return ValidatePermissionSnapshot("serviceScope", []ScopedPermission{scope.Permission}, scope.Permission.TenantID)
}

func validateRenewedCredential(token string, scope ServiceCredentialScope) error {
	parsed, err := parseUnverifiedAccessToken(token)
	if err != nil {
		return err
	}
	if parsed.SubjectID != scope.SubjectID || parsed.TenantID != scope.Permission.TenantID ||
		parsed.Audience[0] != scope.Audience || len(parsed.ScopedPermissions) != 1 ||
		parsed.ScopedPermissions[0] != scope.Permission || len(parsed.AllowedActions) != 1 ||
		parsed.AllowedActions[0] != scope.Permission.Action {
		return errors.New("renewed credential does not match the requested scope")
	}
	return nil
}

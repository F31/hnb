package provider

import (
	"context"
	"fmt"
)

type ErrorCode string

const (
	ErrorInvalidRequest    ErrorCode = "INVALID_REQUEST"
	ErrorScopeDenied       ErrorCode = "SCOPE_DENIED"
	ErrorUnsupportedAction ErrorCode = "UNSUPPORTED_ACTION"
	ErrorResourceConflict  ErrorCode = "RESOURCE_CONFLICT"
	ErrorFenced            ErrorCode = "FENCED"
	ErrorTargetUnavailable ErrorCode = "TARGET_UNAVAILABLE"
	ErrorCancelled         ErrorCode = "CANCELLED"
)

type StatusError struct {
	HTTPStatus int
	Code       ErrorCode
	Retryable  bool
	Message    string
}

func (e *StatusError) Error() string { return e.Message }

func fail(status int, code ErrorCode, retryable bool, format string, args ...any) error {
	return &StatusError{HTTPStatus: status, Code: code, Retryable: retryable, Message: fmt.Sprintf(format, args...)}
}

func invalid(format string, args ...any) error {
	return fail(400, ErrorInvalidRequest, false, format, args...)
}

func targetError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return fail(408, ErrorCancelled, true, "%s: %v", operation, err)
	}
	return fail(503, ErrorTargetUnavailable, true, "%s: %v", operation, err)
}

type SecretReference struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

type ExecutionContext struct {
	StepID                  string
	OperationID             string
	TenantID                string
	ProjectID               string
	EnvironmentID           string
	StepType                string
	Inputs                  map[string]any
	ProviderID              string
	ProviderVersion         string
	ProviderDigest          string
	ProviderProtocolVersion string
	Checkpoint              string
	IdempotencyKey          string
	ExecutionAttemptID      string
	FencingGeneration       int64
}

type LifecycleInput struct {
	SchemaVersion       string
	TargetID            string
	TargetKind          string
	Action              string
	DisplayName         string
	DesiredVersion      string
	CloudCoreEndpoint   string
	CredentialSecretRef *SecretReference
	IdempotencyKey      string
	FencingGeneration   int64
	ObservationVersion  int64
}

type Result struct {
	Outputs    map[string]string
	Checkpoint string
}

type SecretResolver interface {
	Resolve(ctx context.Context, tenantID, providerID, purpose string, ref SecretReference) ([]byte, error)
}

type MetadataOnlySecretResolver struct{}

func (MetadataOnlySecretResolver) Resolve(_ context.Context, _ string, _ string, _ string, ref SecretReference) ([]byte, error) {
	if ref.Provider == "" || ref.Scope == "" || ref.Name == "" || ref.Version == "" {
		return nil, invalid("credentialSecretRef must include provider, scope, name, and version")
	}
	return []byte("dummy-kubeconfig-content"), nil
}

type ObserverRegistry interface {
	Register(ctx context.Context, tenantID, targetID, targetKind, observerKind string) error
	Unregister(ctx context.Context, tenantID, targetID string) error
}

type LifecycleManager interface {
	Apply(ctx context.Context, execution ExecutionContext, input LifecycleInput) (Result, error)
}

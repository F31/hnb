package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

const (
	contractVersion             = "2.0.0"
	maxResponseSize             = 1 << 20
	lifecycleProviderKubernetes = "runtime-target.lifecycle.kubernetes"
	lifecycleProviderEdge       = "runtime-target.lifecycle.edge"
	storageDriverStepPrefix     = "storage.driver."
)

var lifecycleProviderIDs = map[string]bool{
	lifecycleProviderKubernetes: true,
	lifecycleProviderEdge:       true,
}

// IsLifecycleProvider reports whether the provider ID belongs to a namespaced
// lifecycle provider family that must be pinned to a server-resolved version
// and digest.
func IsLifecycleProvider(providerID string) bool {
	return lifecycleProviderIDs[providerID]
}

func requiresPinnedProvider(execution engine.ExecutionContext) bool {
	return IsLifecycleProvider(execution.ProviderID) || strings.HasPrefix(execution.StepType, storageDriverStepPrefix)
}

type ErrorCode string

const (
	ErrorInvalidRequest    ErrorCode = "INVALID_REQUEST"
	ErrorScopeDenied       ErrorCode = "SCOPE_DENIED"
	ErrorUnsupportedAction ErrorCode = "UNSUPPORTED_ACTION"
	ErrorResourceConflict  ErrorCode = "RESOURCE_CONFLICT"
	ErrorFenced            ErrorCode = "FENCED"
	ErrorTargetUnavailable ErrorCode = "TARGET_UNAVAILABLE"
	ErrorCancelled         ErrorCode = "CANCELLED"
	ErrorInternal          ErrorCode = "INTERNAL"
)

type ProviderError struct {
	Code       ErrorCode
	Message    string
	Retryable  bool
	HTTPStatus int
}

func (e *ProviderError) Error() string {
	message := providerError(e.Message)
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("runtime provider error %s (HTTP %d): %s", e.Code, e.HTTPStatus, message)
	}
	return fmt.Sprintf("runtime provider error %s: %s", e.Code, message)
}

type HTTPRunner struct {
	providers map[string]ProviderConfig
	client    *http.Client
}

type ProviderConfig struct {
	Endpoint         string
	Audience         string
	TokenSource      iam.TokenSource
	ProtocolVersion  string
	ProviderVersion  string
	ProviderDigest   string
	RequiredProvider string
}

type executeRequest struct {
	SchemaVersion string           `json:"schemaVersion"`
	Execution     executionRequest `json:"execution"`
}

type executionRequest struct {
	StepID                  string         `json:"step_id"`
	OperationID             string         `json:"operation_id"`
	TenantID                string         `json:"tenant_id"`
	ProjectID               string         `json:"project_id"`
	EnvironmentID           string         `json:"environment_id"`
	StepType                string         `json:"step_type"`
	Inputs                  map[string]any `json:"inputs"`
	ProviderID              string         `json:"provider_id"`
	ProviderVersion         string         `json:"provider_version"`
	ProviderDigest          string         `json:"provider_digest"`
	ProviderProtocolVersion string         `json:"provider_protocol_version"`
	Checkpoint              string         `json:"checkpoint,omitempty"`
	IdempotencyKey          string         `json:"idempotency_key"`
	ExecutionAttemptID      string         `json:"execution_attempt_id"`
	FencingGeneration       string         `json:"fencing_generation"`
}

type executeResponse struct {
	SchemaVersion      string            `json:"schemaVersion"`
	ExecutionAttemptID string            `json:"executionAttemptId"`
	IdempotencyKey     string            `json:"idempotencyKey"`
	FencingGeneration  string            `json:"fencingGeneration"`
	ProviderVersion    string            `json:"providerVersion"`
	ProviderDigest     string            `json:"providerDigest"`
	Status             string            `json:"status"`
	Outputs            map[string]string `json:"outputs,omitempty"`
	Checkpoint         string            `json:"checkpoint,omitempty"`
	ErrorCode          ErrorCode         `json:"errorCode,omitempty"`
	Error              string            `json:"error,omitempty"`
	Retryable          *bool             `json:"retryable,omitempty"`
}

func NewHTTPRunner(providers map[string]ProviderConfig, client *http.Client) (*HTTPRunner, error) {
	validated := make(map[string]ProviderConfig, len(providers))
	for providerID, provider := range providers {
		if strings.TrimSpace(providerID) == "" {
			return nil, fmt.Errorf("provider ID cannot be empty")
		}
		parsed, err := url.Parse(provider.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("provider %q endpoint: %w", providerID, err)
		}
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("provider %q endpoint must be an absolute HTTP URL", providerID)
		}
		if parsed.User != nil {
			return nil, fmt.Errorf("provider %q endpoint cannot contain credentials", providerID)
		}
		if parsed.Fragment != "" {
			return nil, fmt.Errorf("provider %q endpoint cannot contain a fragment", providerID)
		}
		if provider.Audience == "" || provider.Audience == "*" || provider.TokenSource == nil {
			return nil, fmt.Errorf("provider %q requires a non-wildcard audience and token source", providerID)
		}
		if provider.ProtocolVersion == "" {
			provider.ProtocolVersion = contractVersion
		}
		if provider.ProtocolVersion != contractVersion {
			return nil, fmt.Errorf("provider %q protocolVersion %q is not supported", providerID, provider.ProtocolVersion)
		}
		if IsLifecycleProvider(providerID) || strings.HasPrefix(providerID, "storage.") {
			if provider.RequiredProvider == "" {
				return nil, fmt.Errorf("lifecycle provider %q requires requiredProvider to be pinned", providerID)
			}
			if provider.RequiredProvider != providerID {
				return nil, fmt.Errorf("lifecycle provider %q requiredProvider %q does not match provider ID", providerID, provider.RequiredProvider)
			}
			if provider.ProviderVersion == "" || provider.ProviderDigest == "" {
				return nil, fmt.Errorf("lifecycle provider %q requires providerVersion and providerDigest", providerID)
			}
		}
		provider.Endpoint = parsed.String()
		validated[providerID] = provider
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPRunner{providers: validated, client: client}, nil
}

func (r *HTTPRunner) Execute(
	ctx context.Context,
	execution engine.ExecutionContext,
) (map[string]string, string, error) {
	provider, ok := r.providers[execution.ProviderID]
	if !ok {
		return nil, "", &ProviderError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("runtime provider %q is not configured", execution.ProviderID)}
	}
	if _, err := uuid.Parse(execution.ExecutionAttemptID); err != nil {
		return nil, "", &ProviderError{Code: ErrorInvalidRequest, Message: "execution attempt ID must be a UUID"}
	}
	if execution.FencingGeneration <= 0 {
		return nil, "", &ProviderError{Code: ErrorInvalidRequest, Message: "fencing generation must be positive"}
	}
	if requiresPinnedProvider(execution) {
		if execution.ProviderVersion == "" || execution.ProviderDigest == "" {
			return nil, "", &ProviderError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("lifecycle provider %q requires pinned providerVersion and providerDigest", execution.ProviderID)}
		}
		if execution.ProviderVersion != provider.ProviderVersion {
			return nil, "", &ProviderError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("lifecycle provider %q pinned version %q does not match configured version %q", execution.ProviderID, execution.ProviderVersion, provider.ProviderVersion)}
		}
		if execution.ProviderDigest != provider.ProviderDigest {
			return nil, "", &ProviderError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("lifecycle provider %q pinned digest %q does not match configured digest %q", execution.ProviderID, execution.ProviderDigest, provider.ProviderDigest)}
		}
	}
	fencingGeneration := strconv.FormatInt(execution.FencingGeneration, 10)

	body, err := json.Marshal(executeRequest{
		SchemaVersion: contractVersion,
		Execution: executionRequest{
			StepID:                  execution.StepID,
			OperationID:             execution.OperationID,
			TenantID:                execution.TenantID,
			ProjectID:               execution.ProjectID,
			EnvironmentID:           execution.EnvironmentID,
			StepType:                execution.StepType,
			Inputs:                  execution.Inputs,
			ProviderID:              execution.ProviderID,
			ProviderVersion:         execution.ProviderVersion,
			ProviderDigest:          execution.ProviderDigest,
			ProviderProtocolVersion: provider.ProtocolVersion,
			Checkpoint:              execution.Checkpoint,
			IdempotencyKey:          execution.IdempotencyKey,
			ExecutionAttemptID:      execution.ExecutionAttemptID,
			FencingGeneration:       fencingGeneration,
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("encode runtime provider request: %w", err)
	}
	token, err := provider.TokenSource.Token(ctx)
	if err != nil {
		return nil, "", &ProviderError{Code: ErrorTargetUnavailable, Message: fmt.Sprintf("load credential for runtime provider %q: %v", execution.ProviderID, err), Retryable: true}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("create runtime provider request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", &ProviderError{Code: ErrorTargetUnavailable, Message: fmt.Sprintf("call runtime provider %q: %v", execution.ProviderID, err), Retryable: true}
	}
	defer resp.Body.Close()

	result, err := decodeResponse(resp.Body)
	if err != nil {
		return nil, "", protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned invalid response: %v", execution.ProviderID, err))
	}
	if result.SchemaVersion != contractVersion {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned unsupported schema version %q", execution.ProviderID, result.SchemaVersion))
	}
	if result.ExecutionAttemptID != execution.ExecutionAttemptID {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned mismatched execution attempt ID", execution.ProviderID))
	}
	if result.IdempotencyKey != "" && result.IdempotencyKey != execution.IdempotencyKey {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned mismatched idempotency key", execution.ProviderID))
	}
	if requiresPinnedProvider(execution) {
		if result.ProviderVersion != execution.ProviderVersion {
			return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned mismatched providerVersion", execution.ProviderID))
		}
		if result.ProviderDigest != execution.ProviderDigest {
			return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned mismatched providerDigest", execution.ProviderID))
		}
	}
	generation, err := parseGeneration(result.FencingGeneration)
	if err != nil {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned invalid fencing generation: %v", execution.ProviderID, err))
	}
	if generation != execution.FencingGeneration {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned mismatched fencing generation", execution.ProviderID))
	}
	if result.Status != "succeeded" && result.Status != "failed" {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned unknown status %q", execution.ProviderID, result.Status))
	}
	if result.Status == "succeeded" {
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned success with HTTP %d", execution.ProviderID, resp.StatusCode))
		}
		if result.Error != "" || result.ErrorCode != "" || result.Retryable != nil {
			return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned success with error fields", execution.ProviderID))
		}
		return result.Outputs, result.Checkpoint, nil
	}
	if !validErrorCode(result.ErrorCode) || result.Error == "" || result.Retryable == nil {
		return result.Outputs, result.Checkpoint, protocolError(resp.StatusCode, fmt.Sprintf("runtime provider %q returned incomplete failure fields", execution.ProviderID))
	}
	return result.Outputs, result.Checkpoint, &ProviderError{
		Code:       result.ErrorCode,
		Message:    result.Error,
		Retryable:  classifyRetryable(resp.StatusCode, result.ErrorCode),
		HTTPStatus: resp.StatusCode,
	}
}

func parseGeneration(value string) (int64, error) {
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != value {
		return 0, fmt.Errorf("must be a canonical positive decimal string")
	}
	return generation, nil
}

func validErrorCode(code ErrorCode) bool {
	switch code {
	case ErrorInvalidRequest, ErrorScopeDenied, ErrorUnsupportedAction, ErrorResourceConflict,
		ErrorFenced, ErrorTargetUnavailable, ErrorCancelled, ErrorInternal:
		return true
	default:
		return false
	}
}

func classifyRetryable(status int, code ErrorCode) bool {
	if status >= http.StatusInternalServerError {
		return true
	}
	if status == http.StatusBadRequest || status == http.StatusForbidden || status == http.StatusConflict {
		return false
	}
	return code == ErrorTargetUnavailable || code == ErrorCancelled || code == ErrorInternal
}

func protocolError(status int, message string) error {
	code := ErrorInvalidRequest
	retryable := false
	if status >= http.StatusInternalServerError {
		code = ErrorInternal
		retryable = true
	}
	return &ProviderError{Code: code, Message: message, Retryable: retryable, HTTPStatus: status}
}

func decodeResponse(body io.Reader) (executeResponse, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseSize+1))
	if err != nil {
		return executeResponse{}, err
	}
	if len(data) > maxResponseSize {
		return executeResponse{}, fmt.Errorf("response exceeds %d bytes", maxResponseSize)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result executeResponse
	if err := decoder.Decode(&result); err != nil {
		return executeResponse{}, err
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return executeResponse{}, err
	}
	return result, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected trailing JSON")
	}
	return nil
}

func providerError(message string) string {
	if message == "" {
		return "provider did not supply an error"
	}
	return message
}

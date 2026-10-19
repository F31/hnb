package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Intent kinds the server currently accepts for cluster/node operations. The
// client refuses anything else rather than inventing a server-side contract.
const (
	IntentCreateKubernetesTarget = "CreateKubernetesTarget"
	IntentImportRuntimeTarget    = "ImportRuntimeTarget"
	IntentAddNodesRuntimeTarget  = "AddNodesRuntimeTarget"
)

// Stable client-side failure classes. They exist so the CLI can map an outcome
// to an exit code without re-parsing transport details.
var (
	// ErrPlatformUnavailable means the platform could not be reached or did not
	// answer (5xx/timeout). There is deliberately no local fallback for it.
	ErrPlatformUnavailable = errors.New("PLATFORM_UNAVAILABLE")
	// ErrRequestRejected means the server refused the request (validation,
	// authorization or cross-tenant scope). The request is never retried.
	ErrRequestRejected = errors.New("REQUEST_REJECTED")
	// ErrNotAuthenticated means no usable credential was supplied.
	ErrNotAuthenticated = errors.New("NOT_AUTHENTICATED")
)

// IntentSecretRef is a reference to operator-managed secret material. Only
// references cross the CLI boundary; raw secret material is never accepted.
type IntentSecretRef struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

type IntentSpec struct {
	TargetID            string            `json:"targetId,omitempty"`
	ExpectedVersion     int64             `json:"expectedVersion,omitempty"`
	TargetKind          string            `json:"targetKind"`
	DisplayName         string            `json:"displayName,omitempty"`
	CredentialSecretRef *IntentSecretRef  `json:"credentialSecretRef,omitempty"`
	OfferingID          string            `json:"offeringId,omitempty"`
	OfferingVersion     int64             `json:"offeringVersion,omitempty"`
	BindingID           string            `json:"bindingId,omitempty"`
	Parameters          map[string]any    `json:"parameters,omitempty"`
	SecretReferences    []IntentSecretRef `json:"secretReferences,omitempty"`
	SSHSecretRef        *IntentSecretRef  `json:"sshSecretRef,omitempty"`
	StorageClassName    string            `json:"storageClassName,omitempty"`
}

type intentEnvelope struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   intentMetadata `json:"metadata"`
	Spec       IntentSpec     `json:"spec"`
}

type intentMetadata struct {
	IdempotencyKey string `json:"idempotencyKey"`
	CorrelationID  string `json:"correlationId"`
}

type intentResponse struct {
	APIVersion      string `json:"apiVersion"`
	IntentID        string `json:"intentId"`
	Status          string `json:"status"`
	ExecutionPlanID string `json:"executionPlanId,omitempty"`
	OperationID     string `json:"operationId,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
	CorrelationID   string `json:"correlationId,omitempty"`
	Replayed        bool   `json:"replayed"`
	Error           *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// IntentResult is the server-authoritative outcome of a submitted intent.
type IntentResult struct {
	IntentID        string `json:"intentId"`
	Status          string `json:"status"`
	ExecutionPlanID string `json:"executionPlanId,omitempty"`
	OperationID     string `json:"operationId,omitempty"`
	Replayed        bool   `json:"replayed"`
	CreatedAt       string `json:"createdAt,omitempty"`
}

// PlatformClient submits typed intents to the platform. It never executes a
// provider, never writes the platform database and never falls back to SSH.
type PlatformClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
	// TenantScope is the scope the credential is bound to. Every secret
	// reference must live inside it; a cross-tenant reference is refused locally
	// rather than sent and rejected later.
	TenantScope string
}

// SubmitIntent sends one typed intent. Transport/availability problems return
// ErrPlatformUnavailable; server refusals return ErrRequestRejected.
func (p *PlatformClient) SubmitIntent(ctx context.Context, kind, idempotencyKey, correlationID string, spec IntentSpec) (IntentResult, error) {
	var result IntentResult
	if p.BaseURL == "" {
		return result, fmt.Errorf("platform base URL is required")
	}
	if strings.TrimSpace(p.Token) == "" {
		return result, ErrNotAuthenticated
	}
	if kind != IntentCreateKubernetesTarget && kind != IntentImportRuntimeTarget && kind != IntentAddNodesRuntimeTarget {
		return result, fmt.Errorf("%w: intent kind %q has no server contract", ErrRequestRejected, kind)
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return result, fmt.Errorf("%w: metadata.idempotencyKey is required", ErrRequestRejected)
	}
	if err := p.validateSecretRefs(spec.SecretReferences); err != nil {
		return result, err
	}
	if spec.SSHSecretRef != nil {
		if err := p.validateOneSecretRef(*spec.SSHSecretRef); err != nil {
			return result, err
		}
	}
	envelope := intentEnvelope{APIVersion: "hnb.io/v1", Kind: kind,
		Metadata: intentMetadata{IdempotencyKey: idempotencyKey, CorrelationID: correlationID}, Spec: spec}
	body, err := json.Marshal(envelope)
	if err != nil {
		return result, fmt.Errorf("%w: cannot encode intent", ErrRequestRejected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/api/v1/runtime-intents", bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	if correlationID != "" {
		req.Header.Set("X-Correlation-ID", correlationID)
	}

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// Unreachable/timeout. There is no local fallback by contract.
		return result, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		var decoded intentResponse
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return result, fmt.Errorf("%w: platform returned an unreadable intent record", ErrPlatformUnavailable)
		}
		return IntentResult{IntentID: decoded.IntentID, Status: decoded.Status, ExecutionPlanID: decoded.ExecutionPlanID, OperationID: decoded.OperationID, Replayed: decoded.Replayed, CreatedAt: decoded.CreatedAt}, nil
	case resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout || resp.StatusCode >= 500:
		// Availability problem: report and keep any known operation reference.
		// Never trigger a local install.
		return result, fmt.Errorf("%w: platform returned HTTP %d", ErrPlatformUnavailable, resp.StatusCode)
	default:
		var problem struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		code := ""
		if json.Unmarshal(payload, &problem) == nil {
			code = problem.Code
			if code == "" {
				code = problem.Error.Code
			}
			if code == "" {
				code = problem.Detail
			}
		}
		if code == "" {
			code = fmt.Sprintf("HTTP_%d", resp.StatusCode)
		}
		return result, fmt.Errorf("%w: %s", ErrRequestRejected, code)
	}
}

// validateSecretRefs refuses raw secret material and cross-tenant references
// before anything leaves the installer.
func (p *PlatformClient) validateSecretRefs(refs []IntentSecretRef) error {
	for _, ref := range refs {
		if err := p.validateOneSecretRef(ref); err != nil {
			return err
		}
	}
	return nil
}

func (p *PlatformClient) validateOneSecretRef(ref IntentSecretRef) error {
	if ref.Provider == "" || ref.Name == "" {
		return fmt.Errorf("%w: incomplete secret reference", ErrRequestRejected)
	}
	if p.TenantScope != "" && ref.Scope != p.TenantScope {
		return fmt.Errorf("%w: secret reference scope is outside the authenticated tenant scope", ErrRequestRejected)
	}
	return nil
}

// OperationRef identifies a server operation for later status queries.
type OperationRef struct {
	OperationID string `json:"operationId"`
	IntentID    string `json:"intentId,omitempty"`
	Status      string `json:"status,omitempty"`
}

// RuntimeTargetSummary is one server-authoritative cluster/target row. Only
// fields the console exposes are surfaced; the CLI never infers a target.
type RuntimeTargetSummary struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	DisplayName        string            `json:"displayName,omitempty"`
	TargetType         string            `json:"targetType"`
	Distribution       string            `json:"distribution"`
	ConnectionType     string            `json:"connectionType"`
	Status             string            `json:"status"`
	Labels             map[string]string `json:"labels"`
	IsActive           bool              `json:"isActive"`
	CreatedAt          time.Time         `json:"createdAt"`
	StaleThresholdSec  int               `json:"staleThresholdSeconds,omitempty"`
	ConnectionEndpoint string            `json:"connectionEndpoint,omitempty"`
}

// ListRuntimeTargets reads the server-authoritative cluster list. Availability
// and refusal mapping match GetOperation exactly: no local fallback, no guess.
func (p *PlatformClient) ListRuntimeTargets(ctx context.Context) ([]RuntimeTargetSummary, error) {
	if strings.TrimSpace(p.Token) == "" {
		return nil, ErrNotAuthenticated
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/api/v1/resources/clusters", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("%w: platform returned HTTP %d", ErrPlatformUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: platform returned HTTP %d", ErrRequestRejected, resp.StatusCode)
	}
	var decoded struct {
		Items []struct {
			ClusterID      string    `json:"clusterId"`
			DisplayName    string    `json:"displayName"`
			Kind           string    `json:"kind"`
			Status         string    `json:"status"`
			RuntimeVersion string    `json:"runtimeVersion"`
			CreatedAt      time.Time `json:"createdAt"`
			Acceptance     struct {
				Available bool `json:"available"`
			} `json:"acceptance"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("%w: platform returned an unreadable target list", ErrPlatformUnavailable)
	}
	if len(decoded.Items) != decoded.Total {
		return nil, fmt.Errorf("%w: platform returned an inconsistent target list", ErrPlatformUnavailable)
	}
	targets := make([]RuntimeTargetSummary, 0, len(decoded.Items))
	for _, item := range decoded.Items {
		targets = append(targets, RuntimeTargetSummary{
			ID: item.ClusterID, Name: item.DisplayName, DisplayName: item.DisplayName,
			TargetType: item.Kind, Distribution: item.RuntimeVersion, ConnectionType: "agent",
			Status: item.Status, IsActive: item.Acceptance.Available, CreatedAt: item.CreatedAt,
		})
	}
	return targets, nil
}

// GetOperation reads server-authoritative operation state. Availability problems
// again return ErrPlatformUnavailable rather than a local guess.
func (p *PlatformClient) GetOperation(ctx context.Context, operationID string) (OperationRef, error) {
	var ref OperationRef
	if strings.TrimSpace(p.Token) == "" {
		return ref, ErrNotAuthenticated
	}
	if !validName(operationID) {
		return ref, fmt.Errorf("%w: invalid operation id", ErrRequestRejected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/api/v1/operations/"+operationID, nil)
	if err != nil {
		return ref, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ref, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusServiceUnavailable {
		return ref, fmt.Errorf("%w: platform returned HTTP %d", ErrPlatformUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return ref, fmt.Errorf("%w: platform returned HTTP %d", ErrRequestRejected, resp.StatusCode)
	}
	var decoded struct {
		OperationID string `json:"operationId"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ref, fmt.Errorf("%w: platform returned an unreadable operation", ErrPlatformUnavailable)
	}
	return OperationRef{OperationID: decoded.OperationID, Status: decoded.Status}, nil
}

// TokenTenantID decodes the tenant claim for a client-side mismatch check.
// It is not used as an authorization decision; the server remains authoritative.
func (p *PlatformClient) TokenTenantID() string {
	parts := strings.Split(p.Token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		TenantID string `json:"tenantId"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	return claims.TenantID
}

func (p *PlatformClient) ApplicationData(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.BaseURL, "/")+path, reader)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(payload, &problem)
		code := problem.Code
		if code == "" {
			code = problem.Detail
		}
		if code == "" {
			code = fmt.Sprintf("HTTP_%d", resp.StatusCode)
		}
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: %s", ErrPlatformUnavailable, code)
		}
		return nil, fmt.Errorf("%w: %s", ErrRequestRejected, code)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || len(envelope.Data) == 0 {
		return nil, fmt.Errorf("%w: platform returned unreadable application data", ErrPlatformUnavailable)
	}
	return envelope.Data, nil
}

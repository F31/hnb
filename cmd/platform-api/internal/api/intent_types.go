package api

// IntentRequest is the canonical submission body for POST /v1/intents.
type IntentRequest struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   IntentMetadata `json:"metadata"`
	Spec       IntentSpec     `json:"spec"`
}

// IntentMetadata matches the runtime-intent.schema.json metadata section.
type IntentMetadata struct {
	IdempotencyKey string `json:"idempotencyKey"`
	CorrelationID  string `json:"correlationId,omitempty"`
}

// IntentSpec matches the runtime-intent.schema.json spec section.
type IntentSpec struct {
	ReleaseID        string            `json:"releaseId"`
	TargetRef        string            `json:"targetRef"`
	ScopeRef         string            `json:"scopeRef"`
	Parameters       map[string]any    `json:"parameters,omitempty"`
	SecretReferences []IntentSecretRef `json:"secretReferences,omitempty"`
}

// IntentSecretRef mirrors common/v1/secret-reference.schema.json.
type IntentSecretRef struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
}

// IntentResponse is returned after successful intent processing.
type IntentResponse struct {
	IntentID       string `json:"intentId"`
	OperationID    string `json:"operationId"`
	PlanID         string `json:"planId"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	CorrelationID  string `json:"correlationId"`
	CreatedAt      string `json:"createdAt"`
	SemanticDigest string `json:"semanticDigest"`
	Replayed       bool   `json:"replayed"`
}

type BatchDeleteRuntimeTargetsRequest struct {
	TargetIDs      []string `json:"targetIds"`
	IdempotencyKey string   `json:"idempotencyKey"`
	CorrelationID  string   `json:"correlationId"`
}

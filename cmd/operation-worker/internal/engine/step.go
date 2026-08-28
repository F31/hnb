package engine

import (
	"crypto/sha256"
	"fmt"
	"time"
)

type StepExecution struct {
	StepID         string
	OperationID    string
	Spec           StepSpec
	IdempotencyKey string
	Checkpoint     string
	Attempt        int
	MaxAttempts    int
	Timeout        time.Duration
	StartedAt      time.Time
}

func NewIdempotencyKey(operationID, stepID string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", operationID, stepID)))
	return fmt.Sprintf("%x", h[:16])
}

type StepExecutor struct {
	checkpoints map[string]string
}

func NewStepExecutor() *StepExecutor {
	return &StepExecutor{
		checkpoints: make(map[string]string),
	}
}

type ExecutionContext struct {
	StepID                  string         `json:"step_id"`
	OperationID             string         `json:"operation_id"`
	TenantID                string         `json:"tenant_id"`
	ProjectID               string         `json:"project_id"`
	EnvironmentID           string         `json:"environment_id"`
	StepType                string         `json:"step_type"`
	Inputs                  map[string]any `json:"inputs"`
	Outputs                 map[string]any `json:"outputs"`
	ProviderID              string         `json:"provider_id"`
	ProviderVersion         string         `json:"provider_version"`
	ProviderDigest          string         `json:"provider_digest"`
	ProviderProtocolVersion string         `json:"provider_protocol_version"`
	Checkpoint              string         `json:"checkpoint,omitempty"`
	IdempotencyKey          string         `json:"idempotency_key"`
	ExecutionAttemptID      string         `json:"execution_attempt_id"`
	FencingGeneration       int64          `json:"-"`
	NodeGroupAffinity       []string       `json:"node_group_affinity,omitempty"`
	TargetClusterIDs        []string       `json:"target_cluster_ids,omitempty"`
}

type StepResult struct {
	StepID       string         `json:"step_id"`
	OperationID  string         `json:"operation_id"`
	Status       StepStatus     `json:"status"`
	Outputs      map[string]any `json:"outputs"`
	Checkpoint   string         `json:"checkpoint"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	CompletedAt  time.Time      `json:"completed_at"`
}

type Operation struct {
	ID               string            `json:"id"`
	TenantID         string            `json:"tenant_id"`
	ProjectID        string            `json:"project_id"`
	EnvironmentID    string            `json:"environment_id"`
	NamespaceID      string            `json:"namespace_id"`
	PlanID           string            `json:"plan_id"`
	Type             OperationType     `json:"type"`
	Status           OperationStatus   `json:"status"`
	InitiatedBy      string            `json:"initiated_by"`
	ApprovedBy       string            `json:"approved_by"`
	CorrelationID    string            `json:"correlation_id"`
	IdempotencyKey   string            `json:"idempotency_key"`
	PlanDigest       string            `json:"plan_digest"`
	TotalSteps       int               `json:"total_steps"`
	CompletedSteps   int               `json:"completed_steps"`
	FailedSteps      int               `json:"failed_steps"`
	Version          int64             `json:"version"`
	TargetClusterIDs []string          `json:"target_cluster_ids"`
	Tags             map[string]string `json:"tags"`
	CreatedAt        time.Time         `json:"created_at"`
	StartedAt        *time.Time        `json:"started_at"`
	CompletedAt      *time.Time        `json:"completed_at"`
}

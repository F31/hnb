package api

import (
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
)

type submitStepRequest struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	StepType        string            `json:"stepType"`
	ProviderID      string            `json:"providerId"`
	DependsOn       []string          `json:"dependsOn"`
	Optional        bool              `json:"optional"`
	Inputs          map[string]string `json:"inputs"`
	SecretReference string            `json:"secretReference"`
	MaxRetries      int               `json:"maxRetries"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
}

type submitOperationRequest struct {
	TenantID         string              `json:"tenantId"`
	ProjectID        string              `json:"projectId"`
	EnvironmentID    string              `json:"environmentId"`
	NamespaceID      string              `json:"namespaceId"`
	ReleaseID        string              `json:"releaseId"`
	OperationType    string              `json:"operationType"`
	IdempotencyKey   string              `json:"idempotencyKey"`
	InitiatedBy      string              `json:"initiatedBy"`
	CorrelationID    string              `json:"correlationId"`
	TargetClusterIDs []string            `json:"targetClusterIds"`
	SchedulingPolicy *schedulingPolicy   `json:"schedulingPolicy,omitempty"`
	Tags             map[string]string   `json:"tags"`
	Steps            []submitStepRequest `json:"steps"`
}

type schedulingPolicy struct {
	Strategy string           `json:"strategy"`
	Selector *clusterSelector `json:"selector,omitempty"`
}

type clusterSelector struct {
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
	Region        string            `json:"region,omitempty"`
	Zone          string            `json:"zone,omitempty"`
	ClusterIDs    []string          `json:"clusterIds,omitempty"`
}

type actionRequest struct {
	TenantID string `json:"tenantId"`
	ActorID  string `json:"actorId"`
	Reason   string `json:"reason"`
}

type stepResponse struct {
	ID           string     `json:"id"`
	PlanStepID   string     `json:"planStepId,omitempty"`
	Name         string     `json:"name"`
	StepType     string     `json:"stepType"`
	ProviderID   string     `json:"providerId,omitempty"`
	Status       string     `json:"status"`
	DependsOn    []string   `json:"dependsOn,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

// operationResponse always carries lastStateChangedAt and lastObservedAt from
// the read model (KERNEL-003).
type operationResponse struct {
	ID                 string         `json:"id"`
	IntentID           string         `json:"intentId,omitempty"`
	TenantID           string         `json:"tenantId"`
	ProjectID          string         `json:"projectId,omitempty"`
	EnvironmentID      string         `json:"environmentId,omitempty"`
	NamespaceID        string         `json:"namespaceId,omitempty"`
	PlanID             string         `json:"planId,omitempty"`
	OperationType      string         `json:"operationType"`
	Status             string         `json:"status"`
	InitiatedBy        string         `json:"initiatedBy"`
	ApprovedBy         string         `json:"approvedBy,omitempty"`
	IdempotencyKey     string         `json:"idempotencyKey,omitempty"`
	TotalSteps         int            `json:"totalSteps"`
	CompletedSteps     int            `json:"completedSteps"`
	FailedSteps        int            `json:"failedSteps"`
	TargetClusterIDs   []string       `json:"targetClusterIds,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	StartedAt          *time.Time     `json:"startedAt,omitempty"`
	CompletedAt        *time.Time     `json:"completedAt,omitempty"`
	LastStateChangedAt time.Time      `json:"last_state_changed_at"`
	LastObservedAt     time.Time      `json:"lastObservedAt"`
	Steps              []stepResponse `json:"steps,omitempty"`
}

type operationSummaryResponse struct {
	ID                 string     `json:"id"`
	IntentID           string     `json:"intentId,omitempty"`
	TenantID           string     `json:"tenantId"`
	ProjectID          string     `json:"projectId,omitempty"`
	EnvironmentID      string     `json:"environmentId,omitempty"`
	NamespaceID        string     `json:"namespaceId,omitempty"`
	OperationType      string     `json:"operationType"`
	Status             string     `json:"status"`
	TotalSteps         int        `json:"totalSteps"`
	CompletedSteps     int        `json:"completedSteps"`
	FailedSteps        int        `json:"failedSteps"`
	InitiatedBy        string     `json:"initiatedBy"`
	ApprovedBy         string     `json:"approvedBy,omitempty"`
	Summary            string     `json:"summary,omitempty"`
	TargetClusterIDs   []string   `json:"targetClusterIds,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	LastStateChangedAt time.Time  `json:"last_state_changed_at"`
	LastObservedAt     time.Time  `json:"lastObservedAt"`
}

type listOperationsResponse struct {
	Operations []operationSummaryResponse `json:"operations"`
	Total      int                        `json:"total"`
	Limit      int                        `json:"limit"`
	Offset     int                        `json:"offset"`
}

type problemDetails struct {
	Type          string             `json:"type"`
	Title         string             `json:"title"`
	Status        int                `json:"status"`
	Detail        string             `json:"detail,omitempty"`
	Instance      string             `json:"instance,omitempty"`
	Code          string             `json:"code"`
	CorrelationID string             `json:"correlationId"`
	TraceID       string             `json:"traceId"`
	Retryable     bool               `json:"retryable,omitempty"`
	Violations    []problemViolation `json:"violations,omitempty"`
}

type problemViolation struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

type createRuntimeTargetRequest struct {
	TenantID           string            `json:"tenantId"`
	Name               string            `json:"name"`
	DisplayName        string            `json:"displayName,omitempty"`
	TargetType         string            `json:"targetType"`
	Distribution       string            `json:"distribution,omitempty"`
	EdgeType           string            `json:"edgeType,omitempty"`
	EdgeConfig         map[string]any    `json:"edgeConfig,omitempty"`
	ConnectionType     string            `json:"connectionType,omitempty"`
	ConnectionEndpoint string            `json:"connectionEndpoint,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	StaleThresholdSec  int               `json:"staleThresholdSeconds,omitempty"`
}

type runtimeTargetResponse struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenantId"`
	Name               string            `json:"name"`
	DisplayName        string            `json:"displayName,omitempty"`
	TargetType         string            `json:"targetType"`
	Distribution       string            `json:"distribution"`
	EdgeType           string            `json:"edgeType,omitempty"`
	EdgeConfig         map[string]any    `json:"edgeConfig,omitempty"`
	ConnectionType     string            `json:"connectionType"`
	ConnectionEndpoint string            `json:"connectionEndpoint,omitempty"`
	Status             string            `json:"status"`
	Labels             map[string]string `json:"labels"`
	ObservedAt         *time.Time        `json:"observedAt,omitempty"`
	StaleThresholdSec  int               `json:"staleThresholdSeconds"`
	IsActive           bool              `json:"isActive"`
	CreatedAt          time.Time         `json:"createdAt"`
}

type listRuntimeTargetsResponse struct {
	Targets []runtimeTargetResponse `json:"targets"`
	Total   int                     `json:"total"`
}

func toOperationResponse(op *store.Operation) operationResponse {
	resp := operationResponse{
		ID:                 op.ID,
		IntentID:           op.IntentID,
		TenantID:           op.TenantID,
		ProjectID:          op.ProjectID,
		EnvironmentID:      op.EnvironmentID,
		NamespaceID:        op.NamespaceID,
		PlanID:             op.PlanID,
		OperationType:      op.OperationType,
		Status:             op.Status,
		InitiatedBy:        op.InitiatedBy,
		ApprovedBy:         op.ApprovedBy,
		IdempotencyKey:     op.IdempotencyKey,
		TotalSteps:         op.TotalSteps,
		CompletedSteps:     op.CompletedSteps,
		FailedSteps:        op.FailedSteps,
		TargetClusterIDs:   op.TargetClusterIDs,
		CreatedAt:          op.CreatedAt,
		StartedAt:          op.StartedAt,
		CompletedAt:        op.CompletedAt,
		LastStateChangedAt: op.LastStateChangedAt,
		LastObservedAt:     op.LastStateChangedAt,
	}
	for _, step := range op.Steps {
		resp.Steps = append(resp.Steps, stepResponse{
			ID:           step.ID,
			PlanStepID:   step.PlanStepID,
			Name:         step.Name,
			StepType:     step.StepType,
			ProviderID:   step.ProviderID,
			Status:       step.Status,
			DependsOn:    step.DependsOn,
			ErrorMessage: step.ErrorMessage,
			StartedAt:    step.StartedAt,
			CompletedAt:  step.CompletedAt,
		})
	}
	return resp
}

func toSummaryResponse(item store.OperationSummary) operationSummaryResponse {
	return operationSummaryResponse{
		ID:                 item.ID,
		IntentID:           item.IntentID,
		TenantID:           item.TenantID,
		ProjectID:          item.ProjectID,
		EnvironmentID:      item.EnvironmentID,
		NamespaceID:        item.NamespaceID,
		OperationType:      item.OperationType,
		Status:             item.Status,
		TotalSteps:         item.TotalSteps,
		CompletedSteps:     item.CompletedSteps,
		FailedSteps:        item.FailedSteps,
		InitiatedBy:        item.InitiatedBy,
		ApprovedBy:         item.ApprovedBy,
		Summary:            item.Summary,
		TargetClusterIDs:   item.TargetClusterIDs,
		CreatedAt:          item.CreatedAt,
		StartedAt:          item.StartedAt,
		CompletedAt:        item.CompletedAt,
		LastStateChangedAt: item.LastStateChangedAt,
		LastObservedAt:     item.LastStateChangedAt,
	}
}

func toRuntimeTargetResponse(rt *store.RuntimeTarget) runtimeTargetResponse {
	return runtimeTargetResponse{
		ID:                 rt.ID,
		TenantID:           rt.TenantID,
		Name:               rt.Name,
		DisplayName:        rt.DisplayName,
		TargetType:         rt.TargetType,
		Distribution:       rt.Distribution,
		EdgeType:           rt.EdgeType,
		EdgeConfig:         rt.EdgeConfig,
		ConnectionType:     rt.ConnectionType,
		ConnectionEndpoint: rt.ConnectionEndpoint,
		Status:             rt.Status,
		Labels:             rt.Labels,
		ObservedAt:         rt.ObservedAt,
		StaleThresholdSec:  rt.StaleThresholdSec,
		IsActive:           rt.IsActive,
		CreatedAt:          rt.CreatedAt,
	}
}

// MenuItemResponse is a single menu entry in the accessible menu tree.
type MenuItemResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Group       string `json:"group"`
	Order       int    `json:"order"`
}

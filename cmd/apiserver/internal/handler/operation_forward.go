package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

// ---------------------------------------------------------------------------
// Operation BFF (7.1 / 7.2)
//
// These handlers are the browser-facing Operation Read Model and action
// forwarding. They are tenant-scoped, never read the database directly for the
// projection, and forward to the canonical platform-api versioned API with a
// trusted service delegation that preserves the actor, correlation, and the
// operation-scoped action.
// ---------------------------------------------------------------------------

// consoleOperationStatus is the browser-facing OperationStatus enum.
const (
	consoleOpPending       = "pending"
	consoleOpPendingAppr   = "pending_approval"
	consoleOpQueued        = "queued"
	consoleOpQueuedOffline = "queued_offline"
	consoleOpInProgress    = "in_progress"
	consoleOpPaused        = "paused"
	consoleOpCompensating  = "compensating"
	consoleOpSucceeded     = "succeeded"
	consoleOpFailed        = "failed"
	consoleOpCancelled     = "cancelled"
)

type consoleOperationProgress struct {
	CompletedSteps int `json:"completedSteps"`
	TotalSteps     int `json:"totalSteps"`
	Percent        int `json:"percent"`
}

type consoleSafeFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type consoleOperationSummary struct {
	OperationID   string                   `json:"operationId"`
	IntentID      string                   `json:"intentId"`
	Type          string                   `json:"type"`
	Status        string                   `json:"status"`
	TargetID      string                   `json:"targetId"`
	TargetKind    string                   `json:"targetKind"`
	Progress      consoleOperationProgress `json:"progress"`
	Failure       *consoleSafeFailure      `json:"failure,omitempty"`
	CorrelationID string                   `json:"correlationId"`
	CreatedAt     string                   `json:"createdAt"`
	UpdatedAt     string                   `json:"updatedAt"`
	CompletedAt   string                   `json:"completedAt,omitempty"`
}

type consoleOperationStep struct {
	StepID      string              `json:"stepId"`
	Name        string              `json:"name"`
	Status      string              `json:"status"`
	Attempt     int                 `json:"attempt"`
	StartedAt   string              `json:"startedAt,omitempty"`
	CompletedAt string              `json:"completedAt,omitempty"`
	Failure     *consoleSafeFailure `json:"failure,omitempty"`
}

type consoleOperationLinks struct {
	Operation string `json:"operation"`
	Intent    string `json:"intent,omitempty"`
	Target    string `json:"target,omitempty"`
}

type consoleOperationDetail struct {
	consoleOperationSummary
	ExecutionPlanID string                 `json:"executionPlanId"`
	Steps           []consoleOperationStep `json:"steps"`
	AllowedActions  []string               `json:"allowedActions"`
	Links           consoleOperationLinks  `json:"links"`
}

type consoleOperationListResponse struct {
	APIVersion string                    `json:"apiVersion"`
	Items      []consoleOperationSummary `json:"items"`
	Pagination consolePageMetadata       `json:"pagination"`
}

type consoleOperationDetailResponse struct {
	APIVersion string                 `json:"apiVersion"`
	Data       consoleOperationDetail `json:"data"`
}

type consolePageMetadata struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"pageSize"`
	Total      int  `json:"total"`
	PageCount  int  `json:"pageCount"`
	ExactTotal bool `json:"exactTotal"`
}

// platformOperation is the JSON shape returned by platform-api /v1/operations.
type platformOperation struct {
	ID               string           `json:"id"`
	IntentID         string           `json:"intentId"`
	TenantID         string           `json:"tenantId"`
	ProjectID        string           `json:"projectId"`
	EnvironmentID    string           `json:"environmentId"`
	NamespaceID      string           `json:"namespaceId"`
	PlanID           string           `json:"planId"`
	OperationType    string           `json:"operationType"`
	Status           string           `json:"status"`
	InitiatedBy      string           `json:"initiatedBy"`
	IdempotencyKey   string           `json:"idempotencyKey"`
	TotalSteps       int              `json:"totalSteps"`
	CompletedSteps   int              `json:"completedSteps"`
	FailedSteps      int              `json:"failedSteps"`
	TargetClusterIDs []string         `json:"targetClusterIds"`
	CreatedAt        string           `json:"createdAt"`
	StartedAt        string           `json:"startedAt"`
	CompletedAt      string           `json:"completedAt"`
	Steps            []platformOpStep `json:"steps"`
}

type platformOpStep struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
	StartedAt    string `json:"startedAt"`
	CompletedAt  string `json:"completedAt"`
}

type platformOperationSummary struct {
	ID               string   `json:"id"`
	IntentID         string   `json:"intentId"`
	TenantID         string   `json:"tenantId"`
	ProjectID        string   `json:"projectId"`
	OperationType    string   `json:"operationType"`
	Status           string   `json:"status"`
	TotalSteps       int      `json:"totalSteps"`
	CompletedSteps   int      `json:"completedSteps"`
	FailedSteps      int      `json:"failedSteps"`
	InitiatedBy      string   `json:"initiatedBy"`
	Summary          string   `json:"summary"`
	TargetClusterIDs []string `json:"targetClusterIds"`
	CreatedAt        string   `json:"createdAt"`
	CompletedAt      string   `json:"completedAt"`
	LastObservedAt   string   `json:"lastObservedAt"`
}

type platformOperationList struct {
	Operations []platformOperationSummary `json:"operations"`
	Total      int                        `json:"total"`
	Limit      int                        `json:"limit"`
	Offset     int                        `json:"offset"`
}

// ListOperations forwards the tenant-scoped Operation list to platform-api.
func (h *ResourceClusterHandler) ListOperations(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	token, ok := h.signOperationDelegation(r, trusted, "", iam.ActionList)
	if !ok {
		writeUpstreamUnavailable(w, r)
		return
	}
	query := r.URL.Query()
	page := atoiDefault(query.Get("page"), 1)
	pageSize := atoiDefault(query.Get("pageSize"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	target := h.platformURL + "/v1/operations?limit=" + strconv.Itoa(pageSize) + "&offset=" + strconv.Itoa((page-1)*pageSize)
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		target += "&status=" + url.QueryEscape(status)
	}
	if opType := strings.TrimSpace(query.Get("type")); opType != "" {
		target += "&type=" + url.QueryEscape(opType)
	}

	body, statusCode, err := h.forwardOperation(r, token, http.MethodGet, target, nil)
	if err != nil || statusCode >= 400 {
		writeMappedUpstreamProblem(w, r, statusCode, "", body)
		return
	}
	var list platformOperationList
	if err := json.Unmarshal(body, &list); err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	items := make([]consoleOperationSummary, 0, len(list.Operations))
	for _, item := range list.Operations {
		items = append(items, h.toConsoleSummary(r, item))
	}
	pageCount := 0
	if pageSize > 0 {
		pageCount = (list.Total + pageSize - 1) / pageSize
	}
	writeJSONRaw(w, consoleOperationListResponse{
		APIVersion: "ui.hnb.io/v1",
		Items:      items,
		Pagination: consolePageMetadata{Page: page, PageSize: pageSize, Total: list.Total, PageCount: pageCount, ExactTotal: true},
	})
}

// GetOperation forwards the tenant-scoped Operation detail to platform-api.
func (h *ResourceClusterHandler) GetOperation(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	if id == "" || !uuidStr(id) {
		response.NotFound(w, "operation not found")
		return
	}
	token, ok := h.signOperationDelegation(r, trusted, id, iam.ActionRead)
	if !ok {
		writeUpstreamUnavailable(w, r)
		return
	}
	body, statusCode, err := h.forwardOperation(r, token, http.MethodGet, h.platformURL+"/v1/operations/"+id, nil)
	if err != nil || statusCode >= 400 {
		writeMappedUpstreamProblem(w, r, statusCode, "", body)
		return
	}
	var op platformOperation
	if err := json.Unmarshal(body, &op); err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	writeJSONRaw(w, consoleOperationDetailResponse{APIVersion: "ui.hnb.io/v1", Data: h.toConsoleDetail(r, op)})
}

// OperationApprove/Reject/Cancel forward the operation action to platform-api,
// preserving the actor and correlation context.
func (h *ResourceClusterHandler) OperationApprove(w http.ResponseWriter, r *http.Request) {
	h.forwardOperationAction(w, r, "approve", iam.ActionApprove)
}

func (h *ResourceClusterHandler) OperationReject(w http.ResponseWriter, r *http.Request) {
	h.forwardOperationAction(w, r, "reject", iam.ActionReject)
}

func (h *ResourceClusterHandler) OperationCancel(w http.ResponseWriter, r *http.Request) {
	h.forwardOperationAction(w, r, "cancel", iam.ActionCancel)
}

func (h *ResourceClusterHandler) forwardOperationAction(w http.ResponseWriter, r *http.Request, suffix string, action iam.AuthorizationAction) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	id = strings.TrimSuffix(id, "/actions/"+suffix)
	if id == "" || !uuidStr(id) {
		response.NotFound(w, "operation not found")
		return
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	token, ok := h.signOperationDelegation(r, trusted, id, action)
	if !ok {
		writeUpstreamUnavailable(w, r)
		return
	}
	body, statusCode, err := h.forwardOperation(r, token, http.MethodPost, h.platformURL+"/v1/operations/"+id+"/"+suffix, bodyBytes)
	if err != nil || statusCode >= 400 {
		writeMappedUpstreamProblem(w, r, statusCode, "", body)
		return
	}
	var op platformOperation
	if err := json.Unmarshal(body, &op); err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	writeJSONRaw(w, consoleOperationDetailResponse{APIVersion: "ui.hnb.io/v1", Data: h.toConsoleDetail(r, op)})
}

func (h *ResourceClusterHandler) signOperationDelegation(r *http.Request, trusted iam.TrustedContext, resourceID string, action iam.AuthorizationAction) (string, bool) {
	if h.delegationSigner == nil {
		return "", false
	}
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{
		Scope: iam.DelegationScope{
			ResourceKind: string(iam.ResourceOperation), ResourceID: resourceID,
			ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
		},
		Action: action, CorrelationID: trusted.CorrelationID,
	})
	if err != nil {
		return "", false
	}
	return token, true
}

func (h *ResourceClusterHandler) forwardOperation(r *http.Request, token, method, target string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(r.Context(), method, target, strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if trusted, ok := iam.TrustedContextFrom(r.Context()); ok {
		req.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	}
	copyHeader(req.Header, r.Header, "X-Trace-Id")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, 0, err
	}
	return data, resp.StatusCode, nil
}

func (h *ResourceClusterHandler) toConsoleSummary(r *http.Request, item platformOperationSummary) consoleOperationSummary {
	targetID := ""
	if len(item.TargetClusterIDs) > 0 {
		targetID = item.TargetClusterIDs[0]
	}
	targetKind := h.targetKindFor(r, item.TenantID, targetID)
	percent := 0
	if item.TotalSteps > 0 {
		percent = item.CompletedSteps * 100 / item.TotalSteps
	}
	return consoleOperationSummary{
		OperationID:   item.ID,
		IntentID:      item.IntentID,
		Type:          item.OperationType,
		Status:        consoleStatus(item.Status),
		TargetID:      targetID,
		TargetKind:    targetKind,
		Progress:      consoleOperationProgress{CompletedSteps: item.CompletedSteps, TotalSteps: item.TotalSteps, Percent: percent},
		Failure:       consoleFailureFor(item.Status, item.FailedSteps),
		CorrelationID: item.ID,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.LastObservedAt,
		CompletedAt:   item.CompletedAt,
	}
}

func (h *ResourceClusterHandler) toConsoleDetail(r *http.Request, op platformOperation) consoleOperationDetail {
	summary := h.toConsoleSummary(r, platformOperationSummary{
		ID: op.ID, IntentID: op.IntentID, TenantID: op.TenantID, OperationType: op.OperationType,
		Status: op.Status, TotalSteps: op.TotalSteps, CompletedSteps: op.CompletedSteps, FailedSteps: op.FailedSteps,
		TargetClusterIDs: op.TargetClusterIDs, CreatedAt: op.CreatedAt, CompletedAt: op.CompletedAt, LastObservedAt: op.CompletedAt,
	})
	if summary.CompletedAt == "" && op.CompletedAt != "" {
		summary.CompletedAt = op.CompletedAt
	}
	steps := make([]consoleOperationStep, 0, len(op.Steps))
	for _, step := range op.Steps {
		steps = append(steps, consoleOperationStep{
			StepID: step.ID, Name: step.Name, Status: consoleStepStatus(step.Status),
			StartedAt: step.StartedAt, CompletedAt: step.CompletedAt,
			Failure: consoleFailureFor(step.Status, boolInt(step.ErrorMessage != "")),
		})
	}
	links := consoleOperationLinks{Operation: "/operations/" + op.ID}
	if summary.IntentID != "" {
		links.Intent = "/runtime-intents/" + summary.IntentID
	}
	if summary.TargetID != "" {
		links.Target = "/resource/clusters/" + summary.TargetID
	}
	return consoleOperationDetail{
		consoleOperationSummary: summary,
		ExecutionPlanID:         op.PlanID,
		Steps:                   steps,
		AllowedActions:          allowedActionsFor(op.Status),
		Links:                   links,
	}
}

// targetKindFor resolves the target kind (KubernetesTarget/EdgeRuntimeTarget)
// for an operation's target, falling back to a conservative guess based on the
// operation type when the target is not resolvable.
func (h *ResourceClusterHandler) targetKindFor(r *http.Request, tenantID, targetID string) string {
	if targetID != "" && h.db != nil {
		var targetType string
		err := h.db.QueryRowContext(r.Context(),
			`SELECT target_type FROM runtime_targets WHERE id = $1 AND tenant_id = $2`,
			targetID, tenantID,
		).Scan(&targetType)
		if err == nil {
			if kind := targetTypeToKind(targetType); kind != "" {
				return kind
			}
		}
	}
	return "KubernetesTarget"
}

func consoleStatus(status string) string {
	switch status {
	case "pending", "pending_approval", "queued", "queued_offline", "in_progress", "paused", "compensating", "succeeded", "failed", "cancelled":
		return status
	default:
		return consoleOpPending
	}
}

func consoleStepStatus(status string) string {
	switch status {
	case "pending", "queued", "in_progress", "paused", "succeeded", "failed", "cancelled", "compensating":
		return status
	default:
		return "pending"
	}
}

func allowedActionsFor(status string) []string {
	switch status {
	case "pending_approval", "pending":
		return []string{"approve", "reject"}
	case "queued", "queued_offline", "in_progress":
		return []string{"cancel"}
	default:
		return nil
	}
}

func consoleFailureFor(status string, failedSteps int) *consoleSafeFailure {
	if status == "failed" {
		return &consoleSafeFailure{Code: "OPERATION_FAILED", Message: "The operation failed.", Retryable: false}
	}
	if failedSteps > 0 {
		return &consoleSafeFailure{Code: "STEP_FAILED", Message: fmt.Sprintf("%d step(s) failed.", failedSteps), Retryable: false}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var _ = sql.ErrNoRows
var _ = time.Now

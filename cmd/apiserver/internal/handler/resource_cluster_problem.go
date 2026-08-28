package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

const maxUpstreamProblemBytes = 32 << 10

var safeProblemField = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\[\]-]{0,255}$`)

type consoleViolation struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

type consoleProblem struct {
	Type              string             `json:"type"`
	Title             string             `json:"title"`
	Status            int                `json:"status"`
	Detail            string             `json:"detail,omitempty"`
	Instance          string             `json:"instance,omitempty"`
	Code              string             `json:"code"`
	CorrelationID     string             `json:"correlationId"`
	TraceID           string             `json:"traceId"`
	Retryable         bool               `json:"retryable,omitempty"`
	Violations        []consoleViolation `json:"violations,omitempty"`
	Confirmation      string             `json:"confirmation,omitempty"`
	TargetID          string             `json:"targetId,omitempty"`
	Action            string             `json:"action,omitempty"`
	LastKnownStateAt  string             `json:"lastKnownStateAt,omitempty"`
	LifecycleState    string             `json:"lifecycleState,omitempty"`
	HealthState       string             `json:"healthState,omitempty"`
	ConnectivityState string             `json:"connectivityState,omitempty"`
	PolicyOutcome     string             `json:"policyOutcome,omitempty"`
}

func writeMappedUpstreamProblem(w http.ResponseWriter, r *http.Request, status int, contentType string, body []byte) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/problem+json" || len(body) == 0 || len(body) > maxUpstreamProblemBytes {
		writeUpstreamUnavailable(w, r)
		return
	}
	var upstream consoleProblem
	if json.Unmarshal(body, &upstream) != nil || upstream.Status != status {
		writeUpstreamUnavailable(w, r)
		return
	}
	code, ok := browserProblemCode(upstream.Code)
	if !ok || !validProblemStatus(code, status) {
		writeUpstreamUnavailable(w, r)
		return
	}
	correlationID, traceID := trustedProblemIDs(r)
	problem := consoleProblem{
		Type:  "https://hnb.cloud/problems/" + strings.ToLower(strings.ReplaceAll(code, "_", "-")),
		Title: http.StatusText(status), Status: status, Detail: browserProblemDetail(code),
		Instance: r.URL.Path, Code: code, CorrelationID: correlationID, TraceID: traceID,
	}
	for _, violation := range upstream.Violations {
		if len(problem.Violations) == 64 || !safeProblemField.MatchString(violation.Field) || !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`).MatchString(violation.Code) {
			continue
		}
		if len(violation.Message) > 512 {
			violation.Message = ""
		}
		problem.Violations = append(problem.Violations, violation)
	}
	if code == "STALE_CONFIRMATION_REQUIRED" && len(upstream.Confirmation) >= 16 && len(upstream.Confirmation) <= 2048 {
		problem.Confirmation = upstream.Confirmation
		problem.TargetID, problem.Action = upstream.TargetID, upstream.Action
		problem.LastKnownStateAt, problem.LifecycleState = upstream.LastKnownStateAt, upstream.LifecycleState
		problem.HealthState, problem.ConnectivityState, problem.PolicyOutcome = upstream.HealthState, upstream.ConnectivityState, upstream.PolicyOutcome
	}
	writeConsoleProblem(w, problem)
}

func writeUpstreamUnavailable(w http.ResponseWriter, r *http.Request) {
	correlationID, traceID := trustedProblemIDs(r)
	writeConsoleProblem(w, consoleProblem{
		Type: "https://hnb.cloud/problems/upstream-unavailable", Title: "Service Unavailable",
		Status: http.StatusServiceUnavailable, Detail: "The platform service is temporarily unavailable.",
		Instance: r.URL.Path, Code: "UPSTREAM_UNAVAILABLE", CorrelationID: correlationID, TraceID: traceID, Retryable: true,
	})
}

func writeLocalClusterProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, violations ...consoleViolation) {
	correlationID, traceID := trustedProblemIDs(r)
	writeConsoleProblem(w, consoleProblem{
		Type:  "https://hnb.cloud/problems/" + strings.ToLower(strings.ReplaceAll(code, "_", "-")),
		Title: http.StatusText(status), Status: status, Detail: detail, Instance: r.URL.Path,
		Code: code, CorrelationID: correlationID, TraceID: traceID, Violations: violations,
	})
}

func writeConsoleProblem(w http.ResponseWriter, problem consoleProblem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Correlation-ID", problem.CorrelationID)
	w.Header().Set("X-Trace-Id", problem.TraceID)
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}

func trustedProblemIDs(r *http.Request) (string, string) {
	correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if trusted, ok := iam.TrustedContextFrom(r.Context()); ok && uuidStr(trusted.CorrelationID) {
		correlationID = trusted.CorrelationID
	}
	if !uuidStr(correlationID) {
		correlationID = uuid.NewString()
	}
	trace := strings.ToLower(strings.ReplaceAll(r.Header.Get("X-Trace-Id"), "-", ""))
	if !regexp.MustCompile(`^[0-9a-f]{16,64}$`).MatchString(trace) {
		digest := sha256.Sum256([]byte(correlationID))
		trace = fmt.Sprintf("%x", digest[:16])
	}
	return correlationID, trace
}

func browserProblemCode(code string) (string, bool) {
	aliases := map[string]string{
		"VALIDATION_FAILED": "VALIDATION_FAILED", "validation_error": "VALIDATION_FAILED", "invalid_request": "VALIDATION_FAILED",
		"NOT_FOUND": "NOT_FOUND", "target_not_found": "NOT_FOUND", "FORBIDDEN": "FORBIDDEN", "permission_denied": "FORBIDDEN",
		"IDEMPOTENCY_CONFLICT": "IDEMPOTENCY_CONFLICT", "idempotency-conflict": "IDEMPOTENCY_CONFLICT",
		"STALE_CONFIRMATION_REQUIRED": "STALE_CONFIRMATION_REQUIRED", "stale-confirmation-required": "STALE_CONFIRMATION_REQUIRED",
		"STALE_CONFIRMATION_EXPIRED": "STALE_CONFIRMATION_EXPIRED", "STALE_POLICY_DENIED": "STALE_POLICY_DENIED",
		"SECRET_REFERENCE_DENIED": "SECRET_REFERENCE_DENIED", "TARGET_VERSION_CONFLICT": "TARGET_VERSION_CONFLICT",
		"TARGET_ACTION_UNSUPPORTED": "TARGET_ACTION_UNSUPPORTED", "PROVIDER_INCOMPATIBLE": "PROVIDER_INCOMPATIBLE",
		"PROVIDER_ROUTE_NOT_FOUND": "PROVIDER_ROUTE_NOT_FOUND", "OPERATION_ACTION_NOT_ALLOWED": "OPERATION_ACTION_NOT_ALLOWED",
	}
	canonical, ok := aliases[code]
	return canonical, ok
}

func validProblemStatus(code string, status int) bool {
	switch code {
	case "VALIDATION_FAILED":
		return status == 400 || status == 422
	case "NOT_FOUND":
		return status == 404
	case "FORBIDDEN":
		return status == 403
	case "STALE_CONFIRMATION_REQUIRED", "STALE_CONFIRMATION_EXPIRED", "STALE_POLICY_DENIED", "IDEMPOTENCY_CONFLICT", "TARGET_VERSION_CONFLICT", "OPERATION_ACTION_NOT_ALLOWED":
		return status == 409
	case "SECRET_REFERENCE_DENIED":
		return status == 403
	default:
		return status >= 400 && status < 500
	}
}

func browserProblemDetail(code string) string {
	details := map[string]string{
		"VALIDATION_FAILED": "One or more request fields are invalid.", "NOT_FOUND": "The requested resource was not found.",
		"FORBIDDEN": "Permission denied.", "IDEMPOTENCY_CONFLICT": "The idempotency key is committed to a different request.",
		"STALE_CONFIRMATION_REQUIRED": "The target observation is stale and requires explicit confirmation.",
		"STALE_CONFIRMATION_EXPIRED":  "The stale confirmation is invalid or expired.", "STALE_POLICY_DENIED": "The stale target policy denied the action.",
		"SECRET_REFERENCE_DENIED": "The secret reference is not authorized for this action.",
		"TARGET_VERSION_CONFLICT": "The runtime target version changed.", "TARGET_ACTION_UNSUPPORTED": "The target does not support this action.",
		"PROVIDER_INCOMPATIBLE": "No compatible lifecycle provider is available.", "PROVIDER_ROUTE_NOT_FOUND": "The lifecycle provider route is unavailable.",
		"OPERATION_ACTION_NOT_ALLOWED": "The operation action is not allowed in its current state.",
	}
	return details[code]
}

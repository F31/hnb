package provider

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

type Server struct {
	profile  Profile
	manager  LifecycleManager
	secrets  SecretResolver
	observer ObserverRegistry
}

func NewServer(profile Profile, manager LifecycleManager, secrets SecretResolver, observer ObserverRegistry) *Server {
	return &Server{profile: profile, manager: manager, secrets: secrets, observer: observer}
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
	SchemaVersion           string            `json:"schemaVersion"`
	Status                  string            `json:"status"`
	ExecutionAttemptID      string            `json:"executionAttemptId"`
	IdempotencyKey          string            `json:"idempotencyKey"`
	FencingGeneration       string            `json:"fencingGeneration"`
	ProviderVersion         string            `json:"providerVersion"`
	ProviderDigest          string            `json:"providerDigest"`
	ProviderProtocolVersion string            `json:"providerProtocolVersion"`
	Outputs                 map[string]string `json:"outputs,omitempty"`
	Checkpoint              string            `json:"checkpoint,omitempty"`
	Error                   string            `json:"error,omitempty"`
	ErrorCode               ErrorCode         `json:"errorCode,omitempty"`
	Retryable               *bool             `json:"retryable,omitempty"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "providerId": s.profile.ProviderID, "contractVersion": ContractVersion})
	})
	mux.HandleFunc("POST /v2/steps:execute", s.handleExecute)
	return mux
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request executeRequest
	if err := decoder.Decode(&request); err != nil {
		writeFailure(w, invalid("invalid request: %v", err), request.Execution)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeFailure(w, invalid("unexpected trailing JSON"), request.Execution)
		return
	}
	if request.SchemaVersion != ContractVersion {
		writeFailure(w, invalid("unsupported schema version %q", request.SchemaVersion), request.Execution)
		return
	}
	execution, err := s.executionContext(request.Execution)
	if err != nil {
		writeFailure(w, err, request.Execution)
		return
	}
	input, err := parseLifecycleInput(s.profile, execution)
	if err != nil {
		writeFailureResult(w, err, execution, Result{})
		return
	}
	if input.CredentialSecretRef != nil {
		secretBytes, err := s.secrets.Resolve(r.Context(), execution.TenantID, s.profile.ProviderID, s.profile.SecretPurpose, *input.CredentialSecretRef)
		if err != nil {
			writeFailureResult(w, err, execution, Result{})
			return
		}
		execution.Inputs["_resolvedSecretContent"] = string(secretBytes)
	}
	result, err := s.manager.Apply(r.Context(), execution, input)
	if err != nil {
		writeFailureResult(w, err, execution, result)
		return
	}
	if input.Action == "create" || input.Action == "import" {
		if err := s.observer.Register(r.Context(), execution.TenantID, input.TargetID, input.TargetKind, s.profile.ObservationKind); err != nil {
			writeFailureResult(w, targetError("register observer", err), execution, result)
			return
		}
	}
	if input.Action == "unmanage" {
		if err := s.observer.Unregister(r.Context(), execution.TenantID, input.TargetID); err != nil {
			writeFailureResult(w, targetError("unregister observer", err), execution, result)
			return
		}
	}
	writeResponse(w, http.StatusOK, execution, result, "succeeded", nil)
}

func (s *Server) executionContext(request executionRequest) (ExecutionContext, error) {
	generation, err := strconv.ParseInt(request.FencingGeneration, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != request.FencingGeneration {
		return ExecutionContext{}, invalid("fencing_generation must be a canonical positive decimal string")
	}
	if request.ProviderID != s.profile.ProviderID {
		return ExecutionContext{}, invalid("provider_id %q does not match %q", request.ProviderID, s.profile.ProviderID)
	}
	if request.ProviderProtocolVersion != ContractVersion {
		return ExecutionContext{}, invalid("provider_protocol_version %q is not supported", request.ProviderProtocolVersion)
	}
	return ExecutionContext{
		StepID: request.StepID, OperationID: request.OperationID, TenantID: request.TenantID,
		ProjectID: request.ProjectID, EnvironmentID: request.EnvironmentID, StepType: request.StepType,
		Inputs: request.Inputs, ProviderID: request.ProviderID, ProviderVersion: request.ProviderVersion,
		ProviderDigest: request.ProviderDigest, ProviderProtocolVersion: request.ProviderProtocolVersion,
		Checkpoint: request.Checkpoint, IdempotencyKey: request.IdempotencyKey,
		ExecutionAttemptID: request.ExecutionAttemptID, FencingGeneration: generation,
	}, nil
}

func writeFailure(w http.ResponseWriter, err error, request executionRequest) {
	generation := request.FencingGeneration
	writeResponse(w, statusCode(err), ExecutionContext{ExecutionAttemptID: request.ExecutionAttemptID, FencingGeneration: parseBestEffortGeneration(generation)}, Result{}, "failed", err)
}

func writeFailureResult(w http.ResponseWriter, err error, execution ExecutionContext, result Result) {
	writeResponse(w, statusCode(err), execution, result, "failed", err)
}

func writeResponse(w http.ResponseWriter, status int, execution ExecutionContext, result Result, resultStatus string, err error) {
	response := executeResponse{
		SchemaVersion: ContractVersion, Status: resultStatus,
		ExecutionAttemptID: execution.ExecutionAttemptID, IdempotencyKey: execution.IdempotencyKey,
		FencingGeneration: strconv.FormatInt(execution.FencingGeneration, 10),
		ProviderVersion:   execution.ProviderVersion, ProviderDigest: execution.ProviderDigest,
		ProviderProtocolVersion: execution.ProviderProtocolVersion,
		Outputs:                 result.Outputs, Checkpoint: result.Checkpoint,
	}
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			response.ErrorCode = statusErr.Code
			response.Retryable = &statusErr.Retryable
		}
		response.Error = err.Error()
	}
	writeJSON(w, status, response)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func statusCode(err error) int {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

func parseBestEffortGeneration(value string) int64 {
	generation, _ := strconv.ParseInt(value, 10, 64)
	return generation
}

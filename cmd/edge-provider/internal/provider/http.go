package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

const contractVersion = "2.0.0"

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

func (request executionRequest) executionContext() (ExecutionContext, error) {
	generation, err := strconv.ParseInt(request.FencingGeneration, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != request.FencingGeneration {
		return ExecutionContext{}, invalid("fencing_generation must be a canonical positive decimal string")
	}
	if request.ProviderProtocolVersion != "" && request.ProviderProtocolVersion != contractVersion {
		return ExecutionContext{}, invalid("provider_protocol_version %q is not supported", request.ProviderProtocolVersion)
	}
	return ExecutionContext{
		StepID:                  request.StepID,
		OperationID:             request.OperationID,
		TenantID:                request.TenantID,
		ProjectID:               request.ProjectID,
		EnvironmentID:           request.EnvironmentID,
		StepType:                request.StepType,
		Inputs:                  request.Inputs,
		ProviderID:              request.ProviderID,
		ProviderVersion:         request.ProviderVersion,
		ProviderDigest:          request.ProviderDigest,
		ProviderProtocolVersion: request.ProviderProtocolVersion,
		Checkpoint:              request.Checkpoint,
		IdempotencyKey:          request.IdempotencyKey,
		ExecutionAttemptID:      request.ExecutionAttemptID,
		FencingGeneration:       generation,
	}, nil
}

type executeResponse struct {
	SchemaVersion           string         `json:"schemaVersion"`
	Status                  string         `json:"status"`
	ExecutionAttemptID      string         `json:"executionAttemptId"`
	IdempotencyKey          string         `json:"idempotencyKey"`
	FencingGeneration       string         `json:"fencingGeneration"`
	ProviderVersion         string         `json:"providerVersion"`
	ProviderDigest          string         `json:"providerDigest"`
	ProviderProtocolVersion string         `json:"providerProtocolVersion"`
	Outputs                 map[string]any `json:"outputs,omitempty"`
	Checkpoint              string         `json:"checkpoint,omitempty"`
	Error                   string         `json:"error,omitempty"`
	ErrorCode               ErrorCode      `json:"errorCode,omitempty"`
	Retryable               *bool          `json:"retryable,omitempty"`
	CloudCoreVersion        string         `json:"cloudCoreVersion,omitempty"`
	EdgeNodes               int            `json:"edgeNodes,omitempty"`
}

type healthResponse struct {
	Status           string `json:"status"`
	ContractVersion  string `json:"contractVersion"`
	CloudCoreVersion string `json:"cloudCoreVersion,omitempty"`
	EdgeNodes        int    `json:"edgeNodes,omitempty"`
}

func NewHandler(executor *Executor) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		version, err := executor.CloudCoreVersion(nil)
		status := "ok"
		if err != nil {
			status = "degraded"
		}
		writeJSON(w, http.StatusOK, healthResponse{
			Status: status, ContractVersion: contractVersion,
			CloudCoreVersion: version,
		})
	})
	mux.HandleFunc("POST /v2/steps:execute", func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 64<<10)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		var input executeRequest
		if err := decoder.Decode(&input); err != nil {
			writeFailure(w, invalid("invalid request: %v", err), input.Execution.ExecutionAttemptID, input.Execution.FencingGeneration)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			writeFailure(w, invalid("unexpected trailing JSON"), input.Execution.ExecutionAttemptID, input.Execution.FencingGeneration)
			return
		}
		if input.SchemaVersion != contractVersion {
			writeFailure(w, invalid("unsupported schema version %q", input.SchemaVersion), input.Execution.ExecutionAttemptID, input.Execution.FencingGeneration)
			return
		}
		execution, err := input.Execution.executionContext()
		if err != nil {
			writeFailure(w, err, input.Execution.ExecutionAttemptID, input.Execution.FencingGeneration)
			return
		}
		result, err := executor.Execute(request.Context(), execution)
		if err != nil {
			writeFailureResult(w, err, execution, result)
			return
		}
		writeResponse(w, 200, executeResponse{
			SchemaVersion: contractVersion, Status: "succeeded",
			ExecutionAttemptID:      execution.ExecutionAttemptID,
			IdempotencyKey:          execution.IdempotencyKey,
			FencingGeneration:       strconv.FormatInt(execution.FencingGeneration, 10),
			ProviderVersion:         execution.ProviderVersion,
			ProviderDigest:          execution.ProviderDigest,
			ProviderProtocolVersion: execution.ProviderProtocolVersion,
			Outputs:                 result.Outputs,
			Checkpoint:              result.Checkpoint,
		})
	})
	return mux
}

func writeFailure(w http.ResponseWriter, err error, executionAttemptID, fencingGeneration string) {
	code, errorCode, retryable := statusDetails(err)
	writeResponse(w, code, executeResponse{
		SchemaVersion: contractVersion, Status: "failed",
		ExecutionAttemptID: executionAttemptID, FencingGeneration: fencingGeneration,
		Error: err.Error(), ErrorCode: errorCode, Retryable: &retryable,
	})
}

func writeFailureResult(w http.ResponseWriter, err error, execution ExecutionContext, result Result) {
	code, errorCode, retryable := statusDetails(err)
	writeResponse(w, code, executeResponse{
		SchemaVersion: contractVersion, Status: "failed",
		ExecutionAttemptID:      execution.ExecutionAttemptID,
		IdempotencyKey:          execution.IdempotencyKey,
		FencingGeneration:       strconv.FormatInt(execution.FencingGeneration, 10),
		ProviderVersion:         execution.ProviderVersion,
		ProviderDigest:          execution.ProviderDigest,
		ProviderProtocolVersion: execution.ProviderProtocolVersion,
		Outputs:                 result.Outputs, Checkpoint: result.Checkpoint,
		Error: err.Error(), ErrorCode: errorCode, Retryable: &retryable,
	})
}

func writeResponse(w http.ResponseWriter, code int, response executeResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(response)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

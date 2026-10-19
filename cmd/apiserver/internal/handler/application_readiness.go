package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/application/readiness"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type ApplicationReadinessHandler struct {
	db *sql.DB
}

func NewApplicationReadinessHandler(db *sql.DB) *ApplicationReadinessHandler {
	return &ApplicationReadinessHandler{db: db}
}

type verificationInput struct {
	OperationID           string `json:"operationId,omitempty"`
	Revision              int64  `json:"revision"`
	ResourceUID           string `json:"resourceUid"`
	ObservationGeneration int64  `json:"observationGeneration"`
	ObservationRevision   int64  `json:"observationRevision"`
	ObservationFreshness  string `json:"observationFreshness"`
	ExecutionStatus       string `json:"executionStatus"`
	ExternalProbeStatus   string `json:"externalProbeStatus"`
	ExternalProbeURL      string `json:"externalProbeUrl,omitempty"`
	VolumeStatus          string `json:"volumeStatus"`
	PVCName               string `json:"pvcName,omitempty"`
	MarkerSHA256          string `json:"markerSha256,omitempty"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (h *ApplicationReadinessHandler) Record(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	bindingID := r.PathValue("id")
	if _, err := uuid.Parse(bindingID); err != nil {
		response.BadRequest(w, "invalid delivery id")
		return
	}
	var input verificationInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.BadRequest(w, "invalid verification body")
		return
	}
	if !validVerification(input) {
		response.BadRequest(w, "invalid verification status or identity")
		return
	}
	expected, err := h.loadBinding(r, trusted.TenantID, bindingID)
	if err != nil {
		response.NotFound(w, "application delivery binding not found")
		return
	}
	result := readiness.Evaluate(readiness.Evidence{
		Revision: input.Revision, ExpectedRevision: expected.revision,
		ResourceUID: input.ResourceUID, ExpectedResourceUID: expected.resourceUID,
		ObservationGeneration: input.ObservationGeneration, ObservationRevision: input.ObservationRevision,
		ExpectedGeneration: expected.observationGeneration, ExpectedObservationRevision: expected.observationRevision,
		ObservationFreshness: input.ObservationFreshness, ExecutionStatus: input.ExecutionStatus,
		ExternalProbeStatus: input.ExternalProbeStatus, ExternalProbeURL: input.ExternalProbeURL,
		VolumeStatus: input.VolumeStatus, PVCName: input.PVCName, MarkerSHA256: input.MarkerSHA256,
	})
	reasons, _ := json.Marshal(result.Reasons)
	_, err = h.db.ExecContext(r.Context(), `
		INSERT INTO application_delivery_verifications (
			binding_id, tenant_id, target_id, release_id, release_version, revision,
			operation_id, resource_uid, observation_generation, observation_revision,
			observation_freshness, execution_status, external_probe_status, external_probe_url,
			volume_status, pvc_name, marker_sha256, ready, reasons, checked_at
		) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,now())
		ON CONFLICT (binding_id, revision) DO UPDATE SET
			operation_id=EXCLUDED.operation_id, resource_uid=EXCLUDED.resource_uid,
			observation_generation=EXCLUDED.observation_generation, observation_revision=EXCLUDED.observation_revision,
			observation_freshness=EXCLUDED.observation_freshness, execution_status=EXCLUDED.execution_status,
			external_probe_status=EXCLUDED.external_probe_status, external_probe_url=EXCLUDED.external_probe_url,
			volume_status=EXCLUDED.volume_status, pvc_name=EXCLUDED.pvc_name, marker_sha256=EXCLUDED.marker_sha256,
			ready=EXCLUDED.ready, reasons=EXCLUDED.reasons, checked_at=now()`,
		bindingID, trusted.TenantID, expected.targetID, expected.releaseID, expected.releaseVersion, input.Revision,
		input.OperationID, input.ResourceUID, input.ObservationGeneration, input.ObservationRevision,
		input.ObservationFreshness, input.ExecutionStatus, input.ExternalProbeStatus, input.ExternalProbeURL,
		input.VolumeStatus, input.PVCName, input.MarkerSHA256, result.Ready, reasons)
	if err != nil {
		response.InternalError(w, "store application verification failed")
		return
	}
	response.Success(w, result)
}

func (h *ApplicationReadinessHandler) Get(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	bindingID := r.PathValue("id")
	if _, err := uuid.Parse(bindingID); err != nil {
		response.BadRequest(w, "invalid delivery id")
		return
	}
	var result struct {
		Ready       bool
		Revision    int64
		ResourceUID string
		Execution   string
		Observation string
		External    string
		Volume      string
		Reasons     []byte
		CheckedAt   time.Time
	}
	err := h.db.QueryRowContext(r.Context(), `
		SELECT ready, revision, resource_uid, execution_status, observation_freshness,
		       external_probe_status, volume_status, reasons, checked_at
		FROM application_delivery_verifications
		WHERE binding_id=$1 AND tenant_id=$2 ORDER BY checked_at DESC LIMIT 1`,
		bindingID, trusted.TenantID).Scan(&result.Ready, &result.Revision, &result.ResourceUID,
		&result.Execution, &result.Observation, &result.External, &result.Volume, &result.Reasons, &result.CheckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "application verification not found")
		return
	}
	if err != nil {
		response.InternalError(w, "read application verification failed")
		return
	}
	var reasons []string
	_ = json.Unmarshal(result.Reasons, &reasons)
	response.Success(w, map[string]any{
		"ready": result.Ready, "revision": result.Revision, "resourceUid": result.ResourceUID,
		"execution": result.Execution, "observation": result.Observation,
		"externalProbe": result.External, "volume": result.Volume,
		"reasons": reasons, "checkedAt": result.CheckedAt,
	})
}

type deliveryBindingIdentity struct {
	targetID, releaseID, releaseVersion, resourceUID     string
	revision, observationGeneration, observationRevision int64
}

func (h *ApplicationReadinessHandler) loadBinding(r *http.Request, tenantID, bindingID string) (deliveryBindingIdentity, error) {
	var item deliveryBindingIdentity
	err := h.db.QueryRowContext(r.Context(), `
		SELECT target_id::text, release_id, release_version, revision, resource_uid,
		       COALESCE(observation_generation,0), COALESCE(observation_revision,0)
		FROM application_delivery_bindings WHERE id=$1 AND tenant_id=$2`,
		bindingID, tenantID).Scan(&item.targetID, &item.releaseID, &item.releaseVersion,
		&item.revision, &item.resourceUID, &item.observationGeneration, &item.observationRevision)
	return item, err
}

func validVerification(input verificationInput) bool {
	if input.Revision <= 0 || strings.TrimSpace(input.ResourceUID) == "" || len(input.ResourceUID) > 256 {
		return false
	}
	if input.MarkerSHA256 != "" && !sha256Pattern.MatchString(input.MarkerSHA256) {
		return false
	}
	if input.OperationID != "" {
		if _, err := uuid.Parse(input.OperationID); err != nil {
			return false
		}
	}
	return oneOf(input.ObservationFreshness, "FRESH", "STALE", "UNKNOWN") &&
		oneOf(input.ExecutionStatus, "succeeded", "failed", "running", "unknown") &&
		oneOf(input.ExternalProbeStatus, "passed", "failed", "not_run") &&
		oneOf(input.VolumeStatus, "passed", "failed", "not_run")
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

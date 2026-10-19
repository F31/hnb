package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type ApplicationDeliveryBinding struct {
	ID                    string    `json:"id"`
	TenantID              string    `json:"tenantId"`
	TargetID              string    `json:"targetId"`
	Namespace             string    `json:"namespace"`
	ReleaseID             string    `json:"releaseId"`
	ReleaseVersion        string    `json:"releaseVersion"`
	Revision              int64     `json:"revision"`
	ResourceUID           string    `json:"resourceUid"`
	PVCName               string    `json:"pvcName,omitempty"`
	IngressName           string    `json:"ingressName,omitempty"`
	OperationID           string    `json:"operationId,omitempty"`
	ObservationGeneration int64     `json:"observationGeneration,omitempty"`
	ObservationRevision   int64     `json:"observationRevision,omitempty"`
	Status                string    `json:"status"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type ApplicationDeliveryHandler struct {
	db *sql.DB
}

func NewApplicationDeliveryHandler(db *sql.DB) *ApplicationDeliveryHandler {
	return &ApplicationDeliveryHandler{db: db}
}

func (h *ApplicationDeliveryHandler) List(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	targetID := strings.TrimSpace(r.URL.Query().Get("targetId"))
	releaseID := strings.TrimSpace(r.URL.Query().Get("releaseId"))
	namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
	if targetID != "" {
		if _, err := uuid.Parse(targetID); err != nil {
			response.BadRequest(w, "targetId must be a UUID")
			return
		}
	}
	if len(releaseID) > 256 || len(namespace) > 63 {
		response.BadRequest(w, "invalid filter")
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			response.BadRequest(w, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, tenant_id, target_id, namespace, release_id, release_version, revision,
		       resource_uid, COALESCE(pvc_name, ''), COALESCE(ingress_name, ''),
		       COALESCE(operation_id::text, ''), COALESCE(observation_generation, 0),
		       COALESCE(observation_revision, 0), status, created_at, updated_at
		FROM (
			SELECT DISTINCT ON (target_id, namespace, release_id) *
			FROM application_delivery_bindings
			WHERE tenant_id = $1
			  AND ($2 = '' OR target_id = $2::uuid)
			  AND ($3 = '' OR release_id = $3)
			  AND ($4 = '' OR namespace = $4)
			ORDER BY target_id, namespace, release_id, revision DESC
		) current
		ORDER BY created_at DESC, revision DESC
		LIMIT $5`, trusted.TenantID, targetID, releaseID, namespace, limit)
	if err != nil {
		response.InternalError(w, "application delivery query failed")
		return
	}
	defer rows.Close()
	items := make([]ApplicationDeliveryBinding, 0)
	for rows.Next() {
		var item ApplicationDeliveryBinding
		if err := rows.Scan(&item.ID, &item.TenantID, &item.TargetID, &item.Namespace, &item.ReleaseID,
			&item.ReleaseVersion, &item.Revision, &item.ResourceUID, &item.PVCName, &item.IngressName,
			&item.OperationID, &item.ObservationGeneration, &item.ObservationRevision, &item.Status,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			response.InternalError(w, "application delivery query failed")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		response.InternalError(w, "application delivery query failed")
		return
	}
	response.Success(w, map[string]any{"items": items, "total": len(items)})
}

package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type ApplicationDeletionPreviewHandler struct{ db *sql.DB }

func NewApplicationDeletionPreviewHandler(db *sql.DB) *ApplicationDeletionPreviewHandler {
	return &ApplicationDeletionPreviewHandler{db: db}
}

type deletionResource struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	UID    string `json:"uid,omitempty"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

func (h *ApplicationDeletionPreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.BadRequest(w, "invalid delivery id")
		return
	}
	var bindingID, targetID, releaseID, resourceUID string
	var workload, service, ingress, pvc sql.NullString
	var revision int64
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id::text, target_id::text, release_id, revision, resource_uid,
		       workload_name, service_name, ingress_name, pvc_name
		FROM application_delivery_bindings WHERE id=$1 AND tenant_id=$2`, id, trusted.TenantID).
		Scan(&bindingID, &targetID, &releaseID, &revision, &resourceUID, &workload, &service, &ingress, &pvc)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "application delivery binding not found")
		return
	}
	if err != nil {
		response.InternalError(w, "deletion preview query failed")
		return
	}
	resources := make([]deletionResource, 0, 4)
	resources = append(resources, deletionResource{Kind: "Deployment", Name: workload.String, UID: resourceUID, Action: "delete"})
	if service.Valid && service.String != "" {
		resources = append(resources, deletionResource{Kind: "Service", Name: service.String, Action: "delete"})
	}
	if ingress.Valid && ingress.String != "" {
		resources = append(resources, deletionResource{Kind: "HTTPRoute", Name: ingress.String, Action: "delete"})
	}
	if pvc.Valid && pvc.String != "" {
		resources = append(resources, deletionResource{Kind: "PersistentVolumeClaim", Name: pvc.String, Action: "retain", Note: "separate data cleanup authorization required"})
	}
	response.Success(w, map[string]any{
		"bindingId": bindingID, "targetId": targetID, "releaseId": releaseID, "revision": revision,
		"resourceUid": resourceUID, "retainVolumes": true,
		"requiresDataCleanupAuthorization": pvc.Valid && pvc.String != "",
		"resources":                        resources,
	})
}

package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

type applicationActionRequest struct {
	Action           string `json:"action"`
	IdempotencyKey   string `json:"idempotencyKey"`
	Image            string `json:"image,omitempty"`
	SourceImage      string `json:"sourceImage,omitempty"`
	TargetImage      string `json:"targetImage,omitempty"`
	ExpectedUID      string `json:"expectedUid,omitempty"`
	BackupRef        string `json:"backupRef,omitempty"`
	DataCompatible   *bool  `json:"dataCompatible,omitempty"`
	DataIrreversible *bool  `json:"dataIrreversible,omitempty"`
	Replicas         string `json:"replicas,omitempty"`
	PVCMountPath     string `json:"pvcMountPath,omitempty"`
	SecretName       string `json:"secretName,omitempty"`
	SecretMountPath  string `json:"secretMountPath,omitempty"`
}

func (h *ResourceClusterHandler) ApplicationAction(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	bindingID := r.PathValue("id")
	var req applicationActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		response.BadRequest(w, "invalid application action body")
		return
	}
	if !validApplicationAction(req.Action) || strings.TrimSpace(req.IdempotencyKey) == "" || len(req.IdempotencyKey) > 128 {
		response.BadRequest(w, "action and idempotencyKey are required")
		return
	}
	var binding struct {
		releaseID      string
		releaseVersion string
		revision       int64
		resourceUID    string
		namespace      string
		workloadName   string
		serviceName    string
		ingressName    string
		pvcName        string
		targetID       string
	}
	err := h.db.QueryRowContext(r.Context(), `
		SELECT release_id, release_version, revision, resource_uid, namespace,
		       COALESCE(workload_name,''), COALESCE(service_name,''), COALESCE(ingress_name,''),
		       COALESCE(pvc_name,''), target_id::text
		FROM application_delivery_bindings
		WHERE id=$1 AND tenant_id=$2`, bindingID, trusted.TenantID).Scan(
		&binding.releaseID, &binding.releaseVersion, &binding.revision, &binding.resourceUID,
		&binding.namespace, &binding.workloadName, &binding.serviceName, &binding.ingressName,
		&binding.pvcName, &binding.targetID)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "application delivery binding not found")
		return
	}
	if err != nil {
		response.InternalError(w, "load application delivery binding failed")
		return
	}
	if binding.workloadName == "" {
		response.BadRequest(w, "binding has no workload name")
		return
	}
	inputs, err := applicationActionInputs(req, binding.namespace, binding.workloadName, binding.pvcName, binding.resourceUID)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	steps := []map[string]any{{
		"id": "application-" + req.Action, "name": "application " + req.Action,
		"stepType": applicationProviderStepType(req.Action), "providerId": "kubernetes-provider",
		"inputs": inputs, "maxRetries": 1, "timeoutSeconds": 900,
	}}
	if req.Action == "delete" {
		// The deployment step tombstones the workload; associated Service and
		// HTTPRoute are deleted through the same provider so the deletion
		// preview matches what is actually executed.
		if binding.serviceName != "" {
			steps = append(steps, map[string]any{
				"id": "application-service-delete", "name": "application service delete",
				"stepType": "service.delete", "providerId": "kubernetes-provider",
				"inputs": map[string]string{"namespace": binding.namespace, "name": binding.serviceName},
				"maxRetries": 1, "timeoutSeconds": 900,
			})
		}
		if binding.ingressName != "" {
			steps = append(steps, map[string]any{
				"id": "application-httproute-delete", "name": "application httproute delete",
				"stepType": "httproute.delete", "providerId": "kubernetes-provider",
				"inputs": map[string]string{"namespace": binding.namespace, "name": binding.ingressName},
				"maxRetries": 1, "timeoutSeconds": 900,
			})
		}
	}
	body, _ := json.Marshal(map[string]any{
		"tenantId": trusted.TenantID, "namespaceId": binding.namespace,
		"releaseId": binding.releaseID, "operationType": req.Action,
		"idempotencyKey": req.IdempotencyKey, "initiatedBy": trusted.SubjectID,
		"correlationId": trusted.CorrelationID, "targetClusterIds": []string{binding.targetID},
		"applicationBindingId": bindingID,
		"tags": map[string]string{
			"applicationDeliveryId": bindingID, "applicationRevision": binding.releaseVersion,
		},
		"steps": steps,
	})
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{
		Scope: iam.DelegationScope{ResourceKind: string(iam.ResourceApplicationDelivery), ResourceID: bindingID,
			ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID},
		Action: iam.ActionExecute, CorrelationID: trusted.CorrelationID,
	})
	if err != nil || token == "" {
		response.Error(w, http.StatusServiceUnavailable, 50300, "operation delegation unavailable")
		return
	}
	data, status, err := h.forwardOperation(r, token, http.MethodPost, h.platformURL+"/v1/operations", body)
	if err != nil {
		response.Error(w, http.StatusServiceUnavailable, 50300, "platform API unavailable")
		return
	}
	if status < 200 || status >= 300 {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = w.Write(data)
		return
	}
	response.Success(w, json.RawMessage(data))
}

func applicationProviderStepType(action string) string {
	switch action {
	case "upgrade":
		return "workload.upgrade"
	case "rollback":
		return "workload.rollback"
	default:
		return action
	}
}

func validApplicationAction(action string) bool {
	switch action {
	case "deploy", "upgrade", "rollback", "delete":
		return true
	default:
		return false
	}
}

func applicationActionInputs(req applicationActionRequest, namespace, workload, pvc, resourceUID string) (map[string]string, error) {
	expectedUID := strings.TrimSpace(req.ExpectedUID)
	if expectedUID == "" {
		expectedUID = resourceUID
	}
	inputs := map[string]string{"namespace": namespace, "name": workload}
	switch req.Action {
	case "deploy":
		if strings.TrimSpace(req.Image) == "" {
			return nil, errors.New("deploy requires image")
		}
		inputs["image"] = req.Image
		inputs["replicas"] = "1"
		if req.Replicas != "" {
			inputs["replicas"] = req.Replicas
		}
		if pvc != "" {
			inputs["pvc_name"] = pvc
			inputs["pvc_mount_path"] = applicationDefault(req.PVCMountPath, "/usr/share/nginx/html")
		}
		inputs["secret_reference"] = applicationDefault(req.SecretName, "demo-secret")
		inputs["mount_reference"] = applicationDefault(req.SecretMountPath, "/etc/hnb-secret")
	case "upgrade", "rollback":
		if strings.TrimSpace(req.SourceImage) == "" || strings.TrimSpace(req.TargetImage) == "" || expectedUID == "" {
			return nil, errors.New(req.Action + " requires sourceImage, targetImage and expectedUid")
		}
		inputs["source_image"] = req.SourceImage
		inputs["target_image"] = req.TargetImage
		inputs["expected_uid"] = expectedUID
		inputs["backup_ref"] = applicationDefault(req.BackupRef, "application-action")
		if req.Action == "upgrade" {
			if req.DataCompatible == nil || !*req.DataCompatible {
				return nil, errors.New("upgrade requires dataCompatible=true")
			}
			inputs["data_compatible"] = "true"
		} else {
			if req.DataIrreversible != nil && *req.DataIrreversible {
				return nil, errors.New("irreversible data migration cannot be rolled back automatically")
			}
			inputs["data_irreversible"] = "false"
		}
	case "delete":
		if expectedUID == "" {
			return nil, errors.New("delete requires expectedUid")
		}
		inputs["expected_uid"] = expectedUID
	}
	return inputs, nil
}

func applicationDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

package handler

import (
	"encoding/json"
	"net/http"

	"github.com/F31/hnb/cmd/apiserver/internal/capability"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

type SessionHandler struct{ caps capability.Set }

func NewSessionHandler(platformURL string) *SessionHandler {
	_ = platformURL
	return &SessionHandler{caps: capability.AllStages()}
}

func NewSessionHandlerWithCapabilities(platformURL string, caps capability.Set) *SessionHandler {
	_ = platformURL
	return &SessionHandler{caps: caps}
}

func (h *SessionHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	memberships := make([]map[string]string, 0)
	seen := make(map[string]struct{})
	for _, permission := range trusted.ScopedPermissions {
		if permission.TenantID == "" {
			continue
		}
		if _, ok := seen[permission.TenantID]; ok {
			continue
		}
		seen[permission.TenantID] = struct{}{}
		memberships = append(memberships, map[string]string{"membershipId": trusted.MembershipID, "tenantId": permission.TenantID, "tenantName": permission.TenantID})
	}
	if len(memberships) == 0 && trusted.TenantID != "" {
		memberships = append(memberships, map[string]string{"membershipId": trusted.MembershipID, "tenantId": trusted.TenantID, "tenantName": trusted.TenantID})
	}
	subjectType := trusted.SubjectType
	if subjectType != "service" && subjectType != "workload" {
		subjectType = "user"
	}
	permissions := make([]map[string]string, 0, len(trusted.ScopedPermissions))
	for _, permission := range trusted.ScopedPermissions {
		permissions = append(permissions, map[string]string{"tenantId": permission.TenantID, "resourceKind": permission.ResourceKind, "resourceId": permission.ResourceID, "action": string(permission.Action)})
	}
	selectedTenant := trusted.TenantID
	if selectedTenant == "" && len(memberships) > 0 {
		selectedTenant = memberships[0]["tenantId"]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"subject":           map[string]string{"id": trusted.SubjectID, "type": subjectType, "displayName": trusted.SubjectID},
		"selectedTenantId":  selectedTenant,
		"memberships":       memberships,
		"capabilities":      consoleBootstrapCapabilities(h.caps),
		"permissions":       permissions,
		"policyVersion":     trusted.PolicyVersion,
		"permissionVersion": trusted.PolicyVersion,
	})
}

func consoleBootstrapCapabilities(caps capability.Set) []map[string]string {
	result := []map[string]string{
		{"id": "kubernetes_targets", "version": "v1"},
		{"id": "edge_targets", "version": "v1"},
		{"id": "helm_operations", "version": "v1"},
		{"id": "policy_enforcement", "version": "v1"},
		{"id": "runtime_intents", "version": "v1"},
	}
	for _, stage := range caps.EnabledStages() {
		result = append(result, map[string]string{"id": stage, "version": "v1"})
	}
	return result
}

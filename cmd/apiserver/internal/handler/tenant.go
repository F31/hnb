package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	tenantapp "github.com/F31/hnb/cmd/apiserver/internal/application/tenant"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
)

type TenantHandler struct{ service *tenantapp.Service }

func NewTenantHandler(db *sql.DB) *TenantHandler {
	return &TenantHandler{service: tenantapp.NewService(tenantapp.NewSQLRepository(db))}
}

func (h *TenantHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	tenantID := trusted.TenantID
	workspaces, err := h.service.ListWorkspaces(r.Context(), tenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, workspaces)
}

func (h *TenantHandler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	tenantID := trusted.TenantID
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Name == "" {
		response.BadRequest(w, "name is required")
		return
	}
	workspace, err := h.service.CreateWorkspace(r.Context(), tenantID, req.Name, req.DisplayName)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, workspace)
}

// ListTenantNamespaces supports tenants that do not use optional Workspaces.
// Workspace-scoped routes remain available for project/team organisation.
func (h *TenantHandler) ListTenantNamespaces(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	namespaces, err := h.service.ListNamespaces(r.Context(), trusted.TenantID, "", r.URL.Query().Get("cluster_id"))
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, namespaces)
}

func (h *TenantHandler) CreateTenantNamespace(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	var req struct {
		Name        string     `json:"name"`
		Description string     `json:"description,omitempty"`
		ClusterID   string     `json:"cluster_id"`
		WorkspaceID string     `json:"workspace_id,omitempty"`
		Quota       core.Quota `json:"quota,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Name == "" || req.ClusterID == "" {
		response.BadRequest(w, "name and cluster_id are required")
		return
	}
	if req.WorkspaceID != "" {
		workspace, err := h.service.GetWorkspace(r.Context(), req.WorkspaceID)
		if err != nil || workspace.TenantID != trusted.TenantID {
			response.NotFound(w, "workspace not found")
			return
		}
	}
	allocated, err := h.service.HasActiveTenantClusterAllocation(r.Context(), trusted.TenantID, req.ClusterID)
	if err != nil || !allocated {
		response.BadRequest(w, "tenant has no active allocation in this cluster")
		return
	}
	ns := core.Namespace{ID: req.Name, WorkspaceID: req.WorkspaceID, ClusterID: req.ClusterID, Name: req.Name, Description: req.Description, Quota: req.Quota, Status: "active"}
	fits, err := h.service.NamespaceQuotaFitsAllocation(r.Context(), trusted.TenantID, ns.ClusterID, ns.ID, ns.Quota)
	if err != nil || !fits {
		response.BadRequest(w, "namespace quota exceeds the tenant allocation in this cluster")
		return
	}
	if err := h.service.CreateNamespace(r.Context(), trusted.TenantID, ns); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, ns)
}

func (h *TenantHandler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	wsID := r.PathValue("workspace_id")
	clusterID := r.URL.Query().Get("cluster_id")
	namespaces, err := h.service.ListNamespaces(r.Context(), trusted.TenantID, wsID, clusterID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, namespaces)
}

func (h *TenantHandler) GetNamespace(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	nsID := r.PathValue("namespace_id")
	ns, err := h.service.GetNamespace(r.Context(), trusted.TenantID, nsID)
	if err != nil {
		response.NotFound(w, "namespace not found")
		return
	}
	response.Success(w, ns)
}

func (h *TenantHandler) CreateNamespace(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	wsID := r.PathValue("workspace_id")
	var req struct {
		Name        string     `json:"name"`
		Description string     `json:"description,omitempty"`
		ClusterID   string     `json:"cluster_id,omitempty"`
		Quota       core.Quota `json:"quota,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Name == "" {
		response.BadRequest(w, "name is required")
		return
	}
	workspace, err := h.service.GetWorkspace(r.Context(), wsID)
	if err != nil || workspace.TenantID != trusted.TenantID {
		response.NotFound(w, "workspace not found")
		return
	}
	ns := core.Namespace{
		ID:          req.Name,
		WorkspaceID: wsID,
		ClusterID:   req.ClusterID,
		Name:        req.Name,
		Description: req.Description,
		Quota:       req.Quota,
		Status:      "active",
	}
	if ns.ClusterID != "" {
		accessible, err := h.service.ClusterAccessibleTo(r.Context(), ns.ClusterID, trusted.TenantID, wsID)
		if err != nil || !accessible {
			response.BadRequest(w, "cluster is not accessible to this workspace")
			return
		}
		fits, err := h.service.NamespaceQuotaFitsAllocation(r.Context(), trusted.TenantID, ns.ClusterID, ns.ID, ns.Quota)
		if err != nil || !fits {
			response.BadRequest(w, "namespace quota exceeds the tenant allocation in this cluster")
			return
		}
	}
	if err := h.service.CreateNamespace(r.Context(), trusted.TenantID, ns); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, ns)
}

func (h *TenantHandler) UpdateNamespace(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	wsID := r.PathValue("workspace_id")
	nsID := r.PathValue("namespace_id")
	var req struct {
		Description string     `json:"description,omitempty"`
		ClusterID   string     `json:"cluster_id,omitempty"`
		Quota       core.Quota `json:"quota,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	ns := core.Namespace{
		ID:          nsID,
		WorkspaceID: wsID,
		ClusterID:   req.ClusterID,
		Description: req.Description,
		Quota:       req.Quota,
	}
	if ns.ClusterID != "" {
		accessible, err := h.service.ClusterAccessibleTo(r.Context(), ns.ClusterID, trusted.TenantID, wsID)
		if err != nil || !accessible {
			response.BadRequest(w, "cluster is not accessible to this workspace")
			return
		}
		fits, err := h.service.NamespaceQuotaFitsAllocation(r.Context(), trusted.TenantID, ns.ClusterID, ns.ID, ns.Quota)
		if err != nil || !fits {
			response.BadRequest(w, "namespace quota exceeds the tenant allocation in this cluster")
			return
		}
	}
	if err := h.service.UpdateNamespace(r.Context(), trusted.TenantID, ns); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, ns)
}

func (h *TenantHandler) DeleteNamespace(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	nsID := r.PathValue("namespace_id")
	if err := h.service.DeleteNamespace(r.Context(), trusted.TenantID, nsID); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "deleted"})
}

func (h *TenantHandler) ListNamespaceMembers(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	nsID := r.PathValue("namespace_id")
	members, err := h.service.ListNamespaceMembers(r.Context(), trusted.TenantID, nsID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, members)
}

func (h *TenantHandler) AddNamespaceMember(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	nsID := r.PathValue("namespace_id")
	var req struct {
		SubjectID string `json:"subject_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.SubjectID == "" {
		response.BadRequest(w, "subject_id is required")
		return
	}
	if err := h.service.AddNamespaceMember(r.Context(), trusted.TenantID, nsID, req.SubjectID); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, map[string]string{"status": "added"})
}

func (h *TenantHandler) RemoveNamespaceMember(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	nsID := r.PathValue("namespace_id")
	subjectID := r.PathValue("subject_id")
	if err := h.service.RemoveNamespaceMember(r.Context(), trusted.TenantID, nsID, subjectID); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "removed"})
}

func (h *TenantHandler) ListTenantUsers(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	users, err := h.service.ListTenantUsers(r.Context(), trusted.TenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, users)
}

func (h *TenantHandler) GetNamespaceQuotaRemaining(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	wsID := r.PathValue("workspace_id")
	remaining, err := h.service.NamespaceQuotaRemaining(r.Context(), trusted.TenantID, wsID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, remaining)
}

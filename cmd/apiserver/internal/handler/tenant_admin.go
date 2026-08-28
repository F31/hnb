package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	tenantapp "github.com/F31/hnb/cmd/apiserver/internal/application/tenant"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type TenantAdminHandler struct {
	service    *tenantapp.Service
	harborURL  string
	harborUser string
	harborPass string
	httpClient *http.Client
}

func NewTenantAdminHandler(db *sql.DB, harborURL, harborUser, harborPass string) *TenantAdminHandler {
	return &TenantAdminHandler{
		service:    tenantapp.NewService(tenantapp.NewSQLRepository(db)),
		harborURL:  harborURL,
		harborUser: harborUser,
		harborPass: harborPass,
		httpClient: &http.Client{},
	}
}

func (h *TenantAdminHandler) harborRequest(method, path string, body []byte) (*http.Response, error) {
	url := h.harborURL + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("harbor request: %w", err)
	}
	req.SetBasicAuth(h.harborUser, h.harborPass)
	return h.httpClient.Do(req)
}

func (h *TenantAdminHandler) ensureHarborProject(name string) error {
	resp, err := h.harborRequest("HEAD", "/api/v2.0/projects/"+name, nil)
	if err != nil {
		return fmt.Errorf("harbor check project: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"project_name": name, "public": false, "storage_limit": -1})
	resp, err = h.harborRequest("POST", "/api/v2.0/projects", body)
	if err != nil {
		return fmt.Errorf("harbor create project: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("harbor create project: status %d body=%s", resp.StatusCode, string(raw))
	}
	return nil
}

func (h *TenantAdminHandler) deleteHarborProject(name string) error {
	resp, err := h.harborRequest("DELETE", "/api/v2.0/projects/"+name, nil)
	if err != nil {
		return fmt.Errorf("harbor delete project: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("harbor delete project: status %d", resp.StatusCode)
	}
	return nil
}

func (h *TenantAdminHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	tenants, err := h.service.ListTenants(r.Context())
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	total := len(tenants)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	response.Success(w, map[string]any{"items": tenants[start:end], "total": total})
}

func (h *TenantAdminHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string     `json:"name"`
		DisplayName string     `json:"display_name,omitempty"`
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
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}
	tenant, err := h.service.CreateTenant(r.Context(), req.Name, displayName, req.Quota)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if h.harborURL != "" {
		if err := h.ensureHarborProject(req.Name); err != nil {
			fmt.Printf("[harbor-sync] create project for tenant %s: %v\n", req.Name, err)
		}
	}
	response.Created(w, tenant)
}

func (h *TenantAdminHandler) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tenant, err := h.service.GetTenant(r.Context(), id)
	if err != nil {
		response.NotFound(w, "tenant not found")
		return
	}
	response.Success(w, tenant)
}

func (h *TenantAdminHandler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		DisplayName string `json:"display_name,omitempty"`
		Status      string `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Status != "" && req.Status != "active" && req.Status != "suspended" && req.Status != "deleted" {
		response.BadRequest(w, "status must be active, suspended, or deleted")
		return
	}
	tenant, err := h.service.UpdateTenant(r.Context(), id, req.DisplayName, req.Status)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if h.harborURL != "" && req.Status == "deleted" {
		if err := h.deleteHarborProject(tenant.Name); err != nil {
			fmt.Printf("[harbor-sync] delete project for tenant %s: %v\n", tenant.Name, err)
		}
	}
	response.Success(w, tenant)
}

func (h *TenantAdminHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tenant, err := h.service.GetTenant(r.Context(), id)
	if err != nil {
		response.NotFound(w, "tenant not found")
		return
	}
	if err := h.service.DeleteTenant(r.Context(), id); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if h.harborURL != "" {
		if err := h.deleteHarborProject(tenant.Name); err != nil {
			fmt.Printf("[harbor-sync] delete project for tenant %s: %v\n", tenant.Name, err)
		}
	}
	response.Success(w, map[string]string{"status": "deleted"})
}

func (h *TenantAdminHandler) ListTenantWorkspaces(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	workspaces, err := h.service.ListWorkspaces(r.Context(), tenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, workspaces)
}

func (h *TenantAdminHandler) GetTenantQuota(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	quota, err := h.service.GetTenantQuota(r.Context(), id)
	if err != nil {
		response.NotFound(w, "tenant not found")
		return
	}
	response.Success(w, quota)
}

func (h *TenantAdminHandler) UpdateTenantQuota(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req core.Quota
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if err := h.service.UpdateTenantQuota(r.Context(), id, req); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, req)
}

func (h *TenantAdminHandler) ListTenantClusterAllocations(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	allocations, err := h.service.ListTenantClusterAllocations(r.Context(), tenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	total := core.Quota{}
	for _, allocation := range allocations {
		if allocation.Status != "active" {
			continue
		}
		total.CPU += allocation.Quota.CPU
		total.Memory += allocation.Quota.Memory
		total.Storage += allocation.Quota.Storage
		total.VGPU += allocation.Quota.VGPU
		total.VRAM += allocation.Quota.VRAM
		total.GPU += allocation.Quota.GPU
	}
	response.Success(w, map[string]any{"items": allocations, "total_quota": total})
}

func (h *TenantAdminHandler) UpsertTenantClusterAllocation(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	clusterID := r.PathValue("cluster_id")
	var req struct {
		Quota            core.Quota `json:"quota"`
		Status           string     `json:"status,omitempty"`
		NamespacePrefix  string     `json:"namespace_prefix"`
		IsolationEnabled bool       `json:"isolation_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.NamespacePrefix == "" {
		response.BadRequest(w, "namespace_prefix is required")
		return
	}
	allocation := core.TenantClusterAllocation{
		TenantID: tenantID, ClusterID: clusterID, Quota: req.Quota, Status: req.Status,
		NamespacePrefix: req.NamespacePrefix, IsolationEnabled: req.IsolationEnabled,
	}
	if err := h.service.UpsertTenantClusterAllocation(r.Context(), allocation); err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	response.Success(w, allocation)
}

func (h *TenantAdminHandler) DeleteTenantClusterAllocation(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteTenantClusterAllocation(r.Context(), r.PathValue("id"), r.PathValue("cluster_id")); err != nil {
		if err == sql.ErrNoRows {
			response.NotFound(w, "tenant cluster allocation not found or still has namespaces")
			return
		}
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "deleted"})
}

func (h *TenantAdminHandler) GetWorkspaceQuota(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace_id")
	quota, err := h.service.GetWorkspaceQuota(r.Context(), workspaceID)
	if err != nil {
		response.NotFound(w, "workspace not found")
		return
	}
	response.Success(w, quota)
}

func (h *TenantAdminHandler) UpdateWorkspaceQuota(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace_id")
	var req core.Quota
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if err := h.service.UpdateWorkspaceQuota(r.Context(), workspaceID, req); err != nil {
		response.Error(w, http.StatusBadRequest, 40000, err.Error())
		return
	}
	response.Success(w, req)
}

func (h *TenantAdminHandler) BindWorkspaceCluster(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace_id")
	var req struct {
		ClusterID string `json:"cluster_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.ClusterID == "" {
		response.BadRequest(w, "cluster_id is required")
		return
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	share := core.ClusterShare{
		ID:                     uuid.NewString(),
		ClusterID:              req.ClusterID,
		GranteeTenantID:        trusted.TenantID,
		GranteeWorkspaceID:     workspaceID,
		Permissions:            []string{"read", "deploy"},
		K8sNamespacePrefix:     fmt.Sprintf("t-%s-w-%s", trusted.TenantID[:8], workspaceID[:8]),
		TenantIsolationEnabled: true,
		CreatedBySubjectID:     trusted.SubjectID,
	}
	if err := h.service.CreateClusterShare(r.Context(), share); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, share)
}

func (h *TenantAdminHandler) UnbindWorkspaceCluster(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace_id")
	clusterID := r.PathValue("cluster_id")
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	shareID, err := h.service.FindClusterShare(r.Context(), clusterID, trusted.TenantID, workspaceID)
	if err != nil || shareID == "" {
		response.NotFound(w, "share not found")
		return
	}
	if err := h.service.DeleteClusterShare(r.Context(), shareID); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "unbound"})
}

func (h *TenantAdminHandler) ListWorkspaceClusters(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace_id")
	clusters, err := h.service.ListWorkspaceClusters(r.Context(), workspaceID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, clusters)
}

func (h *TenantAdminHandler) CreateTenantWorkspace(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
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

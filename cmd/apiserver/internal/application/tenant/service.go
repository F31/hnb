package tenant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/google/uuid"
)

type TenantRecord struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	Quota       core.Quota `json:"quota,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Repository interface {
	ListTenants(ctx context.Context) ([]TenantRecord, error)
	CountTenants(ctx context.Context) (int, error)
	CreateTenant(ctx context.Context, tenant TenantRecord) error
	GetTenant(ctx context.Context, id string) (*TenantRecord, error)
	UpdateTenant(ctx context.Context, tenant TenantRecord) error
	DeleteTenant(ctx context.Context, id string) error
	GetTenantQuota(ctx context.Context, id string) (*core.Quota, error)
	UpdateTenantQuota(ctx context.Context, id string, quota core.Quota) error
	ListTenantClusterAllocations(ctx context.Context, tenantID string) ([]core.TenantClusterAllocation, error)
	UpsertTenantClusterAllocation(ctx context.Context, allocation core.TenantClusterAllocation) error
	DeleteTenantClusterAllocation(ctx context.Context, tenantID, clusterID string) error
	HasActiveTenantClusterAllocation(ctx context.Context, tenantID, clusterID string) (bool, error)
	NamespaceQuotaFitsAllocation(ctx context.Context, tenantID, clusterID, namespaceID string, quota core.Quota) (bool, error)
	ListWorkspaces(ctx context.Context, tenantID string) ([]core.Workspace, error)
	GetWorkspace(ctx context.Context, workspaceID string) (*core.Workspace, error)
	CreateWorkspace(ctx context.Context, workspace core.Workspace) error
	GetWorkspaceQuota(ctx context.Context, workspaceID string) (*core.Quota, error)
	UpdateWorkspaceQuota(ctx context.Context, workspaceID string, quota core.Quota) error
	BindWorkspaceCluster(ctx context.Context, workspaceID, clusterID string) error
	UnbindWorkspaceCluster(ctx context.Context, workspaceID, clusterID string) error
	GetClusterTenant(ctx context.Context, clusterID string) (string, error)
	ListWorkspaceClusters(ctx context.Context, workspaceID string) ([]core.RuntimeTarget, error)
	ListNamespaces(ctx context.Context, tenantID, workspaceID, clusterID string) ([]core.Namespace, error)
	GetNamespace(ctx context.Context, tenantID, namespaceID string) (*core.Namespace, error)
	CreateNamespace(ctx context.Context, tenantID string, ns core.Namespace) error
	UpdateNamespace(ctx context.Context, tenantID string, ns core.Namespace) error
	DeleteNamespace(ctx context.Context, tenantID, namespaceID string) error
	CreateClusterShare(ctx context.Context, share core.ClusterShare) error
	DeleteClusterShare(ctx context.Context, shareID string) error
	ListClusterShares(ctx context.Context, clusterID string) ([]core.ClusterShare, error)
	ClusterAccessibleTo(ctx context.Context, clusterID, tenantID, workspaceID string) (bool, error)
	ListClustersForWorkspace(ctx context.Context, workspaceID string) ([]core.RuntimeTarget, error)
	FindClusterShare(ctx context.Context, clusterID, granteeTenantID, granteeWorkspaceID string) (string, error)
	ListNamespaceMembers(ctx context.Context, tenantID, namespaceID string) ([]NamespaceMember, error)
	AddNamespaceMember(ctx context.Context, tenantID, namespaceID, subjectID, roleID string) error
	RemoveNamespaceMember(ctx context.Context, tenantID, namespaceID, subjectID string) error
	EnsureNamespaceRole(ctx context.Context, tenantID string) (string, error)
	ListTenantUsers(ctx context.Context, tenantID string) ([]TenantUser, error)
}

type NamespaceMember struct {
	BindingID   string    `json:"binding_id"`
	SubjectID   string    `json:"subject_id"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email,omitempty"`
	RoleID      string    `json:"role_id"`
	RoleName    string    `json:"role_name"`
	GrantedAt   time.Time `json:"granted_at"`
}

type TenantUser struct {
	SubjectID   string `json:"subject_id"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) ListTenants(ctx context.Context) ([]TenantRecord, error) {
	return s.repo.ListTenants(ctx)
}

func (s *Service) CountTenants(ctx context.Context) (int, error) {
	return s.repo.CountTenants(ctx)
}

func (s *Service) CreateTenant(ctx context.Context, name, displayName string, quota core.Quota) (TenantRecord, error) {
	now := time.Now().UTC()
	tenant := TenantRecord{ID: uuid.NewString(), Name: name, DisplayName: displayName, Status: "active", Quota: quota, CreatedAt: now, UpdatedAt: now}
	return tenant, s.repo.CreateTenant(ctx, tenant)
}

func (s *Service) GetTenant(ctx context.Context, id string) (*TenantRecord, error) {
	return s.repo.GetTenant(ctx, id)
}

func (s *Service) UpdateTenant(ctx context.Context, id, displayName, status string) (*TenantRecord, error) {
	tenant, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}
	if displayName != "" {
		tenant.DisplayName = displayName
	}
	if status != "" {
		tenant.Status = status
	}
	tenant.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTenant(ctx, *tenant); err != nil {
		return nil, err
	}
	return tenant, nil
}

func (s *Service) DeleteTenant(ctx context.Context, id string) error {
	return s.repo.DeleteTenant(ctx, id)
}

func (s *Service) ListWorkspaces(ctx context.Context, tenantID string) ([]core.Workspace, error) {
	return s.repo.ListWorkspaces(ctx, tenantID)
}

func (s *Service) GetWorkspace(ctx context.Context, workspaceID string) (*core.Workspace, error) {
	return s.repo.GetWorkspace(ctx, workspaceID)
}

func (s *Service) CreateWorkspace(ctx context.Context, tenantID, name, displayName string) (core.Workspace, error) {
	now := time.Now().UTC()
	workspace := core.Workspace{ID: uuid.NewString(), Name: name, DisplayName: displayName, TenantID: tenantID, IsActive: true, CreatedAt: now, UpdatedAt: now}
	return workspace, s.repo.CreateWorkspace(ctx, workspace)
}

func (s *Service) GetTenantQuota(ctx context.Context, id string) (*core.Quota, error) {
	return s.repo.GetTenantQuota(ctx, id)
}

func (s *Service) UpdateTenantQuota(ctx context.Context, id string, quota core.Quota) error {
	return s.repo.UpdateTenantQuota(ctx, id, quota)
}

func (s *Service) ListTenantClusterAllocations(ctx context.Context, tenantID string) ([]core.TenantClusterAllocation, error) {
	return s.repo.ListTenantClusterAllocations(ctx, tenantID)
}

func (s *Service) UpsertTenantClusterAllocation(ctx context.Context, allocation core.TenantClusterAllocation) error {
	if allocation.TenantID == "" || allocation.ClusterID == "" {
		return fmt.Errorf("tenant_id and cluster_id are required")
	}
	if allocation.Status == "" {
		allocation.Status = "active"
	}
	if allocation.Status != "active" && allocation.Status != "suspended" {
		return fmt.Errorf("allocation status must be active or suspended")
	}
	if allocation.Quota.CPU < 0 || allocation.Quota.Memory < 0 || allocation.Quota.Storage < 0 || allocation.Quota.VGPU < 0 || allocation.Quota.VRAM < 0 || allocation.Quota.GPU < 0 {
		return fmt.Errorf("allocation quota cannot be negative")
	}
	if allocation.NamespacePrefix == "" {
		return fmt.Errorf("namespace_prefix is required")
	}
	return s.repo.UpsertTenantClusterAllocation(ctx, allocation)
}

func (s *Service) DeleteTenantClusterAllocation(ctx context.Context, tenantID, clusterID string) error {
	return s.repo.DeleteTenantClusterAllocation(ctx, tenantID, clusterID)
}

func (s *Service) HasActiveTenantClusterAllocation(ctx context.Context, tenantID, clusterID string) (bool, error) {
	return s.repo.HasActiveTenantClusterAllocation(ctx, tenantID, clusterID)
}

func (s *Service) NamespaceQuotaFitsAllocation(ctx context.Context, tenantID, clusterID, namespaceID string, quota core.Quota) (bool, error) {
	return s.repo.NamespaceQuotaFitsAllocation(ctx, tenantID, clusterID, namespaceID, quota)
}

func (s *Service) GetWorkspaceQuota(ctx context.Context, workspaceID string) (*core.Quota, error) {
	return s.repo.GetWorkspaceQuota(ctx, workspaceID)
}

func (s *Service) UpdateWorkspaceQuota(ctx context.Context, workspaceID string, quota core.Quota) error {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	tenantQuota, err := s.repo.GetTenantQuota(ctx, ws.TenantID)
	if err != nil {
		return err
	}
	allWS, err := s.repo.ListWorkspaces(ctx, ws.TenantID)
	if err != nil {
		return err
	}
	var sumCPU, sumMem, sumStor, sumVGPU, sumVRAM, sumGPU int64
	for _, w := range allWS {
		if w.ID == workspaceID {
			continue
		}
		if w.Quota != (core.Quota{}) {
			sumCPU += w.Quota.CPU
			sumMem += w.Quota.Memory
			sumStor += w.Quota.Storage
			sumVGPU += w.Quota.VGPU
			sumVRAM += w.Quota.VRAM
			sumGPU += w.Quota.GPU
		}
	}
	sumCPU += quota.CPU
	sumMem += quota.Memory
	sumStor += quota.Storage
	sumVGPU += quota.VGPU
	sumVRAM += quota.VRAM
	sumGPU += quota.GPU
	if tenantQuota != nil {
		if tenantQuota.CPU > 0 && sumCPU > tenantQuota.CPU {
			return fmt.Errorf("workspace quota cpu %d exceeds tenant quota %d", sumCPU, tenantQuota.CPU)
		}
		if tenantQuota.Memory > 0 && sumMem > tenantQuota.Memory {
			return fmt.Errorf("workspace quota memory %d exceeds tenant quota %d", sumMem, tenantQuota.Memory)
		}
		if tenantQuota.Storage > 0 && sumStor > tenantQuota.Storage {
			return fmt.Errorf("workspace quota storage %d exceeds tenant quota %d", sumStor, tenantQuota.Storage)
		}
		if tenantQuota.VGPU > 0 && sumVGPU > tenantQuota.VGPU {
			return fmt.Errorf("workspace quota vgpu %d exceeds tenant quota %d", sumVGPU, tenantQuota.VGPU)
		}
		if tenantQuota.VRAM > 0 && sumVRAM > tenantQuota.VRAM {
			return fmt.Errorf("workspace quota vram %d exceeds tenant quota %d", sumVRAM, tenantQuota.VRAM)
		}
		if tenantQuota.GPU > 0 && sumGPU > tenantQuota.GPU {
			return fmt.Errorf("workspace quota gpu %d exceeds tenant quota %d", sumGPU, tenantQuota.GPU)
		}
	}
	return s.repo.UpdateWorkspaceQuota(ctx, workspaceID, quota)
}

func (s *Service) BindWorkspaceCluster(ctx context.Context, workspaceID, clusterID string) error {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	clusterTenant, err := s.repo.GetClusterTenant(ctx, clusterID)
	if err != nil {
		return err
	}
	if clusterTenant != ws.TenantID {
		return fmt.Errorf("cluster %s does not belong to the workspace tenant", clusterID)
	}
	return s.repo.BindWorkspaceCluster(ctx, workspaceID, clusterID)
}

func (s *Service) UnbindWorkspaceCluster(ctx context.Context, workspaceID, clusterID string) error {
	return s.repo.UnbindWorkspaceCluster(ctx, workspaceID, clusterID)
}

func (s *Service) ListWorkspaceClusters(ctx context.Context, workspaceID string) ([]core.RuntimeTarget, error) {
	return s.repo.ListWorkspaceClusters(ctx, workspaceID)
}

func (s *Service) ListNamespaces(ctx context.Context, tenantID, workspaceID, clusterID string) ([]core.Namespace, error) {
	return s.repo.ListNamespaces(ctx, tenantID, workspaceID, clusterID)
}

func (s *Service) GetNamespace(ctx context.Context, tenantID, namespaceID string) (*core.Namespace, error) {
	return s.repo.GetNamespace(ctx, tenantID, namespaceID)
}

func (s *Service) CreateNamespace(ctx context.Context, tenantID string, ns core.Namespace) error {
	return s.repo.CreateNamespace(ctx, tenantID, ns)
}

func (s *Service) UpdateNamespace(ctx context.Context, tenantID string, ns core.Namespace) error {
	return s.repo.UpdateNamespace(ctx, tenantID, ns)
}

func (s *Service) DeleteNamespace(ctx context.Context, tenantID, namespaceID string) error {
	return s.repo.DeleteNamespace(ctx, tenantID, namespaceID)
}

func (s *Service) NamespaceQuotaRemaining(ctx context.Context, tenantID, workspaceID string) (*core.Quota, error) {
	workspace, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	namespaces, err := s.repo.ListNamespaces(ctx, tenantID, workspaceID, "")
	if err != nil {
		return nil, err
	}
	var sumCPU, sumMem, sumStor, sumVGPU, sumVRAM, sumGPU int64
	for _, ns := range namespaces {
		sumCPU += ns.Quota.CPU
		sumMem += ns.Quota.Memory
		sumStor += ns.Quota.Storage
		sumVGPU += ns.Quota.VGPU
		sumVRAM += ns.Quota.VRAM
		sumGPU += ns.Quota.GPU
	}
	remaining := core.Quota{
		CPU:     workspace.Quota.CPU - sumCPU,
		Memory:  workspace.Quota.Memory - sumMem,
		Storage: workspace.Quota.Storage - sumStor,
		VGPU:    workspace.Quota.VGPU - sumVGPU,
		VRAM:    workspace.Quota.VRAM - sumVRAM,
		GPU:     workspace.Quota.GPU - sumGPU,
	}
	if remaining.CPU < 0 {
		remaining.CPU = 0
	}
	if remaining.Memory < 0 {
		remaining.Memory = 0
	}
	if remaining.Storage < 0 {
		remaining.Storage = 0
	}
	if remaining.VGPU < 0 {
		remaining.VGPU = 0
	}
	if remaining.VRAM < 0 {
		remaining.VRAM = 0
	}
	if remaining.GPU < 0 {
		remaining.GPU = 0
	}
	return &remaining, nil
}

func (s *Service) GetClusterTenant(ctx context.Context, clusterID string) (string, error) {
	return s.repo.GetClusterTenant(ctx, clusterID)
}

func (s *Service) CreateClusterShare(ctx context.Context, share core.ClusterShare) error {
	return s.repo.CreateClusterShare(ctx, share)
}

func (s *Service) DeleteClusterShare(ctx context.Context, shareID string) error {
	return s.repo.DeleteClusterShare(ctx, shareID)
}

func (s *Service) ListClusterShares(ctx context.Context, clusterID string) ([]core.ClusterShare, error) {
	return s.repo.ListClusterShares(ctx, clusterID)
}

func (s *Service) ClusterAccessibleTo(ctx context.Context, clusterID, tenantID, workspaceID string) (bool, error) {
	return s.repo.ClusterAccessibleTo(ctx, clusterID, tenantID, workspaceID)
}

func (s *Service) ListClustersForWorkspace(ctx context.Context, workspaceID string) ([]core.RuntimeTarget, error) {
	return s.repo.ListClustersForWorkspace(ctx, workspaceID)
}

func (s *Service) FindClusterShare(ctx context.Context, clusterID, granteeTenantID, granteeWorkspaceID string) (string, error) {
	return s.repo.FindClusterShare(ctx, clusterID, granteeTenantID, granteeWorkspaceID)
}

func (s *Service) ListNamespaceMembers(ctx context.Context, tenantID, namespaceID string) ([]NamespaceMember, error) {
	return s.repo.ListNamespaceMembers(ctx, tenantID, namespaceID)
}

func (s *Service) AddNamespaceMember(ctx context.Context, tenantID, namespaceID, subjectID string) error {
	roleID, err := s.repo.EnsureNamespaceRole(ctx, tenantID)
	if err != nil {
		return err
	}
	return s.repo.AddNamespaceMember(ctx, tenantID, namespaceID, subjectID, roleID)
}

func (s *Service) RemoveNamespaceMember(ctx context.Context, tenantID, namespaceID, subjectID string) error {
	return s.repo.RemoveNamespaceMember(ctx, tenantID, namespaceID, subjectID)
}

func (s *Service) ListTenantUsers(ctx context.Context, tenantID string) ([]TenantUser, error) {
	return s.repo.ListTenantUsers(ctx, tenantID)
}

type SQLRepository struct{ db *sql.DB }

func NewSQLRepository(db *sql.DB) *SQLRepository { return &SQLRepository{db: db} }

func (r *SQLRepository) CountTenants(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&count)
	return count, err
}

func (r *SQLRepository) ListTenants(ctx context.Context) ([]TenantRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, display_name, status, quota, created_at, updated_at FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tenants := make([]TenantRecord, 0)
	for rows.Next() {
		var t TenantRecord
		var quotaJSON []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.DisplayName, &t.Status, &quotaJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if len(quotaJSON) > 0 {
			_ = json.Unmarshal(quotaJSON, &t.Quota)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (r *SQLRepository) CreateTenant(ctx context.Context, tenant TenantRecord) error {
	quotaJSON, err := json.Marshal(tenant.Quota)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO tenants (id, name, display_name, status, quota, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		tenant.ID, tenant.Name, tenant.DisplayName, tenant.Status, quotaJSON, tenant.CreatedAt, tenant.UpdatedAt)
	return err
}

func (r *SQLRepository) GetTenant(ctx context.Context, id string) (*TenantRecord, error) {
	var t TenantRecord
	var quotaJSON []byte
	err := r.db.QueryRowContext(ctx, `SELECT id, name, display_name, status, quota, created_at, updated_at FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.DisplayName, &t.Status, &quotaJSON, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(quotaJSON) > 0 {
		_ = json.Unmarshal(quotaJSON, &t.Quota)
	}
	return &t, nil
}

func (r *SQLRepository) UpdateTenant(ctx context.Context, tenant TenantRecord) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tenants SET display_name=$2, status=$3, updated_at=$4 WHERE id=$1`,
		tenant.ID, tenant.DisplayName, tenant.Status, tenant.UpdatedAt)
	return err
}

func (r *SQLRepository) DeleteTenant(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	return err
}

func (r *SQLRepository) GetWorkspace(ctx context.Context, workspaceID string) (*core.Workspace, error) {
	var ws core.Workspace
	var labelsJSON, quotaJSON []byte
	err := r.db.QueryRowContext(ctx, `SELECT id, name, display_name, tenant_id, labels, quota, is_active, created_at, updated_at FROM workspaces WHERE id = $1`, workspaceID).
		Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.TenantID, &labelsJSON, &quotaJSON, &ws.IsActive, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(labelsJSON) > 0 {
		_ = json.Unmarshal(labelsJSON, &ws.Labels)
	}
	if len(quotaJSON) > 0 {
		_ = json.Unmarshal(quotaJSON, &ws.Quota)
	}
	return &ws, nil
}

func (r *SQLRepository) ListWorkspaces(ctx context.Context, tenantID string) ([]core.Workspace, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, display_name, tenant_id, labels, quota, is_active, created_at, updated_at FROM workspaces WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := make([]core.Workspace, 0)
	for rows.Next() {
		var workspace core.Workspace
		var labelsJSON, quotaJSON []byte
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.DisplayName, &workspace.TenantID, &labelsJSON, &quotaJSON, &workspace.IsActive, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
			return nil, err
		}
		if len(labelsJSON) > 0 {
			_ = json.Unmarshal(labelsJSON, &workspace.Labels)
		}
		if len(quotaJSON) > 0 {
			_ = json.Unmarshal(quotaJSON, &workspace.Quota)
		}
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

func (r *SQLRepository) CreateWorkspace(ctx context.Context, workspace core.Workspace) error {
	quotaJSON, err := json.Marshal(workspace.Quota)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO workspaces (id, name, display_name, tenant_id, is_active, quota, created_at, updated_at) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8)`,
		workspace.ID, workspace.Name, workspace.DisplayName, workspace.TenantID, workspace.IsActive, quotaJSON, workspace.CreatedAt, workspace.UpdatedAt)
	return err
}

func (r *SQLRepository) GetTenantQuota(ctx context.Context, id string) (*core.Quota, error) {
	var quotaJSON []byte
	err := r.db.QueryRowContext(ctx, `SELECT quota FROM tenants WHERE id = $1`, id).Scan(&quotaJSON)
	if err != nil {
		return nil, err
	}
	var q core.Quota
	if len(quotaJSON) > 0 {
		_ = json.Unmarshal(quotaJSON, &q)
	}
	return &q, nil
}

func (r *SQLRepository) UpdateTenantQuota(ctx context.Context, id string, quota core.Quota) error {
	quotaJSON, err := json.Marshal(quota)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE tenants SET quota = $2, updated_at = NOW() WHERE id = $1`, id, quotaJSON)
	return err
}

func (r *SQLRepository) ListTenantClusterAllocations(ctx context.Context, tenantID string) ([]core.TenantClusterAllocation, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, tenant_id, cluster_id, quota, status, namespace_prefix, isolation_enabled, created_at, updated_at
		FROM tenant_cluster_allocations WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allocations := make([]core.TenantClusterAllocation, 0)
	for rows.Next() {
		var allocation core.TenantClusterAllocation
		var quotaJSON []byte
		if err := rows.Scan(&allocation.ID, &allocation.TenantID, &allocation.ClusterID, &quotaJSON, &allocation.Status, &allocation.NamespacePrefix, &allocation.IsolationEnabled, &allocation.CreatedAt, &allocation.UpdatedAt); err != nil {
			return nil, err
		}
		if len(quotaJSON) > 0 {
			if err := json.Unmarshal(quotaJSON, &allocation.Quota); err != nil {
				return nil, err
			}
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}

func (r *SQLRepository) UpsertTenantClusterAllocation(ctx context.Context, allocation core.TenantClusterAllocation) error {
	quotaJSON, err := json.Marshal(allocation.Quota)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO tenant_cluster_allocations
		(id, tenant_id, cluster_id, quota, status, namespace_prefix, isolation_enabled)
		VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, cluster_id) DO UPDATE SET quota=EXCLUDED.quota, status=EXCLUDED.status,
		namespace_prefix=EXCLUDED.namespace_prefix, isolation_enabled=EXCLUDED.isolation_enabled, updated_at=now()`,
		allocation.ID, allocation.TenantID, allocation.ClusterID, quotaJSON, allocation.Status, allocation.NamespacePrefix, allocation.IsolationEnabled)
	return err
}

func (r *SQLRepository) DeleteTenantClusterAllocation(ctx context.Context, tenantID, clusterID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tenant_cluster_allocations a
		WHERE a.tenant_id=$1 AND a.cluster_id=$2
		AND NOT EXISTS (SELECT 1 FROM namespaces n WHERE n.tenant_id=a.tenant_id AND n.cluster_id=a.cluster_id)`, tenantID, clusterID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SQLRepository) HasActiveTenantClusterAllocation(ctx context.Context, tenantID, clusterID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_cluster_allocations WHERE tenant_id=$1 AND cluster_id=$2 AND status='active')`, tenantID, clusterID).Scan(&exists)
	return exists, err
}

func (r *SQLRepository) NamespaceQuotaFitsAllocation(ctx context.Context, tenantID, clusterID, namespaceID string, quota core.Quota) (bool, error) {
	var allocationJSON []byte
	if err := r.db.QueryRowContext(ctx, `SELECT quota FROM tenant_cluster_allocations WHERE tenant_id=$1 AND cluster_id=$2 AND status='active'`, tenantID, clusterID).Scan(&allocationJSON); err != nil {
		return false, err
	}
	var allocation, used core.Quota
	if err := json.Unmarshal(allocationJSON, &allocation); err != nil {
		return false, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT quota FROM namespaces WHERE tenant_id=$1 AND cluster_id=$2 AND id<>$3`, tenantID, clusterID, namespaceID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		var q core.Quota
		if len(raw) > 0 && json.Unmarshal(raw, &q) == nil {
			used.CPU += q.CPU
			used.Memory += q.Memory
			used.Storage += q.Storage
			used.VGPU += q.VGPU
			used.VRAM += q.VRAM
			used.GPU += q.GPU
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	used.CPU += quota.CPU
	used.Memory += quota.Memory
	used.Storage += quota.Storage
	used.VGPU += quota.VGPU
	used.VRAM += quota.VRAM
	used.GPU += quota.GPU
	within := func(limit, value int64) bool { return limit <= 0 || value <= limit }
	return within(allocation.CPU, used.CPU) && within(allocation.Memory, used.Memory) &&
		within(allocation.Storage, used.Storage) && within(allocation.VGPU, used.VGPU) &&
		within(allocation.VRAM, used.VRAM) && within(allocation.GPU, used.GPU), nil
}

func (r *SQLRepository) GetWorkspaceQuota(ctx context.Context, workspaceID string) (*core.Quota, error) {
	var quotaJSON []byte
	err := r.db.QueryRowContext(ctx, `SELECT quota FROM workspaces WHERE id = $1`, workspaceID).Scan(&quotaJSON)
	if err != nil {
		return nil, err
	}
	var q core.Quota
	if len(quotaJSON) > 0 {
		_ = json.Unmarshal(quotaJSON, &q)
	}
	return &q, nil
}

func (r *SQLRepository) UpdateWorkspaceQuota(ctx context.Context, workspaceID string, quota core.Quota) error {
	quotaJSON, err := json.Marshal(quota)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE workspaces SET quota = $2, updated_at = NOW() WHERE id = $1`, workspaceID, quotaJSON)
	return err
}

func (r *SQLRepository) BindWorkspaceCluster(ctx context.Context, workspaceID, clusterID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE runtime_targets SET workspace_id = $1 WHERE id = $2`, workspaceID, clusterID)
	return err
}

func (r *SQLRepository) UnbindWorkspaceCluster(ctx context.Context, workspaceID, clusterID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE runtime_targets SET workspace_id = NULL WHERE id = $1 AND workspace_id = $2`, clusterID, workspaceID)
	return err
}

func (r *SQLRepository) ListWorkspaceClusters(ctx context.Context, workspaceID string) ([]core.RuntimeTarget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, tenant_id, name, display_name, target_type, distribution, connection_type, status, labels, is_active, created_at FROM runtime_targets WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []core.RuntimeTarget
	for rows.Next() {
		var rt core.RuntimeTarget
		var dn, lb sql.NullString
		if err := rows.Scan(&rt.ID, &rt.TenantID, &rt.Name, &dn, &rt.TargetType, &rt.Distribution, &rt.ConnectionType, &rt.Status, &lb, &rt.IsActive, &rt.CreatedAt); err != nil {
			return nil, err
		}
		if dn.Valid {
			rt.DisplayName = dn.String
		}
		if lb.Valid {
			json.Unmarshal([]byte(lb.String), &rt.Labels)
		}
		targets = append(targets, rt)
	}
	if targets == nil {
		targets = []core.RuntimeTarget{}
	}
	return targets, rows.Err()
}

func (r *SQLRepository) ListNamespaces(ctx context.Context, tenantID, workspaceID, clusterID string) ([]core.Namespace, error) {
	query := `SELECT id, workspace_id, cluster_id, name, description, labels, quota, status, created_at, updated_at FROM namespaces WHERE tenant_id=$1`
	args := []any{tenantID}
	if workspaceID != "" {
		query += ` AND workspace_id=$2`
		args = append(args, workspaceID)
	}
	if clusterID != "" {
		query += fmt.Sprintf(` AND cluster_id=$%d`, len(args)+1)
		args = append(args, clusterID)
	}
	query += ` ORDER BY name`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	namespaces := make([]core.Namespace, 0)
	for rows.Next() {
		var ns core.Namespace
		var labelsJSON, quotaJSON, clusterIDDB, workspaceIDDB sql.NullString
		if err := rows.Scan(&ns.ID, &workspaceIDDB, &clusterIDDB, &ns.Name, &ns.Description, &labelsJSON, &quotaJSON, &ns.Status, &ns.CreatedAt, &ns.UpdatedAt); err != nil {
			return nil, err
		}
		if workspaceIDDB.Valid {
			ns.WorkspaceID = workspaceIDDB.String
		}
		if clusterIDDB.Valid {
			ns.ClusterID = clusterIDDB.String
		}
		if labelsJSON.Valid && labelsJSON.String != "" {
			_ = json.Unmarshal([]byte(labelsJSON.String), &ns.Labels)
		}
		if quotaJSON.Valid && quotaJSON.String != "" {
			_ = json.Unmarshal([]byte(quotaJSON.String), &ns.Quota)
		}
		namespaces = append(namespaces, ns)
	}
	return namespaces, rows.Err()
}

func (r *SQLRepository) GetNamespace(ctx context.Context, tenantID, namespaceID string) (*core.Namespace, error) {
	var ns core.Namespace
	var labelsJSON, quotaJSON, clusterIDDB, workspaceIDDB sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, cluster_id, name, description, labels, quota, status, created_at, updated_at FROM namespaces WHERE id=$1 AND tenant_id=$2`,
		namespaceID, tenantID).
		Scan(&ns.ID, &workspaceIDDB, &clusterIDDB, &ns.Name, &ns.Description, &labelsJSON, &quotaJSON, &ns.Status, &ns.CreatedAt, &ns.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if clusterIDDB.Valid {
		ns.ClusterID = clusterIDDB.String
	}
	if workspaceIDDB.Valid {
		ns.WorkspaceID = workspaceIDDB.String
	}
	if labelsJSON.Valid && labelsJSON.String != "" {
		_ = json.Unmarshal([]byte(labelsJSON.String), &ns.Labels)
	}
	if quotaJSON.Valid && quotaJSON.String != "" {
		_ = json.Unmarshal([]byte(quotaJSON.String), &ns.Quota)
	}
	return &ns, nil
}

func (r *SQLRepository) CreateNamespace(ctx context.Context, tenantID string, ns core.Namespace) error {
	if ns.WorkspaceID != "" {
		var wsTenantID string
		err := r.db.QueryRowContext(ctx, `SELECT tenant_id FROM workspaces WHERE id=$1`, ns.WorkspaceID).Scan(&wsTenantID)
		if err != nil {
			return err
		}
		if wsTenantID != tenantID {
			return fmt.Errorf("workspace %s does not belong to tenant %s", ns.WorkspaceID, tenantID)
		}
	} else if ns.ClusterID == "" {
		return fmt.Errorf("cluster_id is required when workspace_id is omitted")
	}
	if ns.ClusterID != "" {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM runtime_targets WHERE id=$1)`, ns.ClusterID).Scan(&exists); err != nil || !exists {
			return fmt.Errorf("cluster %s not found", ns.ClusterID)
		}
	}
	var clusterIDArg any
	if ns.ClusterID != "" {
		clusterIDArg = ns.ClusterID
	}
	labelsJSON, _ := json.Marshal(ns.Labels)
	quotaJSON, _ := json.Marshal(ns.Quota)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO namespaces (id, workspace_id, tenant_id, cluster_id, name, description, labels, quota, status) VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, $8, $9)`,
		ns.ID, ns.WorkspaceID, tenantID, clusterIDArg, ns.Name, ns.Description, labelsJSON, quotaJSON, ns.Status)
	return err
}

func (r *SQLRepository) UpdateNamespace(ctx context.Context, tenantID string, ns core.Namespace) error {
	labelsJSON, _ := json.Marshal(ns.Labels)
	quotaJSON, _ := json.Marshal(ns.Quota)
	_, err := r.db.ExecContext(ctx,
		`UPDATE namespaces SET description=$1, cluster_id=$2, labels=$3, quota=$4, updated_at=NOW() WHERE id=$5 AND tenant_id=$6`,
		ns.Description, nullOr(ns.ClusterID), labelsJSON, quotaJSON, ns.ID, tenantID)
	return err
}

func (r *SQLRepository) DeleteNamespace(ctx context.Context, tenantID, namespaceID string) error {
	_, _ = r.db.ExecContext(ctx,
		`DELETE FROM scoped_role_bindings WHERE tenant_id=$1 AND scope_kind='namespace' AND namespace_id=$2`,
		tenantID, namespaceID)
	_, err := r.db.ExecContext(ctx, `DELETE FROM namespaces WHERE id=$1 AND tenant_id=$2`, namespaceID, tenantID)
	return err
}

func (r *SQLRepository) GetClusterTenant(ctx context.Context, clusterID string) (string, error) {
	var tenantID string
	err := r.db.QueryRowContext(ctx, `SELECT tenant_id FROM runtime_targets WHERE id=$1`, clusterID).Scan(&tenantID)
	return tenantID, err
}

func (r *SQLRepository) CreateClusterShare(ctx context.Context, share core.ClusterShare) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cluster_shares (id, cluster_id, grantee_tenant_id, grantee_workspace_id, permissions, k8s_namespace_prefix, tenant_isolation_enabled, created_by_subject_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		share.ID, share.ClusterID, share.GranteeTenantID, nullOr(share.GranteeWorkspaceID),
		share.Permissions, share.K8sNamespacePrefix, share.TenantIsolationEnabled, nullOr(share.CreatedBySubjectID))
	return err
}

func (r *SQLRepository) DeleteClusterShare(ctx context.Context, shareID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM cluster_shares WHERE id=$1`, shareID)
	return err
}

func (r *SQLRepository) ListClusterShares(ctx context.Context, clusterID string) ([]core.ClusterShare, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, cluster_id, grantee_tenant_id, COALESCE(grantee_workspace_id::text, ''), permissions, k8s_namespace_prefix, tenant_isolation_enabled, created_at
		FROM cluster_shares WHERE cluster_id=$1 ORDER BY created_at`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shares []core.ClusterShare
	for rows.Next() {
		var s core.ClusterShare
		var perms []string
		if err := rows.Scan(&s.ID, &s.ClusterID, &s.GranteeTenantID, &s.GranteeWorkspaceID, &perms, &s.K8sNamespacePrefix, &s.TenantIsolationEnabled, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Permissions = perms
		shares = append(shares, s)
	}
	return shares, rows.Err()
}

func (r *SQLRepository) ClusterAccessibleTo(ctx context.Context, clusterID, tenantID, workspaceID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM runtime_targets WHERE id=$1 AND tenant_id=$2
			UNION ALL
			SELECT 1 FROM tenant_cluster_allocations WHERE cluster_id=$1 AND tenant_id=$2 AND status='active'
			UNION ALL
			SELECT 1 FROM cluster_shares WHERE cluster_id=$1 AND grantee_tenant_id=$2 AND (grantee_workspace_id IS NULL OR grantee_workspace_id::text=$3)
		)`, clusterID, tenantID, workspaceID).Scan(&exists)
	return exists, err
}

func (r *SQLRepository) ListClustersForWorkspace(ctx context.Context, workspaceID string) ([]core.RuntimeTarget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rt.id, COALESCE(rt.tenant_id, ''), rt.name, COALESCE(rt.display_name, ''), rt.target_type, rt.status, rt.is_active, rt.created_at,
			CASE WHEN cs.id IS NOT NULL THEN true ELSE false END AS shared
		FROM runtime_targets rt
		LEFT JOIN cluster_shares cs ON cs.cluster_id = rt.id AND cs.grantee_workspace_id::text = $1
		WHERE rt.workspace_id::text = $1 OR cs.id IS NOT NULL
		ORDER BY cs.id IS NOT NULL, rt.name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []core.RuntimeTarget
	for rows.Next() {
		var t core.RuntimeTarget
		var shared bool
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.DisplayName, &t.TargetType, &t.Status, &t.IsActive, &t.CreatedAt, &shared); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (r *SQLRepository) FindClusterShare(ctx context.Context, clusterID, granteeTenantID, granteeWorkspaceID string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM cluster_shares WHERE cluster_id=$1 AND grantee_tenant_id=$2 AND grantee_workspace_id::text=$3`,
		clusterID, granteeTenantID, granteeWorkspaceID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

const namespaceMemberRoleName = "namespace-member"

func (r *SQLRepository) EnsureNamespaceRole(ctx context.Context, tenantID string) (string, error) {
	var roleID string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM scoped_roles WHERE tenant_id=$1 AND name=$2 AND is_active=true`,
		tenantID, namespaceMemberRoleName).Scan(&roleID)
	if err == nil {
		return roleID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	perms, _ := json.Marshal([]map[string]any{
		{"resourceKind": "*", "action": "read", "tenantId": tenantID},
		{"resourceKind": "*", "action": "list", "tenantId": tenantID},
	})
	roleID = uuid.NewString()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO scoped_roles (id, tenant_id, name, permissions, is_active) VALUES ($1,$2,$3,$4,true)
		ON CONFLICT (tenant_id, name) DO NOTHING`,
		roleID, tenantID, namespaceMemberRoleName, string(perms))
	if err != nil {
		return "", err
	}
	// Re-read in case another request won the insert race.
	err = r.db.QueryRowContext(ctx,
		`SELECT id FROM scoped_roles WHERE tenant_id=$1 AND name=$2`, tenantID, namespaceMemberRoleName).Scan(&roleID)
	if err != nil {
		return "", err
	}
	return roleID, nil
}

func (r *SQLRepository) AddNamespaceMember(ctx context.Context, tenantID, namespaceID, subjectID, roleID string) error {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM scoped_role_bindings WHERE tenant_id=$1 AND subject_id=$2 AND scope_kind='namespace' AND namespace_id=$3 AND revoked_at IS NULL)`,
		tenantID, subjectID, namespaceID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO scoped_role_bindings (id, tenant_id, subject_id, role_id, scope_kind, namespace_id, actions)
		VALUES ($1,$2,$3,$4,'namespace',$5,ARRAY['read','list'])`,
		uuid.NewString(), tenantID, subjectID, roleID, namespaceID)
	return err
}

func (r *SQLRepository) RemoveNamespaceMember(ctx context.Context, tenantID, namespaceID, subjectID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE scoped_role_bindings SET revoked_at=NOW() WHERE tenant_id=$1 AND subject_id=$2 AND scope_kind='namespace' AND namespace_id=$3 AND revoked_at IS NULL`,
		tenantID, subjectID, namespaceID)
	return err
}

func (r *SQLRepository) ListNamespaceMembers(ctx context.Context, tenantID, namespaceID string) ([]NamespaceMember, error) {
	query := `
		SELECT srb.id, srb.subject_id, u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.email,''),
		       srb.role_id, COALESCE(sr.name,''), srb.granted_at
		FROM scoped_role_bindings srb
		JOIN identity_subjects s ON s.id = srb.subject_id
		JOIN users u ON u.id = s.external_subject
		LEFT JOIN scoped_roles sr ON sr.id = srb.role_id
		WHERE srb.tenant_id=$1 AND srb.scope_kind='namespace' AND srb.namespace_id=$2 AND srb.revoked_at IS NULL
		ORDER BY u.username`
	rows, err := r.db.QueryContext(ctx, query, tenantID, namespaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]NamespaceMember, 0)
	for rows.Next() {
		var m NamespaceMember
		if err := rows.Scan(&m.BindingID, &m.SubjectID, &m.UserID, &m.Username, &m.DisplayName, &m.Email, &m.RoleID, &m.RoleName, &m.GrantedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *SQLRepository) ListTenantUsers(ctx context.Context, tenantID string) ([]TenantUser, error) {
	query := `
		SELECT s.id, u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.email,'')
		FROM tenant_memberships tm
		JOIN identity_subjects s ON s.id = tm.subject_id
		JOIN users u ON u.id = s.external_subject
		WHERE tm.tenant_id=$1 AND tm.status='active'
		ORDER BY u.username`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]TenantUser, 0)
	for rows.Next() {
		var u TenantUser
		if err := rows.Scan(&u.SubjectID, &u.UserID, &u.Username, &u.DisplayName, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func nullOr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

package reconciler

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/F31/hnb/cmd/rbac-syncer/internal/config"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/informer"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/mapper"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/metrics"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/watcher"
)

type Syncer struct {
	cfg                 *config.Config
	clientset           kubernetes.Interface
	db                  *sql.DB
	userRoleWatcher     *watcher.UserRoleWatcher
	namespaceWatcher    *watcher.NamespaceWatcher
	roleBindingInformer *informer.RoleBindingInformer
	namespaceFinder     *mapper.NamespaceFinder
	auditLogger         *metrics.AuditLogger
}

func NewSyncer(
	cfg *config.Config,
	clientset kubernetes.Interface,
	db *sql.DB,
	userRoleWatcher *watcher.UserRoleWatcher,
	namespaceWatcher *watcher.NamespaceWatcher,
	roleBindingInformer *informer.RoleBindingInformer,
	auditLogger *metrics.AuditLogger,
) *Syncer {
	return &Syncer{
		cfg:                 cfg,
		clientset:           clientset,
		db:                  db,
		userRoleWatcher:     userRoleWatcher,
		namespaceWatcher:    namespaceWatcher,
		roleBindingInformer: roleBindingInformer,
		namespaceFinder:     mapper.NewNamespaceFinder(db),
		auditLogger:         auditLogger,
	}
}

func (s *Syncer) Start(ctx context.Context) {
	s.roleBindingInformer.Start(ctx)
	s.userRoleWatcher.Start(ctx)
	s.namespaceWatcher.Start(ctx)

	s.userRoleWatcher.OnChange(func(roles []watcher.UserRole) {
		s.reconcile(ctx, roles)
	})

	s.namespaceWatcher.OnCreate(func(namespaces []watcher.NamespaceRecord) {
		s.backfillNamespaces(ctx, namespaces)
	})

	<-ctx.Done()
}

func (s *Syncer) backfillNamespaces(ctx context.Context, namespaces []watcher.NamespaceRecord) {
	for _, ns := range namespaces {
		if err := s.backfillNamespace(ctx, ns); err != nil {
			klog.Errorf("Failed to backfill namespace %s (tenant=%s, project=%s): %v", ns.Name, ns.TenantID, ns.ProjectID, err)
		}
	}
}

func (s *Syncer) backfillNamespace(ctx context.Context, ns watcher.NamespaceRecord) error {
	query := `
		SELECT ur.user_id, ur.tenant_id, ur.project_id, r.name, ur.revoked_at
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.tenant_id = $1
		AND (ur.project_id IS NULL OR ur.project_id = $2)
		AND ur.revoked_at IS NULL
	`

	rows, err := s.db.QueryContext(ctx, query, ns.TenantID, ns.ProjectID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type backfillRole struct {
		UserID    string
		TenantID  string
		ProjectID *string
		RoleName  string
		RevokedAt *time.Time
	}

	var roles []backfillRole
	for rows.Next() {
		var r backfillRole
		if err := rows.Scan(&r.UserID, &r.TenantID, &r.ProjectID, &r.RoleName, &r.RevokedAt); err != nil {
			return err
		}
		roles = append(roles, r)
	}

	for _, r := range roles {
		clusterRoleName, err := mapper.RoleNameToClusterRole(r.RoleName)
		if err != nil {
			klog.Errorf("Skip role %s for user %s: %v", r.RoleName, r.UserID, err)
			continue
		}

		role := watcher.UserRole{
			UserID:    r.UserID,
			TenantID:  r.TenantID,
			RoleName:  r.RoleName,
			RevokedAt: r.RevokedAt,
		}

		if err := retryWithBackoff(ctx, s.cfg.MaxRetries, func(retryCtx context.Context) error {
			return s.ensureRoleBindingPresent(retryCtx, ns.Name, role, clusterRoleName)
		}); err != nil {
			return fmt.Errorf("backfill rolebinding for user %s in ns %s: %w", r.UserID, ns.Name, err)
		}
	}

	return nil
}

func (s *Syncer) reconcile(ctx context.Context, roles []watcher.UserRole) {
	klog.Infof("Reconciling %d user role assignments", len(roles))

	for _, role := range roles {
		start := time.Now()
		err := s.reconcileRole(ctx, role)

		latency := time.Since(start)
		metrics.RecordSyncLatency(latency)

		if err != nil {
			metrics.RecordSyncResult(false)
			metrics.RecordSyncError(err.Error(), "", role.UserID)
			action := "sync_create"
			if role.RevokedAt != nil {
				action = "sync_revoke"
			}
			s.auditLogger.LogSyncFailure(ctx, role.UserID, role.TenantID, "", action, err.Error())
			klog.Errorf("Failed to reconcile role %+v: %v", role, err)
		} else {
			metrics.RecordSyncResult(true)
		}
	}

	managedCount := len(s.roleBindingInformer.ListRoleBindings())
	metrics.RecordManagedRoleBindings(managedCount)
}

func (s *Syncer) reconcileRole(ctx context.Context, role watcher.UserRole) error {
	clusterRoleName, err := mapper.RoleNameToClusterRole(role.RoleName)
	if err != nil {
		return fmt.Errorf("map role %s: %w", role.RoleName, err)
	}

	namespaces, err := s.namespaceFinder.FindNamespaces(ctx, role.TenantID, role.ProjectID)
	if err != nil {
		return fmt.Errorf("find namespaces for tenant=%s project=%v: %w", role.TenantID, role.ProjectID, err)
	}

	for _, ns := range namespaces {
		if err := retryWithBackoff(ctx, s.cfg.MaxRetries, func(retryCtx context.Context) error {
			if role.RevokedAt != nil {
				return s.ensureRoleBindingDeleted(retryCtx, ns, role)
			}
			return s.ensureRoleBindingPresent(retryCtx, ns, role, clusterRoleName)
		}); err != nil {
			return fmt.Errorf("reconcile rolebinding in %s after retries: %w", ns, err)
		}
	}

	return nil
}

func (s *Syncer) ensureRoleBindingPresent(ctx context.Context, namespace string, role watcher.UserRole, clusterRoleName string) error {
	bindingName := makeBindingName(role.RoleName, role.TenantID)

	if s.cfg.ShadowMode {
		klog.Infof("[SHADOW] Would create RoleBinding %s/%s: User=%s -> ClusterRole=%s", namespace, bindingName, role.UserID, clusterRoleName)
		return nil
	}

	existing, err := s.clientset.RbacV1().RoleBindings(namespace).Get(ctx, bindingName, metav1.GetOptions{})
	if err == nil {
		if existing.RoleRef.Name == clusterRoleName {
			klog.V(4).Infof("RoleBinding %s/%s already up-to-date", namespace, bindingName)
			return nil
		}
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "hnb-rbac-syncer",
				"hnb.cloud/tenant-id":          role.TenantID,
				"hnb.cloud/role":               role.RoleName,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     fmt.Sprintf("hnb:%s", role.UserID),
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	if err != nil {
		_, err = s.clientset.RbacV1().RoleBindings(namespace).Create(ctx, roleBinding, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create rolebinding: %w", err)
		}
		klog.Infof("Created RoleBinding %s/%s -> %s", namespace, bindingName, clusterRoleName)
	} else {
		existing.RoleRef = roleBinding.RoleRef
		existing.Subjects = roleBinding.Subjects
		existing.Labels = roleBinding.Labels
		_, err = s.clientset.RbacV1().RoleBindings(namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update rolebinding: %w", err)
		}
		klog.Infof("Updated RoleBinding %s/%s -> %s", namespace, bindingName, clusterRoleName)
	}

	return nil
}

func (s *Syncer) ensureRoleBindingDeleted(ctx context.Context, namespace string, role watcher.UserRole) error {
	bindingName := makeBindingName(role.RoleName, role.TenantID)

	if s.cfg.ShadowMode {
		klog.Infof("[SHADOW] Would delete RoleBinding %s/%s", namespace, bindingName)
		return nil
	}

	err := s.clientset.RbacV1().RoleBindings(namespace).Delete(ctx, bindingName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete rolebinding: %w", err)
	}

	klog.Infof("Deleted RoleBinding %s/%s (role revoked)", namespace, bindingName)
	return nil
}

func makeBindingName(roleName, tenantID string) string {
	return fmt.Sprintf("hnb:%s:%s", roleName, tenantID)
}

func retryWithBackoff(ctx context.Context, maxRetries int, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := fn(ctx); err != nil {
			if errors.IsConflict(err) || isRetryable(err) {
				lastErr = err
				if attempt < maxRetries {
					delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
					jitter := time.Duration(rand.Int63n(int64(delay) / 2))
					totalDelay := delay + jitter
					klog.V(4).Infof("Retry attempt %d/%d after %v: %v", attempt+1, maxRetries, totalDelay, err)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(totalDelay):
					}
				}
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	retryableMsgs := []string{
		"connection refused",
		"timeout",
		"server closed",
		"too many requests",
		"internal error",
	}
	for _, msg := range retryableMsgs {
		if contains(errMsg, msg) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

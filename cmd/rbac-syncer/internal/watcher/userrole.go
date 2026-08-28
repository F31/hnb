package watcher

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

type UserRole struct {
	UserID    string
	TenantID  string
	ProjectID *string
	RoleID    string
	RoleName  string
	RevokedAt *time.Time
}

type UserRoleWatcher struct {
	db          *sql.DB
	interval    time.Duration
	lastSync    time.Time
	lastHash    string
	mu          sync.RWMutex
	onChange    func([]UserRole)
}

func NewUserRoleWatcher(db *sql.DB, interval time.Duration) *UserRoleWatcher {
	return &UserRoleWatcher{
		db:       db,
		interval: interval,
	}
}

func (w *UserRoleWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	if err := w.poll(ctx); err != nil {
		klog.Errorf("Initial user_role poll failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.poll(ctx); err != nil {
				klog.Errorf("User_role poll failed: %v", err)
			}
		}
	}
}

func (w *UserRoleWatcher) poll(ctx context.Context) error {
	query := `
		SELECT ur.user_id, ur.tenant_id, ur.project_id, r.id, r.name, ur.revoked_at
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		ORDER BY ur.user_id, ur.tenant_id, ur.project_id
	`

	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var roles []UserRole
	for rows.Next() {
		var r UserRole
		if err := rows.Scan(&r.UserID, &r.TenantID, &r.ProjectID, &r.RoleID, &r.RoleName, &r.RevokedAt); err != nil {
			return err
		}
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	currentHash := hashRoles(roles)
	w.mu.RLock()
	lastHash := w.lastHash
	w.mu.RUnlock()

	if currentHash != lastHash {
		klog.Infof("User role change detected: %d active records", len(roles))
		w.mu.Lock()
		w.lastHash = currentHash
		w.lastSync = time.Now()
		w.mu.Unlock()

		if w.onChange != nil {
			w.onChange(roles)
		}
	}

	return nil
}

func (w *UserRoleWatcher) OnChange(fn func([]UserRole)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = fn
}

func (w *UserRoleWatcher) GetLastSync() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastSync
}

func hashRoles(roles []UserRole) string {
	var sum string
	for _, r := range roles {
		sum += r.UserID + r.TenantID
		if r.ProjectID != nil {
			sum += *r.ProjectID
		}
		sum += r.RoleID
	}
	return sum
}

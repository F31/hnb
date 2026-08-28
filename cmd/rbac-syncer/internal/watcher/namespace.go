package watcher

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

type NamespaceRecord struct {
	ID            string
	TenantID      string
	EnvironmentID string
	ProjectID     string
	Name          string
	Status        string
	CreatedAt     time.Time
}

type NamespaceWatcher struct {
	db        *sql.DB
	interval  time.Duration
	mu        sync.RWMutex
	knownIDs  map[string]bool
	lastHash  string
	onCreate  func([]NamespaceRecord)
}

func NewNamespaceWatcher(db *sql.DB, interval time.Duration) *NamespaceWatcher {
	return &NamespaceWatcher{
		db:       db,
		interval: interval,
		knownIDs: make(map[string]bool),
	}
}

func (w *NamespaceWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	if err := w.poll(ctx); err != nil {
		klog.Errorf("Initial namespace poll failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.poll(ctx); err != nil {
				klog.Errorf("Namespace poll failed: %v", err)
			}
		}
	}
}

func (w *NamespaceWatcher) poll(ctx context.Context) error {
	query := `
		SELECT id, tenant_id, environment_id, project_id, name, status, created_at
		FROM namespaces
		WHERE status = 'active'
		ORDER BY created_at
	`

	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var newNamespaces []NamespaceRecord
	for rows.Next() {
		var ns NamespaceRecord
		if err := rows.Scan(&ns.ID, &ns.TenantID, &ns.EnvironmentID, &ns.ProjectID, &ns.Name, &ns.Status, &ns.CreatedAt); err != nil {
			return err
		}

		w.mu.RLock()
		known := w.knownIDs[ns.ID]
		w.mu.RUnlock()

		if !known {
			newNamespaces = append(newNamespaces, ns)
			w.mu.Lock()
			w.knownIDs[ns.ID] = true
			w.mu.Unlock()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(newNamespaces) > 0 {
		klog.Infof("New namespaces detected: %d", len(newNamespaces))
		for _, ns := range newNamespaces {
			klog.Infof("New namespace: %s (tenant=%s, project=%s)", ns.Name, ns.TenantID, ns.ProjectID)
		}

		if w.onCreate != nil {
			w.onCreate(newNamespaces)
		}
	}

	return nil
}

func (w *NamespaceWatcher) OnCreate(fn func([]NamespaceRecord)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onCreate = fn
}

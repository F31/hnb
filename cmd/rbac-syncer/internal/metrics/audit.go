package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type AuditEntry struct {
	EventType string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	ActorID   string                 `json:"actor_id,omitempty"`
	TenantID  string                 `json:"tenant_id,omitempty"`
	Namespace string                 `json:"namespace,omitempty"`
	Action    string                 `json:"action"`
	Result    string                 `json:"result"`
	Detail    string                 `json:"detail,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type AuditLogger struct {
	db *sql.DB
}

func NewAuditLogger(db *sql.DB) *AuditLogger {
	return &AuditLogger{db: db}
}

func (a *AuditLogger) LogSyncEvent(ctx context.Context, entry AuditEntry) error {
	entry.EventType = "rbac_sync"
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	metadataJSON, err := json.Marshal(entry.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO audit_log (event_type, timestamp, actor_id, tenant_id, namespace, action, result, detail, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = a.db.ExecContext(ctx, query,
		entry.EventType,
		entry.Timestamp,
		entry.ActorID,
		entry.TenantID,
		entry.Namespace,
		entry.Action,
		entry.Result,
		entry.Detail,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return nil
}

func (a *AuditLogger) LogSyncSuccess(ctx context.Context, actorID, tenantID, namespace, action string) {
	a.LogSyncEvent(ctx, AuditEntry{
		ActorID:   actorID,
		TenantID:  tenantID,
		Namespace: namespace,
		Action:    action,
		Result:    "success",
	})
}

func (a *AuditLogger) LogSyncFailure(ctx context.Context, actorID, tenantID, namespace, action, detail string) {
	a.LogSyncEvent(ctx, AuditEntry{
		ActorID:   actorID,
		TenantID:  tenantID,
		Namespace: namespace,
		Action:    action,
		Result:    "failure",
		Detail:    detail,
	})
}

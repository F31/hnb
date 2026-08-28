package store

import (
	"context"
	"encoding/json"
)

func (s *PGStore) AppendSecurityAudit(ctx context.Context, record SecurityAuditRecord) error {
	detail, err := json.Marshal(record.Detail)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO security_audit_events (
			tenant_id, subject_id, event_type, decision, reason_code, action,
			resource_kind, resource_id, scope, correlation_id, trace_id, outcome, detail
		) VALUES ($1,$2,$3,$4,$5,$6,'runtimeTarget',NULLIF($7,''),'{}',$8,$9,$10,$11)`,
		record.TenantID, record.SubjectID, record.EventType, record.Decision, record.ReasonCode,
		record.Action, record.ResourceID, record.CorrelationID, record.TraceID, record.Outcome, string(detail))
	return err
}

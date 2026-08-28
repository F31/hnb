package store

import (
	"context"
	"time"
)

type SLOConfig struct {
	ID              string        `json:"id"`
	OperationType   string        `json:"operation_type"`
	MaxDuration     time.Duration `json:"max_duration"`
	AlertSeverity   string        `json:"alert_severity"`
	EscalationDelay time.Duration `json:"escalation_delay"`
}

type SLOAlert struct {
	ID            string     `json:"id"`
	OperationID   string     `json:"operation_id"`
	TenantID      string     `json:"tenant_id"`
	OperationType string     `json:"operation_type"`
	Status        string     `json:"status"`
	StalledSince  time.Time  `json:"stalled_since"`
	AlertSentAt   *time.Time `json:"alert_sent_at,omitempty"`
	EscalatedAt   *time.Time `json:"escalated_at,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
}

func (s *PGStore) GetSLOConfig(ctx context.Context, operationType string) (*SLOConfig, error) {
	cfg := &SLOConfig{}
	var maxDuration time.Duration
	var escDelay time.Duration
	err := s.db.QueryRowContext(ctx, `
		SELECT id, operation_type, max_duration, alert_severity, escalation_delay
		FROM operation_slo_config WHERE operation_type = $1`, operationType,
	).Scan(&cfg.ID, &cfg.OperationType, &maxDuration, &cfg.AlertSeverity, &escDelay)
	if err != nil {
		return nil, err
	}
	cfg.MaxDuration = maxDuration
	cfg.EscalationDelay = escDelay
	return cfg, nil
}

func (s *PGStore) SaveSLOConfig(ctx context.Context, cfg *SLOConfig) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO operation_slo_config (operation_type, max_duration, alert_severity, escalation_delay)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (operation_type) DO UPDATE SET
			max_duration = EXCLUDED.max_duration,
			alert_severity = EXCLUDED.alert_severity,
			escalation_delay = EXCLUDED.escalation_delay`,
		cfg.OperationType, cfg.MaxDuration, cfg.AlertSeverity, cfg.EscalationDelay,
	)
	return err
}

func (s *PGStore) CheckStalledOperations(ctx context.Context, threshold time.Duration) ([]SLOAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, operation_type, status, last_state_changed_at
		FROM operations
		WHERE status NOT IN ('succeeded', 'failed', 'cancelled')
		AND last_state_changed_at < NOW() - $1
		ORDER BY last_state_changed_at`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []SLOAlert
	for rows.Next() {
		var a SLOAlert
		var stalledSince time.Time
		if err := rows.Scan(&a.ID, &a.TenantID, &a.OperationType, &a.Status, &stalledSince); err != nil {
			return nil, err
		}
		a.OperationID = a.ID
		a.StalledSince = stalledSince
		a.Status = "stalled"
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func (s *PGStore) RecordSLOAlert(ctx context.Context, alert *SLOAlert) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO operation_slo_alerts (operation_id, tenant_id, operation_type, status, stalled_since, alert_sent_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (operation_id) DO UPDATE SET
			status = EXCLUDED.status,
			alert_sent_at = CASE WHEN operation_slo_alerts.alert_sent_at IS NULL THEN NOW() ELSE operation_slo_alerts.alert_sent_at END`,
		alert.OperationID, alert.TenantID, alert.OperationType, alert.Status, alert.StalledSince,
	)
	return err
}

func (s *PGStore) ResolveSLOAlert(ctx context.Context, operationID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE operation_slo_alerts SET status = 'resolved', resolved_at = NOW()
		WHERE operation_id = $1 AND status = 'stalled'`, operationID)
	return err
}

func (s *PGStore) ListActiveSLOAlerts(ctx context.Context) ([]SLOAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, operation_id, tenant_id, operation_type, status, stalled_since, alert_sent_at, escalated_at, resolved_at
		FROM operation_slo_alerts WHERE status = 'stalled' ORDER BY stalled_since`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []SLOAlert
	for rows.Next() {
		var a SLOAlert
		if err := rows.Scan(&a.ID, &a.OperationID, &a.TenantID, &a.OperationType, &a.Status, &a.StalledSince, &a.AlertSentAt, &a.EscalatedAt, &a.ResolvedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

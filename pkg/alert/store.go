package alert

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type AlertDBStore struct {
	db *sql.DB
}

func NewAlertDBStore(db *sql.DB) *AlertDBStore {
	return &AlertDBStore{db: db}
}

func (s *AlertDBStore) CreateRule(rule *AlertRule) error {
	labelsJSON, _ := json.Marshal(rule.Labels)
	annotationsJSON, _ := json.Marshal(rule.Annotations)
	_, err := s.db.Exec(`
		INSERT INTO alert_rules (id, tenant_scope, name, description, source_type, severity, enabled, expression_ref, labels, annotations, created_at)
		VALUES ($1, 'global', $2, NULLIF($3, ''), 'legacy-event', $4, $5, NULLIF($6,''), $7, $8, $9)`,
		rule.ID, rule.Name, rule.Description, string(rule.Severity), rule.Enabled,
		rule.Expr, labelsJSON, annotationsJSON, time.Now())
	return err
}

func (s *AlertDBStore) UpdateRule(rule *AlertRule) error {
	labelsJSON, _ := json.Marshal(rule.Labels)
	annotationsJSON, _ := json.Marshal(rule.Annotations)
	_, err := s.db.Exec(`
		UPDATE alert_rules SET name=$2, description=NULLIF($3,''), severity=$4, enabled=$5,
		       expression_ref=NULLIF($6,''), labels=$7, annotations=$8, updated_at=NOW()
		WHERE id=$1`,
		rule.ID, rule.Name, rule.Description, string(rule.Severity), rule.Enabled,
		rule.Expr, labelsJSON, annotationsJSON)
	return err
}

func (s *AlertDBStore) DeleteRule(id string) error {
	_, err := s.db.Exec(`DELETE FROM alert_rules WHERE id = $1`, id)
	return err
}

func (s *AlertDBStore) GetRule(id string) (*AlertRule, error) {
	var rule AlertRule
	var labelsJSON, annotationsJSON []byte
	var description sql.NullString

	err := s.db.QueryRow(`
		SELECT id, COALESCE(name,''), description, severity, enabled, COALESCE(expression_ref,''),
		       EXTRACT(EPOCH FROM duration), labels, annotations, created_at
		FROM alert_rules WHERE id = $1`, id).
		Scan(&rule.ID, &rule.Name, &description, &rule.Severity, &rule.Enabled,
			&rule.Expr, newDurationScanner(&rule.Duration), &labelsJSON, &annotationsJSON, &rule.CreatedAt)
	if err != nil {
		return nil, err
	}
	if description.Valid {
		rule.Description = description.String
	}
	json.Unmarshal(labelsJSON, &rule.Labels)
	json.Unmarshal(annotationsJSON, &rule.Annotations)
	return &rule, nil
}

func (s *AlertDBStore) ListRules() ([]AlertRule, error) {
	rows, err := s.db.Query(`
		SELECT id, COALESCE(name,''), description, severity, enabled, COALESCE(expression_ref,''),
		       EXTRACT(EPOCH FROM duration), labels, annotations, created_at
		FROM alert_rules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		var rule AlertRule
		var labelsJSON, annotationsJSON []byte
		var description sql.NullString
		if err := rows.Scan(&rule.ID, &rule.Name, &description, &rule.Severity, &rule.Enabled,
			&rule.Expr, newDurationScanner(&rule.Duration), &labelsJSON, &annotationsJSON, &rule.CreatedAt); err != nil {
			continue
		}
		if description.Valid {
			rule.Description = description.String
		}
		json.Unmarshal(labelsJSON, &rule.Labels)
		json.Unmarshal(annotationsJSON, &rule.Annotations)
		rules = append(rules, rule)
	}
	return rules, nil
}

func (s *AlertDBStore) CreateEvent(event *AlertEvent) error {
	labelsJSON, _ := json.Marshal(event.Labels)
	annotationsJSON, _ := json.Marshal(event.Annotations)
	_, err := s.db.Exec(`
		INSERT INTO alert_instances (id, tenant_id, rule_id, source, severity, fingerprint, state, summary,
			first_seen_at, last_seen_at, resolved_at, acknowledged_by, labels, source_ref)
		VALUES ($1, COALESCE(NULLIF($2,''),'global'), NULLIF($3,'')::uuid, 'legacy-event', $4,
			$1, $5, $6, $7, $7, $8, NULLIF($9,''), $10, $11)`,
		event.ID, event.Labels["tenant_id"], event.RuleID, string(event.Severity), string(event.Status),
		event.Message, event.StartedAt, event.ResolvedAt, event.AcknowledgedBy, labelsJSON, string(annotationsJSON))
	return err
}

func (s *AlertDBStore) UpdateEvent(event *AlertEvent) error {
	_, err := s.db.Exec(`
		UPDATE alert_instances SET state=$2, resolved_at=$3, acknowledged_by=NULLIF($4,''), updated_at=now() WHERE id=$1`,
		event.ID, string(event.Status), event.ResolvedAt, event.AcknowledgedBy)
	return err
}

func (s *AlertDBStore) GetEvent(id string) (*AlertEvent, error) {
	var event AlertEvent
	var labelsJSON, annotationsJSON []byte
	var resolvedAt sql.NullTime
	var acknowledgedBy sql.NullString

	err := s.db.QueryRow(`
		SELECT a.id, COALESCE(a.rule_id::text,''), COALESCE(r.name,''), a.severity, a.state,
		       a.summary, a.labels, COALESCE(a.source_ref,'{}')::bytea, 0, a.first_seen_at,
		       a.resolved_at, a.acknowledged_by
		FROM alert_instances a LEFT JOIN alert_rules r ON r.id=a.rule_id WHERE a.id = $1`, id).
		Scan(&event.ID, &event.RuleID, &event.RuleName, &event.Severity, &event.Status,
			&event.Message, &labelsJSON, &annotationsJSON, &event.Value, &event.StartedAt,
			&resolvedAt, &acknowledgedBy)
	if err != nil {
		return nil, err
	}
	if resolvedAt.Valid {
		event.ResolvedAt = &resolvedAt.Time
	}
	if acknowledgedBy.Valid {
		event.AcknowledgedBy = acknowledgedBy.String
	}
	json.Unmarshal(labelsJSON, &event.Labels)
	json.Unmarshal(annotationsJSON, &event.Annotations)
	return &event, nil
}

func (s *AlertDBStore) ListEvents(severity Severity, status Status, limit int) ([]AlertEvent, error) {
	query := `SELECT a.id, COALESCE(a.rule_id::text,''), COALESCE(r.name,''), a.severity, a.state,
		a.summary, a.labels, '{}'::jsonb, 0, a.first_seen_at, a.resolved_at, a.acknowledged_by
		FROM alert_instances a LEFT JOIN alert_rules r ON r.id=a.rule_id WHERE 1=1`
	args := []any{}
	argIdx := 1

	if severity != "" {
		query += fmt.Sprintf(" AND severity=$%d", argIdx)
		args = append(args, string(severity))
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status=$%d", argIdx)
		args = append(args, string(status))
		argIdx++
	}
	query += " ORDER BY first_seen_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []AlertEvent
	for rows.Next() {
		var event AlertEvent
		var labelsJSON, annotationsJSON []byte
		var resolvedAt sql.NullTime
		var acknowledgedBy sql.NullString
		if err := rows.Scan(&event.ID, &event.RuleID, &event.RuleName, &event.Severity, &event.Status,
			&event.Message, &labelsJSON, &annotationsJSON, &event.Value, &event.StartedAt,
			&resolvedAt, &acknowledgedBy); err != nil {
			continue
		}
		if resolvedAt.Valid {
			event.ResolvedAt = &resolvedAt.Time
		}
		if acknowledgedBy.Valid {
			event.AcknowledgedBy = acknowledgedBy.String
		}
		json.Unmarshal(labelsJSON, &event.Labels)
		json.Unmarshal(annotationsJSON, &event.Annotations)
		events = append(events, event)
	}
	return events, nil
}

func (s *AlertDBStore) CreateNotification(n *Notification) error {
	return errors.New("legacy notification persistence is retired; use canonical notification policies and jobs")
}

func (s *AlertDBStore) ListNotifications(eventID string) ([]Notification, error) {
	return []Notification{}, nil
}

func (s *AlertDBStore) Migrate() error {
	var canonical bool
	if err := s.db.QueryRow(`SELECT to_regclass('alert_rules') IS NOT NULL
		AND to_regclass('alert_instances') IS NOT NULL
		AND to_regclass('notification_policies') IS NOT NULL
		AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='alert_rules' AND column_name='metric_kind')`).Scan(&canonical); err != nil {
		return fmt.Errorf("verify canonical alert migrations: %w", err)
	}
	if !canonical {
		return errors.New("canonical alert migrations are not installed")
	}
	return nil
}

var _ AlertStore = (*AlertDBStore)(nil)

type durationScanner struct{ value *string }

func newDurationScanner(value *string) durationScanner { return durationScanner{value: value} }

func (s durationScanner) Scan(src any) error {
	var seconds float64
	switch value := src.(type) {
	case float64:
		seconds = value
	case []byte:
		if _, err := fmt.Sscan(string(value), &seconds); err != nil {
			return err
		}
	case string:
		if _, err := fmt.Sscan(value, &seconds); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported duration seconds type %T", src)
	}
	*s.value = time.Duration(seconds * float64(time.Second)).String()
	return nil
}

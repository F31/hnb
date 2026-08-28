package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrMetricUnavailable = errors.New("storage metric is unavailable")
	ErrMetricStale       = errors.New("storage metric is stale")
)

type ResourceReference struct {
	TenantID  string `json:"tenantId"`
	TargetID  string `json:"targetId"`
	Kind      string `json:"kind"`
	UID       string `json:"uid"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type SecretReference struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

type ChannelReference struct {
	Type            string          `json:"type"`
	ConfigReference string          `json:"configReference"`
	SecretReference SecretReference `json:"secretReference"`
}

type StorageMetricCondition struct {
	ProviderID string        `json:"providerId"`
	Kind       string        `json:"kind"`
	Unit       string        `json:"unit"`
	Source     string        `json:"source"`
	FreshFor   time.Duration `json:"-"`
	Operator   string        `json:"operator"`
	Threshold  float64       `json:"threshold"`
}

type StorageAlertContext struct {
	BindingID     string `json:"bindingId,omitempty"`
	OfferingID    string `json:"offeringId,omitempty"`
	OperationID   string `json:"operationId,omitempty"`
	RunbookRef    string `json:"runbookRef,omitempty"`
	NavigationRef string `json:"navigationRef,omitempty"`
}

type StorageAlertRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Severity    Severity               `json:"severity"`
	Enabled     bool                   `json:"enabled"`
	Resource    ResourceReference      `json:"resource"`
	Metric      StorageMetricCondition `json:"metric"`
	Context     StorageAlertContext    `json:"context,omitempty"`
	Duration    time.Duration          `json:"-"`
	Channels    []ChannelReference     `json:"channels,omitempty"`
	Version     int                    `json:"version"`
}

type StorageRuleStore interface {
	ValidateStorageMetric(context.Context, ResourceReference, StorageMetricCondition) error
	ValidateChannelReferences(context.Context, string, []ChannelReference) error
	CreateStorageRule(context.Context, StorageAlertRule) error
	ListStorageRules(context.Context, string) ([]StorageAlertRule, error)
	EvaluateStorageRules(context.Context, time.Time) error
}

func (s *AlertDBStore) ValidateChannelReferences(ctx context.Context, tenantID string, channels []ChannelReference) error {
	for _, channel := range channels {
		ref := channel.SecretReference
		var exists bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM secret_references sr
			JOIN kms_providers kp ON kp.id=sr.kms_provider_id AND kp.is_active
			WHERE sr.tenant_id=$1 AND kp.name=$2 AND sr.scope=$3 AND sr.name=$4
			  AND sr.is_active AND ($5='' OR sr.version::text=$5)
			  AND (sr.expires_at IS NULL OR sr.expires_at>now())
		)`, tenantID, ref.Provider, ref.Scope, ref.Name, ref.Version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("validate channel SecretReference: %w", err)
		}
		if !exists {
			return fmt.Errorf("%w: channel SecretReference", ErrMetricUnavailable)
		}
	}
	return nil
}

func (s *AlertDBStore) ValidateStorageMetric(ctx context.Context, ref ResourceReference, metric StorageMetricCondition) error {
	var applicability, freshness, status, unit, source string
	var observedAt, staleAfter time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT metric->>'applicability', metric->>'freshness', metric->>'status',
		       metric->>'unit', metric->>'source', (metric->>'observedAt')::timestamptz, snapshot.stale_after
		FROM storage_metric_snapshots snapshot
		CROSS JOIN LATERAL jsonb_array_elements(snapshot.metrics) metric
		WHERE snapshot.tenant_id=$1 AND snapshot.target_id=$2 AND snapshot.provider_id=$3
		  AND snapshot.resource_kind=$4 AND snapshot.resource_uid=$5 AND metric->>'kind'=$6`,
		ref.TenantID, ref.TargetID, metric.ProviderID, ref.Kind, ref.UID, metric.Kind).
		Scan(&applicability, &freshness, &status, &unit, &source, &observedAt, &staleAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMetricUnavailable
	}
	if err != nil {
		return fmt.Errorf("validate storage metric: %w", err)
	}
	if applicability != "Applicable" || status != "Known" || unit != metric.Unit || source != metric.Source {
		return ErrMetricUnavailable
	}
	if freshness != "Fresh" || time.Now().UTC().After(staleAfter) || time.Now().UTC().After(observedAt.Add(metric.FreshFor)) {
		return ErrMetricStale
	}
	return nil
}

func (s *AlertDBStore) CreateStorageRule(ctx context.Context, rule StorageAlertRule) error {
	channels, err := json.Marshal(rule.Channels)
	if err != nil {
		return err
	}
	annotations, err := json.Marshal(rule.Context)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (
			id, tenant_scope, tenant_id, name, description, source_type, severity, enabled,
			target_id, resource_kind, resource_uid, resource_namespace, resource_name,
			provider_id, metric_kind, metric_unit, metric_source, metric_fresh_for,
			comparison_operator, threshold, duration, channel_refs, annotations)
		VALUES ($1, 'tenant', $2, $3, NULLIF($4,''), 'storage-metric', $5, $6,
			$7, $8, $9, NULLIF($10,''), NULLIF($11,''), $12, $13, $14, $15,
			$16 * interval '1 second', $17, $18, $19 * interval '1 second', $20, $21)`,
		rule.ID, rule.Resource.TenantID, rule.Name, rule.Description, rule.Severity, rule.Enabled,
		rule.Resource.TargetID, rule.Resource.Kind, rule.Resource.UID, rule.Resource.Namespace, rule.Resource.Name,
		rule.Metric.ProviderID, rule.Metric.Kind, rule.Metric.Unit, rule.Metric.Source, rule.Metric.FreshFor.Seconds(),
		rule.Metric.Operator, rule.Metric.Threshold, rule.Duration.Seconds(), channels, annotations)
	return err
}

func (s *AlertDBStore) ListStorageRules(ctx context.Context, tenantID string) ([]StorageAlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(description,''), severity, enabled, tenant_id, target_id,
		       resource_kind, resource_uid, COALESCE(resource_namespace,''), COALESCE(resource_name,''),
		       provider_id, metric_kind, metric_unit, metric_source, EXTRACT(EPOCH FROM metric_fresh_for),
		       comparison_operator, threshold, EXTRACT(EPOCH FROM duration), channel_refs, annotations, version
		FROM alert_rules WHERE source_type='storage-metric' AND tenant_id=$1 ORDER BY name, id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]StorageAlertRule, 0)
	for rows.Next() {
		var rule StorageAlertRule
		var channels, annotations []byte
		var freshForSeconds, durationSeconds float64
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Description, &rule.Severity, &rule.Enabled,
			&rule.Resource.TenantID, &rule.Resource.TargetID, &rule.Resource.Kind, &rule.Resource.UID,
			&rule.Resource.Namespace, &rule.Resource.Name, &rule.Metric.ProviderID, &rule.Metric.Kind,
			&rule.Metric.Unit, &rule.Metric.Source, &freshForSeconds, &rule.Metric.Operator,
			&rule.Metric.Threshold, &durationSeconds, &channels, &annotations, &rule.Version); err != nil {
			return nil, err
		}
		rule.Metric.FreshFor = time.Duration(freshForSeconds * float64(time.Second))
		rule.Duration = time.Duration(durationSeconds * float64(time.Second))
		if err := json.Unmarshal(channels, &rule.Channels); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(annotations, &rule.Context); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *AlertDBStore) EvaluateStorageRules(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_instances (
			tenant_id, rule_id, source, severity, resource_ref, fingerprint, state, summary,
			first_seen_at, last_seen_at, operation_id, runbook_ref, source_ref, target_id, resource_kind,
			resource_uid, resource_namespace, resource_name, binding_id, offering_id)
		SELECT r.tenant_id, r.id, 'storage-metric', r.severity,
		       jsonb_strip_nulls(jsonb_build_object('tenantId',r.tenant_id,'targetId',r.target_id,
		         'kind',r.resource_kind,'uid',r.resource_uid,'namespace',r.resource_namespace,'name',r.resource_name))::text,
		       md5(concat_ws(':',r.tenant_id,r.id,r.target_id,r.resource_kind,r.resource_uid)),
		       'firing', COALESCE(r.name, 'Storage metric threshold exceeded'), $1, $1,
		       NULLIF(r.annotations->>'operationId','')::uuid, r.annotations->>'runbookRef',
		       r.annotations->>'navigationRef', r.target_id, r.resource_kind, r.resource_uid,
		       r.resource_namespace, r.resource_name, NULLIF(r.annotations->>'bindingId','')::uuid,
		       NULLIF(r.annotations->>'offeringId','')::uuid
		FROM alert_rules r
		JOIN storage_metric_snapshots s ON s.tenant_id=r.tenant_id AND s.target_id=r.target_id
		 AND s.provider_id=r.provider_id AND s.resource_kind=r.resource_kind AND s.resource_uid=r.resource_uid
		CROSS JOIN LATERAL jsonb_array_elements(s.metrics) metric
		WHERE r.source_type='storage-metric' AND r.enabled AND s.stale_after > $1
		  AND metric->>'kind'=r.metric_kind AND metric->>'source'=r.metric_source
		  AND metric->>'unit'=r.metric_unit AND metric->>'applicability'='Applicable'
		  AND metric->>'freshness'='Fresh' AND metric->>'status'='Known'
		  AND CASE r.comparison_operator
		    WHEN 'gt' THEN (metric->>'value')::double precision > r.threshold
		    WHEN 'gte' THEN (metric->>'value')::double precision >= r.threshold
		    WHEN 'lt' THEN (metric->>'value')::double precision < r.threshold
		    WHEN 'lte' THEN (metric->>'value')::double precision <= r.threshold END
		ON CONFLICT (tenant_id, fingerprint) WHERE state != 'resolved'
		DO UPDATE SET last_seen_at=EXCLUDED.last_seen_at, occurrence_count=alert_instances.occurrence_count+1,
		              updated_at=now(), resource_ref=EXCLUDED.resource_ref`, now.UTC())
	return err
}

var _ StorageRuleStore = (*AlertDBStore)(nil)

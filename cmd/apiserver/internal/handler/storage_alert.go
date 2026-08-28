package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/alert"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type storageAlertStore interface {
	ValidateStorageMetric(context.Context, alert.ResourceReference, alert.StorageMetricCondition) error
	ValidateChannelReferences(context.Context, string, []alert.ChannelReference) error
	CreateStorageRule(context.Context, alert.StorageAlertRule) error
	ListStorageRules(context.Context, string) ([]alert.StorageAlertRule, error)
}

type StorageAlertHandler struct{ store storageAlertStore }

func NewStorageAlertHandler(store storageAlertStore) *StorageAlertHandler {
	return &StorageAlertHandler{store: store}
}

type storageAlertRuleInput struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Severity    alert.Severity          `json:"severity"`
	Enabled     *bool                   `json:"enabled"`
	Resource    alert.ResourceReference `json:"resource"`
	Metric      struct {
		ProviderID string  `json:"providerId"`
		Kind       string  `json:"kind"`
		Unit       string  `json:"unit"`
		Source     string  `json:"source"`
		FreshFor   string  `json:"freshFor"`
		Operator   string  `json:"operator"`
		Threshold  float64 `json:"threshold"`
	} `json:"metric"`
	Duration string                    `json:"duration"`
	Channels []alert.ChannelReference  `json:"channels,omitempty"`
	Context  alert.StorageAlertContext `json:"context,omitempty"`
}

func (h *StorageAlertHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok || trusted.TenantID == "" {
		writeStorageProblem(w, r, http.StatusUnauthorized, "TRUSTED_TENANT_REQUIRED", "Trusted tenant context is required.")
		return
	}
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); key == "" || len(key) > 128 || strings.ContainsAny(key, "\x00\r\n") {
		writeStorageProblem(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "A valid Idempotency-Key is required for storage writes.")
		return
	}
	var input storageAlertRuleInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_ALERT_RULE", "Storage alert rule input is invalid.")
		return
	}
	if input.Resource.TenantID != "" {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_ALERT_RULE", "tenantId is derived from trusted request context.")
		return
	}
	input.Resource.TenantID = trusted.TenantID
	freshFor, freshErr := time.ParseDuration(input.Metric.FreshFor)
	duration, durationErr := time.ParseDuration(input.Duration)
	rule := alert.StorageAlertRule{
		ID: uuid.NewString(), Name: input.Name, Description: input.Description, Severity: input.Severity,
		Enabled: input.Enabled == nil || *input.Enabled, Resource: input.Resource, Duration: duration, Channels: input.Channels, Context: input.Context, Version: 1,
		Metric: alert.StorageMetricCondition{ProviderID: input.Metric.ProviderID, Kind: input.Metric.Kind, Unit: input.Metric.Unit,
			Source: input.Metric.Source, FreshFor: freshFor, Operator: input.Metric.Operator, Threshold: input.Metric.Threshold},
	}
	if freshErr != nil || durationErr != nil || !validStorageAlertRule(rule) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_STORAGE_ALERT_RULE", "Storage alert rule requires stable resource identity, a canonical metric, and SecretReference channels.")
		return
	}
	if err := h.store.ValidateStorageMetric(r.Context(), rule.Resource, rule.Metric); err != nil {
		if errors.Is(err, alert.ErrMetricUnavailable) || errors.Is(err, alert.ErrMetricStale) {
			writeStorageProblem(w, r, http.StatusUnprocessableEntity, "STORAGE_METRIC_UNAVAILABLE", "The metric is unsupported, unavailable, or stale for this resource.")
			return
		}
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_ALERT_VALIDATION_FAILED", "Storage metric validation failed.")
		return
	}
	if err := h.store.ValidateChannelReferences(r.Context(), trusted.TenantID, rule.Channels); err != nil {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_CHANNEL_SECRET_REFERENCE", "A channel SecretReference is not owned by the tenant or is inactive.")
		return
	}
	if err := h.store.CreateStorageRule(r.Context(), rule); err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_ALERT_CREATE_FAILED", "Storage alert rule could not be saved.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(storageAlertRuleResponse(rule))
}

func (h *StorageAlertHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok || trusted.TenantID == "" {
		writeStorageProblem(w, r, http.StatusUnauthorized, "TRUSTED_TENANT_REQUIRED", "Trusted tenant context is required.")
		return
	}
	rules, err := h.store.ListStorageRules(r.Context(), trusted.TenantID)
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_ALERT_LIST_FAILED", "Storage alert rules could not be read.")
		return
	}
	items := make([]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, storageAlertRuleResponse(rule))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"schemaVersion": "1.0.0", "items": items})
}

func validStorageAlertRule(rule alert.StorageAlertRule) bool {
	if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 256 || rule.Resource.TargetID == "" || rule.Resource.Kind == "" || rule.Resource.UID == "" ||
		rule.Metric.ProviderID == "" || rule.Metric.Kind == "" || rule.Metric.Unit == "" || rule.Metric.Source == "" || rule.Metric.FreshFor <= 0 || rule.Duration <= 0 {
		return false
	}
	if _, err := uuid.Parse(rule.Resource.TargetID); err != nil {
		return false
	}
	if rule.Severity != alert.SeverityCritical && rule.Severity != alert.SeverityWarning && rule.Severity != alert.SeverityInfo {
		return false
	}
	validOperator := rule.Metric.Operator == "gt" || rule.Metric.Operator == "gte" || rule.Metric.Operator == "lt" || rule.Metric.Operator == "lte"
	units := map[string]string{"capacity": "By", "usage": "By", "iops": "1/s", "throughput": "By/s", "latency": "s", "health": "1"}
	if !validOperator || units[rule.Metric.Kind] != rule.Metric.Unit {
		return false
	}
	for _, channel := range rule.Channels {
		ref := channel.SecretReference
		if channel.Type == "" || channel.ConfigReference == "" || ref.Provider == "" || ref.Name == "" || ref.Scope != "tenant:"+rule.Resource.TenantID {
			return false
		}
	}
	for _, id := range []string{rule.Context.BindingID, rule.Context.OfferingID, rule.Context.OperationID} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return false
			}
		}
	}
	return true
}

func storageAlertRuleResponse(rule alert.StorageAlertRule) map[string]any {
	return map[string]any{
		"schemaVersion": "1.0.0", "id": rule.ID, "name": rule.Name, "description": rule.Description,
		"severity": rule.Severity, "enabled": rule.Enabled, "resource": rule.Resource,
		"metric": map[string]any{"providerId": rule.Metric.ProviderID, "kind": rule.Metric.Kind, "unit": rule.Metric.Unit,
			"source": rule.Metric.Source, "freshFor": rule.Metric.FreshFor.String(), "operator": rule.Metric.Operator, "threshold": rule.Metric.Threshold},
		"duration": rule.Duration.String(), "channels": rule.Channels, "context": rule.Context, "version": rule.Version,
	}
}

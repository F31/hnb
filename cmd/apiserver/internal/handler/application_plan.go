package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/F31/hnb/cmd/apiserver/internal/application/deliveryplan"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
)

type ApplicationPlanHandler struct {
	db *sql.DB
}

func NewApplicationPlanHandler(db *sql.DB) *ApplicationPlanHandler {
	return &ApplicationPlanHandler{db: db}
}

func (h *ApplicationPlanHandler) Preview(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(body) == 0 {
		response.BadRequest(w, "composition body is required")
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var composition deliveryplan.Composition
	if err := decoder.Decode(&composition); err != nil {
		response.BadRequest(w, "unknown or invalid composition field: "+err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.BadRequest(w, "composition must contain exactly one document")
		return
	}
	plan, err := deliveryplan.Build(r.Context(), trusted.TenantID, composition, &applicationPlanValidator{db: h.db})
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, response.CodeValidation, err.Error())
		return
	}
	response.Success(w, plan)
}

type applicationPlanValidator struct {
	db *sql.DB
}

func (v *applicationPlanValidator) NamespaceOwned(ctx context.Context, tenantID, namespace string) error {
	var exists bool
	if err := v.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM namespaces WHERE tenant_id=$1 AND name=$2 AND status='active')`,
		tenantID, namespace).Scan(&exists); err != nil {
		return fmt.Errorf("namespace lookup failed")
	}
	if !exists {
		return errors.New("namespace is not owned by the tenant")
	}
	return nil
}

func (v *applicationPlanValidator) SecretReferenceOwned(ctx context.Context, tenantID string, ref deliveryplan.SecretRef) error {
	var exists bool
	if err := v.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM secret_references sr
			JOIN kms_providers kp ON kp.id=sr.kms_provider_id AND kp.is_active
			WHERE sr.tenant_id=$1 AND kp.name=$2 AND sr.scope=$3 AND sr.name=$4
			  AND ($5='' OR sr.version::text=$5) AND sr.is_active
			  AND (sr.expires_at IS NULL OR sr.expires_at>now()))`,
		tenantID, ref.Provider, ref.Scope, ref.Name, ref.Version).Scan(&exists); err != nil {
		return fmt.Errorf("secret reference lookup failed")
	}
	if !exists {
		return errors.New("secret reference is not owned, active, or unexpired")
	}
	return nil
}

func (v *applicationPlanValidator) Quota(ctx context.Context, tenantID, namespace string) (deliveryplan.Quota, error) {
	var raw []byte
	if err := v.db.QueryRowContext(ctx, `SELECT COALESCE(quota,'{}') FROM namespaces WHERE tenant_id=$1 AND name=$2`, tenantID, namespace).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deliveryplan.Quota{}, errors.New("namespace quota is unavailable")
		}
		return deliveryplan.Quota{}, fmt.Errorf("quota lookup failed")
	}
	var quota core.Quota
	if err := json.Unmarshal(raw, &quota); err != nil {
		return deliveryplan.Quota{}, fmt.Errorf("quota is invalid")
	}
	return deliveryplan.Quota{CPU: quota.CPU, Memory: quota.Memory, Storage: quota.Storage}, nil
}

func (v *applicationPlanValidator) Snapshot(ctx context.Context, tenantID, targetID, digest string) (deliveryplan.Snapshot, error) {
	var raw []byte
	if err := v.db.QueryRowContext(ctx, `
		SELECT snapshot_json FROM capability_snapshots
		WHERE tenant_id=$1 AND target_id=$2 AND content_digest=$3
		ORDER BY observed_at DESC LIMIT 1`, tenantID, targetID, digest).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deliveryplan.Snapshot{}, errors.New("fixed capability snapshot was not found for this target")
		}
		return deliveryplan.Snapshot{}, fmt.Errorf("capability snapshot lookup failed")
	}
	var content map[string]any
	if err := json.Unmarshal(raw, &content); err != nil {
		return deliveryplan.Snapshot{}, fmt.Errorf("capability snapshot is invalid")
	}
	return deliveryplan.Snapshot{
		StorageClasses:    stringsFrom(content["storageClasses"]),
		CSIDrivers:        stringsFrom(content["csiDrivers"]),
		GatewayAPIVersion: stringsFrom(content["gatewayApiVersions"]),
	}, nil
}

func stringsFrom(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok && text != "" {
			result = append(result, text)
		}
	}
	return result
}

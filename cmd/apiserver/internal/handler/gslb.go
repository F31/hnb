package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	gslbapp "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

// GSLBHandler 承载 GSLB 受控流量变更（GSLB-005）：
// 意图提交 + 审批 + 拒绝。查询接口（Read Model）见 T5。
type GSLBHandler struct{ app *gslbapp.App }

func NewGSLBHandler(app *gslbapp.App) *GSLBHandler {
	return &GSLBHandler{app: app}
}

// ListServices GET /api/v1/gslb/services（只读 Read Model）
func (h *GSLBHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	models, err := h.app.ListServices(r.Context(), trusted.TenantID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": models, "total": len(models)})
}

// GetService GET /api/v1/gslb/services/{id}（只读 Read Model）
func (h *GSLBHandler) GetService(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	model, err := h.app.GetServiceProjection(r.Context(), r.PathValue("id"), trusted.TenantID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(model)
}

// ListDrills GET /api/v1/gslb/services/{id}/drills（GSLB-010 演练报告，只读）
func (h *GSLBHandler) ListDrills(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	reports, err := h.app.ListDrillReports(r.Context(), r.PathValue("id"), trusted.TenantID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": reports, "total": len(reports)})
}

// ListPools GET /api/v1/gslb/services/{id}/pools（地址池拓扑，只读）
func (h *GSLBHandler) ListPools(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	pools, err := h.app.ListPools(r.Context(), r.PathValue("id"), trusted.TenantID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pools)
}

// CreateService POST /api/v1/gslb/services（GSLB 三期：服务创建，受控写路径）
func (h *GSLBHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var input gslbapp.CreateServiceInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		response.BadRequest(w, "cannot decode create service body")
		return
	}
	detail, created, err := h.app.CreateService(r.Context(), input, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(detail)
}

// ListSwitchRequests GET /api/v1/gslb/switch-requests?service_id=&status=
// （切换记录/审批中心，只读）
func (h *GSLBHandler) ListSwitchRequests(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	serviceID := r.URL.Query().Get("service_id")
	if _, err := uuid.Parse(serviceID); err != nil {
		response.BadRequest(w, "service_id must be a valid UUID")
		return
	}
	requests, err := h.app.ListSwitchRequests(r.Context(), serviceID, r.URL.Query().Get("status"), trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": requests, "total": len(requests)})
}

// GetSwitchRequest GET /api/v1/gslb/switch-requests/{id}
func (h *GSLBHandler) GetSwitchRequest(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	request, err := h.app.GetSwitchRequest(r.Context(), r.PathValue("id"), trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(request)
}

// SubmitIntent POST /api/v1/gslb/services/{id}/intents
func (h *GSLBHandler) SubmitIntent(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	serviceID := r.PathValue("id")
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		response.BadRequest(w, "cannot read intent body")
		return
	}
	request, err := h.app.SubmitIntent(r.Context(), body, serviceID, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(request)
}

// Approve POST /api/v1/gslb/switch-requests/{id}/approve
func (h *GSLBHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, true)
}

// Reject POST /api/v1/gslb/switch-requests/{id}/reject
func (h *GSLBHandler) Reject(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, false)
}

func (h *GSLBHandler) transition(w http.ResponseWriter, r *http.Request, approve bool) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	requestID := r.PathValue("id")
	var request gslbapp.SwitchRequest
	var err error
	if approve {
		request, err = h.app.Approve(r.Context(), requestID, trusted)
	} else {
		request, err = h.app.Reject(r.Context(), requestID, trusted)
	}
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(request)
}

// UpdateService PATCH /api/v1/gslb/services/{id}（GSLB 四期：服务编辑）
// 可变字段：name/requireApproval/healthCheck；If-Match 携带 updatedAt 做乐观并发。
func (h *GSLBHandler) UpdateService(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var input gslbapp.UpdateServiceInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		response.BadRequest(w, "cannot decode update service body")
		return
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		response.BadRequest(w, "If-Match must be an RFC3339 updatedAt value or *")
		return
	}
	detail, err := h.app.UpdateService(r.Context(), r.PathValue("id"), input, ifMatch, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

// ReplacePools PUT /api/v1/gslb/services/{id}/pools（GSLB 四期：地址池整组替换）
func (h *GSLBHandler) ReplacePools(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var input gslbapp.ReplacePoolsInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		response.BadRequest(w, "cannot decode replace pools body")
		return
	}
	ifMatch, err := parseIfMatch(r)
	if err != nil {
		response.BadRequest(w, "If-Match must be an RFC3339 updatedAt value or *")
		return
	}
	pools, err := h.app.ReplaceServicePools(r.Context(), r.PathValue("id"), input, ifMatch, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": pools, "total": len(pools)})
}

// DeleteService DELETE /api/v1/gslb/services/{id}?force=（GSLB 四期：危险操作，
// writeGate + gslb:delete；Inactive/Paused 可删，其余状态需 force=true）
func (h *GSLBHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	force := r.URL.Query().Get("force") == "true"
	if err := h.app.DeleteService(r.Context(), r.PathValue("id"), force, trusted); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseIfMatch 解析 If-Match 头：值为服务当前 updatedAt（RFC3339）或 *（跳过版本比对）。
func parseIfMatch(r *http.Request) (*time.Time, error) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" || raw == "*" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.Trim(raw, `"`))
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}

func (h *GSLBHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gslbapp.ErrInvalid):
		response.BadRequest(w, err.Error())
	case errors.Is(err, gslbapp.ErrNotFound):
		response.NotFound(w, "gslb resource not found")
	case errors.Is(err, gslbapp.ErrForbidden):
		response.Forbidden(w, "forbidden")
	case errors.Is(err, gslbapp.ErrConflict):
		response.Error(w, http.StatusConflict, response.CodeConflict, err.Error())
	default:
		response.InternalError(w, err.Error())
	}
}

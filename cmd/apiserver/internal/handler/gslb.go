package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	gslbapp "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
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

func (h *GSLBHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gslbapp.ErrInvalid):
		response.BadRequest(w, err.Error())
	case errors.Is(err, gslbapp.ErrNotFound):
		response.NotFound(w, "gslb resource not found")
	case errors.Is(err, gslbapp.ErrForbidden):
		response.Forbidden(w, "forbidden")
	default:
		response.InternalError(w, err.Error())
	}
}

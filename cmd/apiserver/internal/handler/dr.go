package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	drapp "github.com/F31/hnb/cmd/apiserver/internal/application/dr"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

// DRHandler 承载 DRProtectionGroup 容灾编排（OBS-008）：
// 保护组/成员管理 + 切换链发起 + 数据层确认门。
type DRHandler struct{ app *drapp.App }

func NewDRHandler(app *drapp.App) *DRHandler {
	return &DRHandler{app: app}
}

// ListGroups GET /api/v1/dr/groups
func (h *DRHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	groups, err := h.app.ListGroups(r.Context(), trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": groups, "total": len(groups)})
}

// CreateGroup POST /api/v1/dr/groups
func (h *DRHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var body struct {
		Name          string `json:"name"`
		PrimaryRegion string `json:"primaryRegion"`
		StandbyRegion string `json:"standbyRegion"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		response.BadRequest(w, "invalid group body")
		return
	}
	group, err := h.app.CreateGroup(r.Context(), body.Name, body.PrimaryRegion, body.StandbyRegion, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(group)
}

// GetGroup GET /api/v1/dr/groups/{id}（组详情：成员 + 最近运行）
func (h *DRHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	detail, err := h.app.GetGroup(r.Context(), r.PathValue("id"), trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

// AddMember POST /api/v1/dr/groups/{id}/members
func (h *DRHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var body struct {
		MemberType string `json:"memberType"`
		RefID      string `json:"refId"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		response.BadRequest(w, "invalid member body")
		return
	}
	member, err := h.app.AddMember(r.Context(), r.PathValue("id"), body.MemberType, body.RefID, body.Name, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(member)
}

// ListRuns GET /api/v1/dr/groups/{id}/runs
func (h *DRHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	runs, err := h.app.ListRuns(r.Context(), r.PathValue("id"), trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": runs, "total": len(runs)})
}

// InitiateSwitch POST /api/v1/dr/groups/{id}/switch
func (h *DRHandler) InitiateSwitch(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var body struct {
		Direction      string `json:"direction"`
		Reason         string `json:"reason"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		response.BadRequest(w, "invalid switch body")
		return
	}
	idempotencyKey := strings.TrimSpace(body.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	run, err := h.app.InitiateSwitch(r.Context(), r.PathValue("id"), body.Direction, body.Reason, idempotencyKey, trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(run)
}

// ConfirmDataLayer POST /api/v1/dr/runs/{id}/confirm-data-layer
func (h *DRHandler) ConfirmDataLayer(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	run, err := h.app.ConfirmDataLayer(r.Context(), r.PathValue("id"), trusted)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(run)
}

func (h *DRHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, drapp.ErrInvalid):
		response.BadRequest(w, err.Error())
	case errors.Is(err, drapp.ErrNotFound):
		response.NotFound(w, "dr resource not found")
	case errors.Is(err, drapp.ErrForbidden):
		response.Forbidden(w, "forbidden")
	case errors.Is(err, drapp.ErrConflict):
		response.Error(w, http.StatusConflict, response.CodeConflict, err.Error())
	default:
		response.InternalError(w, err.Error())
	}
}

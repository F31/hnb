package handler

import (
	"net/http"
	"strconv"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/audit"
	"github.com/F31/hnb/pkg/iam"
)

type AuditHandler struct {
	store *audit.Store
}

func NewAuditHandler(store *audit.Store) *AuditHandler {
	return &AuditHandler{store: store}
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	filters := make(map[string]string)
	for _, key := range []string{"user_id", "action", "resource_type"} {
		if v := r.URL.Query().Get(key); v != "" {
			filters[key] = v
		}
	}
	filters["tenant_id"] = trusted.TenantID
	events, err := h.store.List(limit, offset, filters)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, events)
}

func (h *AuditHandler) Get(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	id := r.PathValue("id")
	event, err := h.store.Get(id, trusted.TenantID)
	if err != nil {
		response.NotFound(w, "audit log not found")
		return
	}
	response.Success(w, event)
}

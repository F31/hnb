package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type ClusterHandler struct {
	db          *sql.DB
	platformURL string
	client      *http.Client
}

func NewClusterHandler(db *sql.DB) *ClusterHandler { return &ClusterHandler{db: db} }

func NewPlatformClusterHandler(platformURL string) *ClusterHandler {
	return &ClusterHandler{platformURL: strings.TrimRight(platformURL, "/"), client: newInternalHTTPClient(30 * time.Second)}
}

func (h *ClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.platformURL != "" {
		h.forwardPlatform(w, r, http.MethodGet, "/v1/clusters")
		return
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	tenantID := trusted.TenantID
	rows, err := h.db.Query(`
		SELECT DISTINCT rt.id, rt.tenant_id, rt.name, rt.display_name, rt.target_type, rt.distribution, rt.connection_type, rt.status, rt.labels, rt.is_active, rt.created_at
		FROM runtime_targets rt
		LEFT JOIN cluster_shares cs ON cs.cluster_id = rt.id AND cs.grantee_tenant_id=$1
		WHERE rt.tenant_id=$1 OR rt.tenant_id IS NULL OR cs.id IS NOT NULL
		ORDER BY rt.created_at DESC`, tenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer rows.Close()
	var targets []core.RuntimeTarget
	for rows.Next() {
		var rt core.RuntimeTarget
		var dn, lb sql.NullString
		if err := rows.Scan(&rt.ID, &rt.TenantID, &rt.Name, &dn, &rt.TargetType, &rt.Distribution, &rt.ConnectionType, &rt.Status, &lb, &rt.IsActive, &rt.CreatedAt); err != nil {
			continue
		}
		if dn.Valid {
			rt.DisplayName = dn.String
		}
		if lb.Valid {
			json.Unmarshal([]byte(lb.String), &rt.Labels)
		}
		targets = append(targets, rt)
	}
	if targets == nil {
		targets = []core.RuntimeTarget{}
	}
	response.Success(w, targets)
}

func (h *ClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h.platformURL != "" {
		h.forwardPlatform(w, r, http.MethodGet, "/v1/clusters/"+r.PathValue("id"))
		return
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	id := r.PathValue("id")
	if !clusterAccessibleTo(h.db, id, trusted.TenantID, "") {
		response.NotFound(w, "cluster not found")
		return
	}
	var rt core.RuntimeTarget
	var dn, lb sql.NullString
	err := h.db.QueryRow(`SELECT id, tenant_id, name, display_name, target_type, distribution, connection_type, status, labels, is_active, created_at FROM runtime_targets WHERE id=$1`, id).
		Scan(&rt.ID, &rt.TenantID, &rt.Name, &dn, &rt.TargetType, &rt.Distribution, &rt.ConnectionType, &rt.Status, &lb, &rt.IsActive, &rt.CreatedAt)
	if err != nil {
		response.NotFound(w, "cluster not found")
		return
	}
	if dn.Valid {
		rt.DisplayName = dn.String
	}
	if lb.Valid {
		json.Unmarshal([]byte(lb.String), &rt.Labels)
	}
	response.Success(w, rt)
}

func (h *ClusterHandler) Register(w http.ResponseWriter, r *http.Request) {
	if h.platformURL != "" {
		h.forwardPlatform(w, r, http.MethodPost, "/v1/clusters")
		return
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	tenantID := trusted.TenantID
	var req struct {
		Name           string `json:"name"`
		TargetType     string `json:"target_type"`
		ConnectionType string `json:"connection_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	rt := core.RuntimeTarget{
		ID: uuid.NewString(), TenantID: tenantID, Name: req.Name,
		TargetType: core.TargetType(req.TargetType), ConnectionType: core.ConnectionType(req.ConnectionType),
		Status: core.StatusUnknown, IsActive: true, CreatedAt: time.Now().UTC(),
	}
	_, err := h.db.Exec(`INSERT INTO runtime_targets (id, tenant_id, name, target_type, connection_type, status, is_active, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		rt.ID, rt.TenantID, rt.Name, string(rt.TargetType), string(rt.ConnectionType), string(rt.Status), rt.IsActive, rt.CreatedAt)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, rt)
}

func (h *ClusterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.platformURL != "" {
		h.forwardPlatform(w, r, http.MethodDelete, "/v1/clusters/"+r.PathValue("id"))
		return
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	id := r.PathValue("id")
	result, err := h.db.Exec(`DELETE FROM runtime_targets WHERE id=$1 AND tenant_id=$2`, id, trusted.TenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		response.NotFound(w, "cluster not found")
		return
	}
	response.Success(w, map[string]string{"status": "deleted"})
}

func (h *ClusterHandler) forwardPlatform(w http.ResponseWriter, r *http.Request, method, path string) {
	var body io.Reader
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			response.BadRequest(w, "invalid body")
			return
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, h.platformURL+path, body)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	copyHeader(req.Header, r.Header, "Authorization")
	copyHeader(req.Header, r.Header, "X-Tenant-ID")
	copyHeader(req.Header, r.Header, "X-Space-ID")
	copyHeader(req.Header, r.Header, "X-Environment-ID")
	copyHeader(req.Header, r.Header, "X-Trace-Id")
	copyHeader(req.Header, r.Header, "X-Correlation-ID")
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		response.Error(w, http.StatusBadGateway, response.CodeServiceUnavailable, "platform-api unavailable")
		return
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func copyHeader(dst, src http.Header, key string) {
	if value := src.Get(key); value != "" {
		dst.Set(key, value)
	}
}

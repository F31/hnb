package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

// pluginCatalogItem is the shape of a plugin catalog entry returned to the
// console. It is consumed by the PluginMarketPage (资源 > 插件市场).
type pluginCatalogItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Kind        string `json:"kind"`
	Provider    string `json:"provider,omitempty"`
	Installed   bool   `json:"installed"`
}

// marketProduct mirrors the relevant fields of app-market GET /products response.
type marketProduct struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// PluginCatalogHandler serves the platform-wide plugin catalog aggregated with
// per-cluster installed state. The catalog itself lives in app-market products
// (hnb-official, public visibility); installed state comes from the extensions
// table scoped to the selected target cluster.
type PluginCatalogHandler struct {
	db        *sql.DB
	marketURL string
	client    *http.Client
}

// NewPluginCatalogHandler builds a catalog handler that reads products from
// app-market and merges installed extensions for a target cluster.
func NewPluginCatalogHandler(db *sql.DB, marketURL string) *PluginCatalogHandler {
	return &PluginCatalogHandler{
		db:        db,
		marketURL: strings.TrimRight(marketURL, "/"),
		client:    newInternalHTTPClient(30 * time.Second),
	}
}

// List returns the plugin catalog with per-cluster installed state.
//   - GET /api/v1/plugin-catalog?clusterId={id}
//
// The catalog is platform-wide (app-market public products labelled plugin=true);
// installed flags are derived from extensions rows bound to the cluster.
func (h *PluginCatalogHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.marketURL == "" {
		response.Error(w, http.StatusBadGateway, response.CodeServiceUnavailable, "app-market unavailable")
		return
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}

	catalog, err := h.fetchCatalog(r)
	if err != nil {
		response.Error(w, http.StatusBadGateway, response.CodeServiceUnavailable, err.Error())
		return
	}

	clusterID := r.URL.Query().Get("clusterId")
	installed, err := h.installedExtensions(r, trusted.TenantID, clusterID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}

	items := make([]pluginCatalogItem, 0, len(catalog))
	for _, p := range catalog {
		items = append(items, pluginCatalogItem{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			// version is carried on the product label (kept in sync with the
			// product's latest published release by the app-market seed)
			Version:     p.Labels["plugin.version"],
			Description: p.Description,
			Category:    p.Labels["plugin.category"],
			Kind:        p.Labels["plugin.kind"],
			Provider:    p.Labels["plugin.provider"],
			Installed:   containsString(installed, p.Name),
		})
	}
	writeJSONRaw(w, items)
}

type marketProductWithVersion struct {
	marketProduct
	ReleaseCount int `json:"release_count"`
}

// fetchCatalog pulls public plugin products from app-market, resolving the
// latest published release version per product via its labels/release query.
func (h *PluginCatalogHandler) fetchCatalog(r *http.Request) ([]marketProductWithVersion, error) {
	token, ok := iam.RawAccessTokenFrom(r.Context())
	if !ok {
		return nil, fmt.Errorf("access token required")
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		h.marketURL+"/api/v1/products?scope=public&pageSize=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	copyHeader(req.Header, r.Header, "X-Trace-Id")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("app-market reachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("app-market returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Items []marketProductWithVersion `json:"items"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	var catalog []marketProductWithVersion
	for _, p := range envelope.Items {
		if p.Labels["plugin"] == "true" {
			catalog = append(catalog, p)
		}
	}
	return catalog, nil
}

// installedExtensions returns the names of extensions bound to a target cluster.
func (h *PluginCatalogHandler) installedExtensions(r *http.Request, tenantID, clusterID string) ([]string, error) {
	if clusterID == "" {
		return nil, nil
	}
	rows, err := h.db.Query(`SELECT e.name
		FROM extensions e
		JOIN runtime_targets rt ON rt.id::text = COALESCE(e.runtime_target_id::text, e.target_id)
		WHERE rt.id::text = $1 AND rt.tenant_id = $2
		  AND e.phase IN ('installing','ready','degraded')`, clusterID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func containsString(hay []string, target string) bool {
	for _, s := range hay {
		if s == target {
			return true
		}
	}
	return false
}

// Install creates an extension record for the plugin bound to a target cluster.
// The extension-controller observes pending extensions and drives the actual
// addon deployment. Request: POST /api/v1/plugin-catalog/installs
// body: {"name": "...", "version": "...", "clusterId": "..."}
func (h *PluginCatalogHandler) Install(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var req struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		ClusterID string `json:"clusterId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Name == "" || req.ClusterID == "" {
		response.BadRequest(w, "name and clusterId are required")
		return
	}
	// The target cluster must belong to the caller's tenant.
	var kindLabel sql.NullString
	err := h.db.QueryRow(`SELECT rt.target_type
		FROM runtime_targets rt
		WHERE rt.id::text=$1 AND rt.tenant_id=$2`, req.ClusterID, trusted.TenantID).
		Scan(&kindLabel)
	if err != nil {
		response.NotFound(w, "cluster not found in tenant")
		return
	}

	extID := uuid.NewString()
	manifest := map[string]any{
		"name": req.Name, "version": req.Version,
		"provider": kindLabel.String,
	}
	mJSON, _ := json.Marshal(manifest)
	if _, err := h.db.Exec(`INSERT INTO extensions (id, name, version, provider_type, target_id, phase, manifest, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,'pending',$6,NOW(),NOW())`,
		extID, req.Name, req.Version, kindLabel.String, req.ClusterID, string(mJSON)); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, map[string]string{"id": extID, "name": req.Name, "status": "pending"})
}

// Uninstall removes the extension bound to (name, clusterId).
// Request: DELETE /api/v1/plugin-catalog/installs/{name}?clusterId={id}
func (h *PluginCatalogHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	name := r.PathValue("name")
	clusterID := r.URL.Query().Get("clusterId")
	if name == "" || clusterID == "" {
		response.BadRequest(w, "name and clusterId are required")
		return
	}
	result, err := h.db.Exec(`DELETE FROM extensions e
		WHERE e.name=$1 AND e.target_id=$2 AND EXISTS (
			SELECT 1 FROM runtime_targets rt WHERE rt.id::text=$2 AND rt.tenant_id=$3
		)`, name, clusterID, trusted.TenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		response.NotFound(w, "plugin not installed on cluster")
		return
	}
	response.Success(w, map[string]string{"name": name, "status": "uninstalled"})
}

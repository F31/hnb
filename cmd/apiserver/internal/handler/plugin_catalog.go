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
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Version      string `json:"version"`
	Description  string `json:"description"`
	Category     string `json:"category"`
	Kind         string `json:"kind"`
	Provider     string `json:"provider,omitempty"`
	InstallPhase string `json:"installPhase,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	Installed    bool   `json:"installed"`
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
	installed, err := h.installedExtensionStatuses(r, trusted.TenantID, clusterID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}

	items := make([]pluginCatalogItem, 0, len(catalog))
	seen := make(map[string]int, len(catalog))
	for _, p := range catalog {
		item := pluginCatalogItem{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			// version is carried on the product label (kept in sync with the
			// product's latest published release by the app-market seed)
			Version:      p.Labels["plugin.version"],
			Description:  p.Description,
			Category:     p.Labels["plugin.category"],
			Kind:         p.Labels["plugin.kind"],
			Provider:     p.Labels["plugin.provider"],
			InstallPhase: installed[p.Name].Phase,
			LastError:    installed[p.Name].LastError,
			Installed:    installed[p.Name].Phase == "ready",
		}
		if idx, ok := seen[p.Name]; ok {
			if items[idx].Version == "" && item.Version != "" {
				items[idx] = item
			}
			if item.InstallPhase != "" && phaseRank(item.InstallPhase) > phaseRank(items[idx].InstallPhase) {
				items[idx].InstallPhase = item.InstallPhase
				items[idx].LastError = item.LastError
				items[idx].Installed = item.Installed
			}
			continue
		}
		seen[p.Name] = len(items)
		items = append(items, item)
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

// installedExtensions returns extension lifecycle states on a target cluster.
// Degraded extensions are reported so the UI can show retry; uninstalling is
// reported so DELETE remains visibly asynchronous until the worker confirms.
type extensionStatus struct {
	Phase     string
	LastError string
}

func (h *PluginCatalogHandler) installedExtensionStatuses(r *http.Request, tenantID, clusterID string) (map[string]extensionStatus, error) {
	if clusterID == "" {
		return map[string]extensionStatus{}, nil
	}
	rows, err := h.db.Query(`SELECT e.name, e.phase, COALESCE(e.last_error, '')
		FROM extensions e
		JOIN runtime_targets rt ON rt.id::text = COALESCE(e.runtime_target_id::text, e.target_id)
		WHERE rt.id::text = $1 AND rt.tenant_id = $2
		  AND e.phase IN ('pending','installing','ready','degraded','uninstalling')`, clusterID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := map[string]extensionStatus{}
	for rows.Next() {
		var name, phase, lastError string
		if err := rows.Scan(&name, &phase, &lastError); err != nil {
			return nil, err
		}
		if phaseRank(phase) > phaseRank(statuses[name].Phase) {
			statuses[name] = extensionStatus{Phase: phase, LastError: lastError}
		}
	}
	return statuses, rows.Err()
}

func phaseRank(phase string) int {
	switch phase {
	case "ready":
		return 3
	case "installing":
		return 2
	case "uninstalling":
		return 2
	case "pending":
		return 1
	case "degraded":
		return 1
	default:
		return 0
	}
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
	var targetType string
	err := h.db.QueryRow(`SELECT rt.target_type
		FROM runtime_targets rt
		WHERE rt.id::text=$1 AND rt.tenant_id=$2`, req.ClusterID, trusted.TenantID).Scan(&targetType)
	if err != nil {
		response.NotFound(w, "cluster not found in tenant")
		return
	}
	_ = targetType

	var existingID, existingPhase string
	err = h.db.QueryRow(`SELECT e.id::text, e.phase
		FROM extensions e
		WHERE e.name=$1 AND COALESCE(e.runtime_target_id::text, e.target_id)=$2
		  AND e.phase IN ('pending','installing','ready','uninstalling')
		ORDER BY CASE e.phase WHEN 'ready' THEN 3 WHEN 'installing' THEN 2 WHEN 'uninstalling' THEN 2 WHEN 'pending' THEN 1 ELSE 0 END DESC, e.updated_at DESC
		LIMIT 1`, req.Name, req.ClusterID).Scan(&existingID, &existingPhase)
	if err == nil {
		response.Success(w, map[string]string{"id": existingID, "name": req.Name, "status": existingPhase})
		return
	}
	if err != sql.ErrNoRows {
		response.InternalError(w, err.Error())
		return
	}

	// Resolve the plugin's provider/key from the catalog so the extension is
	// routed to the right provider worker (hnb.extension.provider.<provider>).
	provider := "plugin"
	labels := map[string]string{}
	if h.marketURL != "" {
		if catalog, err := h.fetchCatalog(r); err == nil {
			for _, p := range catalog {
				if p.Name != req.Name {
					continue
				}
				labels = p.Labels
				if v := p.Labels["plugin.provider"]; v != "" {
					provider = v
				}
				if req.Version == "" {
					req.Version = p.Labels["plugin.version"]
				}
				break
			}
		}
	}
	manifest := map[string]any{
		"name": req.Name, "version": req.Version,
		"provider": provider,
	}
	if helmConfig := helmConfigFromLabels(labels); len(helmConfig) > 0 {
		manifest["config"] = map[string]any{"helm": helmConfig}
	}
	mJSON, _ := json.Marshal(manifest)

	var retriedID string
	err = h.db.QueryRow(`UPDATE extensions
		SET version=$3, provider_type=$4, phase='pending', manifest=$5, health_failures=0, last_error='', updated_at=NOW()
		WHERE id = (
			SELECT e.id FROM extensions e
			WHERE e.name=$1 AND COALESCE(e.runtime_target_id::text, e.target_id)=$2 AND e.phase='degraded'
			ORDER BY e.updated_at DESC LIMIT 1
		)
		RETURNING id::text`, req.Name, req.ClusterID, req.Version, provider, string(mJSON)).Scan(&retriedID)
	if err == nil {
		response.Success(w, map[string]string{"id": retriedID, "name": req.Name, "status": "pending"})
		return
	}
	if err != sql.ErrNoRows {
		response.InternalError(w, err.Error())
		return
	}

	extID := uuid.NewString()
	if _, err := h.db.Exec(`INSERT INTO extensions (id, name, version, provider_type, target_id, phase, manifest, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,'pending',$6,NOW(),NOW())`,
		extID, req.Name, req.Version, provider, req.ClusterID, string(mJSON)); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, map[string]string{"id": extID, "name": req.Name, "status": "pending"})
}

func helmConfigFromLabels(labels map[string]string) map[string]any {
	if len(labels) == 0 {
		return nil
	}
	config := map[string]any{}
	for label, key := range map[string]string{
		"plugin.helm.chart":        "chart",
		"plugin.helm.chartVersion": "chartVersion",
		"plugin.helm.repoName":     "repoName",
		"plugin.helm.repoURL":      "repoURL",
		"plugin.helm.namespace":    "namespace",
	} {
		if value := strings.TrimSpace(labels[label]); value != "" {
			config[key] = value
		}
	}
	if value := strings.TrimSpace(labels["plugin.helm.preflight"]); value != "" {
		var preflight map[string]any
		if err := json.Unmarshal([]byte(value), &preflight); err == nil && len(preflight) > 0 {
			config["preflight"] = preflight
		}
	}
	return config
}

// Uninstall marks the extension bound to (name, clusterId) as uninstalling.
// The extension reconciler performs the provider uninstall and deletes the row
// only after the provider confirms success.
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
	result, err := h.db.Exec(`UPDATE extensions e
		SET phase='uninstalling', last_error=NULL, updated_at=NOW()
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
	response.Success(w, map[string]string{"name": name, "status": "uninstalling"})
}

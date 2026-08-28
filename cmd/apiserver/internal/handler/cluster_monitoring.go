package handler

// Cluster monitoring is a read-only BFF over the central Prometheus-compatible
// query API. Metrics are collected in each target cluster by Prometheus Agent
// or Prometheus Operator and remote-written to the central store; this handler
// never accepts samples and never exposes arbitrary PromQL from a browser.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

type ClusterMonitoringHandler struct {
	db      *sql.DB
	baseURL string
	client  *http.Client
}

func NewClusterMonitoringHandler(db *sql.DB, prometheusURL string) *ClusterMonitoringHandler {
	return &ClusterMonitoringHandler{
		db:      db,
		baseURL: strings.TrimRight(strings.TrimSpace(prometheusURL), "/"),
		client:  newInternalHTTPClient(10 * time.Second),
	}
}

type monitoringResource struct {
	Total             float64 `json:"total"`
	UsagePercent      float64 `json:"usagePercent"`
	Used              float64 `json:"used"`
	AllocationPercent float64 `json:"allocationPercent"`
	Allocated         float64 `json:"allocated"`
	OvercommitPercent float64 `json:"overcommitPercent"`
	Overcommitted     float64 `json:"overcommitted"`
}

type monitoringSummary struct {
	Alerts struct {
		Critical int `json:"critical"`
		Major    int `json:"major"`
		Minor    int `json:"minor"`
		Warning  int `json:"warning"`
		Event    int `json:"event"`
	} `json:"alerts"`
	NamespaceCount       int                `json:"namespaceCount"`
	ProjectCount         int                `json:"projectCount"`
	SchedulableNodeCount int                `json:"schedulableNodeCount"`
	CPU                  monitoringResource `json:"cpu"`
	Memory               monitoringResource `json:"memory"`
}

type monitoringPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

type monitoringSeries struct {
	Name   string            `json:"name"`
	Unit   string            `json:"unit"`
	Points []monitoringPoint `json:"points"`
}

func (h *ClusterMonitoringHandler) enabled(w http.ResponseWriter) bool {
	if h.db == nil || h.baseURL == "" {
		response.ServiceUnavailable(w, "cluster monitoring is not configured")
		return false
	}
	if u, err := url.Parse(h.baseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		response.ServiceUnavailable(w, "cluster monitoring endpoint is invalid")
		return false
	}
	return true
}

func (h *ClusterMonitoringHandler) clusterLabels(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, bool) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return "", false
	}
	id := r.PathValue("id")
	var found bool
	err := h.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM runtime_targets rt WHERE rt.id = $1
		AND rt.target_type IN ('kubernetes','edge_runtime') AND rt.is_active = true
		AND (rt.tenant_id = $2 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=rt.id AND tca.tenant_id=$2 AND tca.status='active')))`, id, trusted.TenantID).Scan(&found)
	if err != nil {
		response.InternalError(w, err.Error())
		return "", false
	}
	if !found {
		response.NotFound(w, "cluster not found")
		return "", false
	}
	return fmt.Sprintf(`hnb_tenant_id=%q,hnb_cluster_id=%q`, trusted.TenantID, id), true
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value  []json.RawMessage   `json:"value"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (h *ClusterMonitoringHandler) request(ctx context.Context, path string, values url.Values) (prometheusResponse, error) {
	var out prometheusResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+path+"?"+values.Encode(), nil)
	if err != nil {
		return out, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("monitoring backend returned %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return out, err
	}
	if out.Status != "success" {
		return out, fmt.Errorf("monitoring backend query failed")
	}
	return out, nil
}

func sampleValue(raw []json.RawMessage) (float64, bool) {
	if len(raw) < 2 {
		return 0, false
	}
	var text string
	if err := json.Unmarshal(raw[1], &text); err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(text, 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (h *ClusterMonitoringHandler) instant(ctx context.Context, query string) (float64, error) {
	res, err := h.request(ctx, "/api/v1/query", url.Values{"query": {query}})
	if err != nil || len(res.Data.Result) == 0 {
		return 0, err
	}
	v, ok := sampleValue(res.Data.Result[0].Value)
	if !ok {
		return 0, fmt.Errorf("monitoring backend returned an invalid sample")
	}
	return v, nil
}

func (h *ClusterMonitoringHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	labels, ok := h.clusterLabels(r.Context(), w, r)
	if !ok {
		return
	}
	query := func(metric string) float64 {
		v, err := h.instant(r.Context(), metric)
		if err != nil {
			return 0
		}
		return v
	}
	cpuTotal := query("sum(machine_cpu_cores{" + labels + "})")
	cpuUsed := query("sum(rate(node_cpu_seconds_total{" + labels + ",mode!=\"idle\"}[5m]))")
	cpuAllocated := query("sum(kube_node_status_allocatable{" + labels + ",resource=\"cpu\"})")
	memTotal := query("sum(node_memory_MemTotal_bytes{" + labels + "}) / 1024 / 1024 / 1024")
	memUsed := query("(sum(node_memory_MemTotal_bytes{" + labels + "}) - sum(node_memory_MemAvailable_bytes{" + labels + "})) / 1024 / 1024 / 1024")
	memAllocated := query("sum(kube_node_status_allocatable{" + labels + ",resource=\"memory\"}) / 1024 / 1024 / 1024")
	percent := func(n, d float64) float64 {
		if d <= 0 {
			return 0
		}
		return math.Round(n/d*10000) / 100
	}
	var out monitoringSummary
	out.CPU = monitoringResource{Total: cpuTotal, Used: cpuUsed, UsagePercent: percent(cpuUsed, cpuTotal), Allocated: cpuAllocated, AllocationPercent: percent(cpuAllocated, cpuTotal), Overcommitted: cpuAllocated, OvercommitPercent: percent(cpuAllocated, cpuTotal)}
	out.Memory = monitoringResource{Total: memTotal, Used: memUsed, UsagePercent: percent(memUsed, memTotal), Allocated: memAllocated, AllocationPercent: percent(memAllocated, memTotal), Overcommitted: memAllocated, OvercommitPercent: percent(memAllocated, memTotal)}
	out.SchedulableNodeCount = int(query("count(kube_node_spec_unschedulable{" + labels + "} == 0)"))
	out.NamespaceCount = int(query("count(kube_namespace_labels{" + labels + "})"))
	out.ProjectCount = int(query("count(count by (hnb_project_id) (kube_namespace_labels{" + labels + ",hnb_project_id!=\"\"}))"))
	out.Alerts.Critical = int(query("count(ALERTS{" + labels + ",alertstate=\"firing\",severity=\"critical\"})"))
	out.Alerts.Warning = int(query("count(ALERTS{" + labels + ",alertstate=\"firing\",severity=\"warning\"})"))
	writeJSONRaw(w, out)
}

func (h *ClusterMonitoringHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	labels, ok := h.clusterLabels(r.Context(), w, r)
	if !ok {
		return
	}
	start, end, err := monitoringRange(r)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	queries := map[string]string{
		"cpuUsage":    "100 * sum(rate(node_cpu_seconds_total{" + labels + ",mode!=\"idle\"}[5m])) / sum(machine_cpu_cores{" + labels + "})",
		"memoryUsage": "100 * (sum(node_memory_MemTotal_bytes{" + labels + "}) - sum(node_memory_MemAvailable_bytes{" + labels + "})) / sum(node_memory_MemTotal_bytes{" + labels + "})",
		"gpuUsage":    "avg(DCGM_FI_DEV_GPU_UTIL{" + labels + "})",
		"vramUsage":   "100 * sum(DCGM_FI_DEV_FB_USED{" + labels + "}) / sum(DCGM_FI_DEV_FB_TOTAL{" + labels + "})",
	}
	out := make(map[string]monitoringSeries, len(queries))
	for key, promQL := range queries {
		series, err := h.rangeQuery(r.Context(), promQL, start, end)
		if err != nil {
			response.ServiceUnavailable(w, "monitoring query failed")
			return
		}
		out[key] = monitoringSeries{Name: key, Unit: "%", Points: series}
	}
	writeJSONRaw(w, out)
}

func (h *ClusterMonitoringHandler) rangeQuery(ctx context.Context, query string, start, end time.Time) ([]monitoringPoint, error) {
	res, err := h.request(ctx, "/api/v1/query_range", url.Values{"query": {query}, "start": {start.Format(time.RFC3339)}, "end": {end.Format(time.RFC3339)}, "step": {"60"}})
	if err != nil || len(res.Data.Result) == 0 {
		return nil, err
	}
	points := make([]monitoringPoint, 0, len(res.Data.Result[0].Values))
	for _, raw := range res.Data.Result[0].Values {
		v, ok := sampleValue(raw)
		if !ok {
			continue
		}
		var unix float64
		if err := json.Unmarshal(raw[0], &unix); err != nil {
			continue
		}
		points = append(points, monitoringPoint{Timestamp: time.Unix(int64(unix), 0).UTC().Format(time.RFC3339), Value: v})
	}
	return points, nil
}

func monitoringRange(r *http.Request) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, r.URL.Query().Get("start"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start must be RFC3339")
	}
	end, err := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end must be RFC3339")
	}
	if !end.After(start) || end.Sub(start) > 31*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("range must be positive and no more than 31 days")
	}
	return start, end, nil
}

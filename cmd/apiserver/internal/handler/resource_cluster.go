package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

// ResourceClusterHandler serves the cluster-management Read Model (list/detail/
// nodes/dictionary) and the RuntimeIntent submission BFF. All queries are
// tenant-scoped and read-only (CQRS, white paper §3.5); writes are typed
// RuntimeIntents and never mutate the target directly.
//
// RuntimeIntent submission:
//   - When platform-api is configured, the body is forwarded to POST /v1/intents
//     (canonical Operation Engine path) and the response is mapped to the
//     RuntimeIntentRecord contract.
//   - Otherwise (standalone/dev), the intent is persisted into
//     bff_runtime_intents and a queued Operation is created. This is a degraded
//     local receipt, NOT a bypass of the Operation Engine.
type ResourceClusterHandler struct {
	db                   *sql.DB
	platformURL          string
	client               *http.Client
	projectionMode       clusterProjectionMode
	projectionComparator clusterProjectionComparator
	projectionRecorder   clusterProjectionRecorder
	delegationSigner     *iam.DelegationSigner
}

func (h *ResourceClusterHandler) ConfigureDelegation(signer *iam.DelegationSigner) {
	h.delegationSigner = signer
}

func NewResourceClusterHandler(db *sql.DB, platformURL string, projectionModes ...string) *ResourceClusterHandler {
	mode := clusterProjectionShadow
	if len(projectionModes) > 0 {
		mode = parseClusterProjectionMode(projectionModes[0])
	}
	h := &ResourceClusterHandler{
		db:                 db,
		platformURL:        strings.TrimRight(platformURL, "/"),
		client:             newInternalHTTPClient(30 * time.Second),
		projectionMode:     mode,
		projectionRecorder: prometheusClusterProjectionRecorder{},
	}
	if db != nil {
		h.projectionComparator = sqlClusterProjectionComparator{db: db}
	}
	return h
}

// ---------------------------------------------------------------------------
// Read Model DTOs (JSON shapes match web/plugins/resource cluster-management)
// ---------------------------------------------------------------------------

type clusterCapabilitySnapshot struct {
	SnapshotVersion int    `json:"snapshotVersion"`
	ObservedAt      string `json:"observedAt"`
	Freshness       string `json:"freshness"`
}

type clusterSummary struct {
	ClusterID          string                     `json:"clusterId"`
	DisplayName        string                     `json:"displayName"`
	Description        string                     `json:"description,omitempty"`
	Kind               string                     `json:"kind"`
	Source             string                     `json:"source"`
	Status             string                     `json:"status"`
	LifecycleState     string                     `json:"lifecycleState"`
	HealthState        string                     `json:"healthState"`
	ConnectivityState  string                     `json:"connectivityState"`
	FreshnessState     string                     `json:"freshnessState"`
	ObservedAt         string                     `json:"observedAt"`
	LastKnownStateAt   string                     `json:"lastKnownStateAt"`
	RuntimeVersion     string                     `json:"runtimeVersion"`
	ExpectedVersion    int64                      `json:"expectedVersion"`
	NodeCount          int                        `json:"nodeCount"`
	CPUTotal           string                     `json:"cpuTotal"`
	MemoryTotal        string                     `json:"memoryTotal"`
	CapabilitySnapshot *clusterCapabilitySnapshot `json:"capabilitySnapshot"`
	TenantID           string                     `json:"tenantId"`
	EnvironmentID      string                     `json:"environmentId,omitempty"`
	CreatedAt          string                     `json:"createdAt"`
	UpdatedAt          string                     `json:"updatedAt"`
}

type clusterListPayload struct {
	Items   []clusterSummary     `json:"items"`
	Total   int                  `json:"total"`
	Summary clusterListAggregate `json:"summary"`
}

// clusterListAggregate is calculated over the entire filtered Read Model, not
// the current result page. This prevents pagination from changing dashboard
// totals and status cards.
type clusterListAggregate struct {
	Total    int `json:"total"`
	Running  int `json:"running"`
	Degraded int `json:"degraded"`
	Stale    int `json:"stale"`
}

type clusterNode struct {
	NodeID            string `json:"nodeId"`
	Name              string `json:"name"`
	Role              string `json:"role"`
	Status            string `json:"status"`
	IPAddress         string `json:"ipAddress,omitempty"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	CPUAllocatable    string `json:"cpuAllocatable"`
	MemoryAllocatable string `json:"memoryAllocatable"`
	KubeletVersion    string `json:"kubeletVersion"`
	LastHeartbeatAt   string `json:"lastHeartbeatAt"`
	LastKnownStateAt  string `json:"lastKnownStateAt,omitempty"`
	Freshness         string `json:"freshness,omitempty"`
}

type clusterNodeListPayload struct {
	Items []clusterNode `json:"items"`
	Total int           `json:"total"`
}

type dictionaryItem struct {
	Code     string `json:"code"`
	LabelKey string `json:"labelKey"`
	Semantic string `json:"semantic"`
	Icon     string `json:"icon"`
	Terminal bool   `json:"terminal"`
}

type statusDictionaryPayload struct {
	DictionaryID string           `json:"dictionaryId"`
	Items        []dictionaryItem `json:"items"`
}

// ---------------------------------------------------------------------------
// Status / kind mapping helpers
// ---------------------------------------------------------------------------

func freshness(observedAt *time.Time, thresholdSec int) string {
	if observedAt == nil {
		return "stale"
	}
	threshold := time.Duration(thresholdSec) * time.Second
	if time.Since(*observedAt) > threshold {
		return "stale"
	}
	return "fresh"
}

func mapCombinedStatus(lifecycle, health, connectivity, freshness string) string {
	if freshness == "STALE" {
		return "STALE"
	}
	if lifecycle == "TERMINATED" {
		return "TERMINATED"
	}
	if lifecycle == "DELETING" {
		return "DELETING"
	}
	if lifecycle == "FAILED" {
		return "FAILED"
	}
	if lifecycle == "UPGRADING" {
		return "UPGRADING"
	}
	if lifecycle == "PROVISIONING" {
		return "PROVISIONING"
	}
	if lifecycle == "REGISTERING" {
		return "REGISTERING"
	}
	if health == "DEGRADED" || health == "UNHEALTHY" || connectivity == "DISCONNECTED" {
		return "DEGRADED"
	}
	if lifecycle == "ACTIVE" && health == "HEALTHY" {
		return "RUNNING"
	}
	return "UNKNOWN"
}

func targetTypeToKind(tt string) string {
	switch tt {
	case "kubernetes":
		return "kubernetes"
	case "edge_runtime":
		return "edge"
	default:
		return ""
	}
}

func formatCPUTotal(cores int) string {
	if cores <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dC", cores)
}

func formatMemoryTotal(memoryMB int64) string {
	if memoryMB <= 0 {
		return "-"
	}
	if memoryMB >= 1024 {
		return fmt.Sprintf("%.0f GiB", float64(memoryMB)/1024)
	}
	return fmt.Sprintf("%d MiB", memoryMB)
}

func parseLabels(raw []byte) map[string]string {
	labels := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &labels)
	}
	return labels
}

// ---------------------------------------------------------------------------
// List (Read Model, paginated)
// ---------------------------------------------------------------------------

func (h *ResourceClusterHandler) ListClusters(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeLocalClusterProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	tenantID := trusted.TenantID
	if !h.allowClusterProjectionRead(r.Context(), tenantID) {
		response.ServiceUnavailable(w, "cluster read projection cutover blocked")
		return
	}

	page := atoiDefault(r.URL.Query().Get("page"), 1)
	pageSize := atoiDefault(r.URL.Query().Get("pageSize"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	kind := r.URL.Query().Get("kind")
	// `status` is the console's public, combined status filter. Keep the
	// four-dimensional filters for advanced callers, but apply this contract
	// server-side so filtering remains authoritative.
	combinedStatus := strings.ToUpper(r.URL.Query().Get("status"))
	lifecycleState := strings.ToUpper(r.URL.Query().Get("lifecycleState"))
	healthState := strings.ToUpper(r.URL.Query().Get("healthState"))
	connectivityState := strings.ToUpper(r.URL.Query().Get("connectivityState"))
	freshnessState := strings.ToUpper(r.URL.Query().Get("freshnessState"))

	// Validate kind filter
	targetType := ""
	if kind != "" {
		switch kind {
		case "kubernetes":
			targetType = "kubernetes"
		case "edge":
			targetType = "edge_runtime"
		default:
			response.BadRequest(w, "invalid kind filter: "+kind)
			return
		}
	}

	// Validate state filters
	validLifecycle := map[string]bool{
		"UNKNOWN": true, "REGISTERING": true, "PROVISIONING": true, "ACTIVE": true,
		"UPGRADING": true, "FAILED": true, "DELETING": true, "TERMINATED": true,
	}
	validHealth := map[string]bool{
		"UNKNOWN": true, "HEALTHY": true, "DEGRADED": true, "UNHEALTHY": true,
	}
	validConnectivity := map[string]bool{
		"UNKNOWN": true, "CONNECTED": true, "DISCONNECTED": true,
	}
	validFreshness := map[string]bool{
		"UNKNOWN": true, "FRESH": true, "STALE": true,
	}

	if lifecycleState != "" && !validLifecycle[lifecycleState] {
		response.BadRequest(w, "invalid lifecycleState filter: "+lifecycleState)
		return
	}
	if healthState != "" && !validHealth[healthState] {
		response.BadRequest(w, "invalid healthState filter: "+healthState)
		return
	}
	if connectivityState != "" && !validConnectivity[connectivityState] {
		response.BadRequest(w, "invalid connectivityState filter: "+connectivityState)
		return
	}
	if freshnessState != "" && !validFreshness[freshnessState] {
		response.BadRequest(w, "invalid freshnessState filter: "+freshnessState)
		return
	}
	validCombined := map[string]bool{
		"RUNNING": true, "DEGRADED": true, "STALE": true, "REGISTERING": true,
		"PROVISIONING": true, "UPGRADING": true, "FAILED": true, "DELETING": true,
		"TERMINATED": true, "UNKNOWN": true,
	}
	if combinedStatus != "" && !validCombined[combinedStatus] {
		response.BadRequest(w, "invalid status filter: "+combinedStatus)
		return
	}

	where, queryArgs := clusterListWhere(tenantID, targetType, keyword, lifecycleState, healthState, connectivityState, freshnessState, combinedStatus)

	// One aggregate scan provides both the exact page total and the dashboard
	// cards, avoiding a second full scan of runtime_targets on every refresh.
	aggregateQuery := `SELECT count(*),
		count(*) FILTER (WHERE rt.freshness_state <> 'STALE' AND rt.lifecycle_state = 'ACTIVE' AND rt.health_state = 'HEALTHY' AND rt.connectivity_state <> 'DISCONNECTED'),
		count(*) FILTER (WHERE rt.freshness_state <> 'STALE' AND rt.lifecycle_state NOT IN ('TERMINATED','DELETING','FAILED','UPGRADING','PROVISIONING','REGISTERING') AND (rt.health_state IN ('DEGRADED','UNHEALTHY') OR rt.connectivity_state = 'DISCONNECTED')),
		count(*) FILTER (WHERE rt.freshness_state = 'STALE')
		FROM runtime_targets rt ` + where
	var aggregate clusterListAggregate
	if err := h.db.QueryRowContext(r.Context(), aggregateQuery, queryArgs...).Scan(&aggregate.Total, &aggregate.Running, &aggregate.Degraded, &aggregate.Stale); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	total := aggregate.Total

	// Data query with proper LIMIT/OFFSET
	dataQuery := `
		SELECT rt.id, rt.tenant_id, rt.name, COALESCE(rt.display_name, rt.name), COALESCE(rt.description, ''),
		       rt.target_type, rt.lifecycle_state, rt.health_state,
		       rt.connectivity_state, rt.freshness_state,
		       rt.observed_at, rt.last_known_state_at,
		       rt.labels, rt.observed_at, rt.stale_threshold_seconds,
		       rt.created_at, rt.updated_at,
		       COALESCE(cs.kube_version, ''), COALESCE(cs.cpu_cores, 0), COALESCE(cs.memory_mb, 0),
		       (SELECT count(*) FROM runtime_target_nodes n WHERE n.target_id = rt.id AND n.deleted_at IS NULL),
		       COALESCE(rt.projection_version, 0)
		FROM runtime_targets rt
		LEFT JOIN LATERAL (
			SELECT kube_version, cpu_cores, memory_mb
			FROM capability_snapshots cs
			WHERE cs.target_id = rt.id
			ORDER BY cs.observed_at DESC
			LIMIT 1
		) cs ON true
		` + where
	dataQuery += fmt.Sprintf(" ORDER BY rt.updated_at DESC, rt.id DESC LIMIT %d OFFSET %d", pageSize, (page-1)*pageSize)

	rows, err := h.db.QueryContext(r.Context(), dataQuery, queryArgs...)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer rows.Close()

	all := make([]clusterSummary, 0, pageSize)
	for rows.Next() {
		var (
			id, tenant, name, displayName, description, targetType string
			lifecycleState, healthState, connState, freshState     string
			labelsRaw                                              []byte
			observedAt                                             sql.NullTime
			threshold                                              int
			createdAt, updatedAt                                   time.Time
			kubeVersion                                            string
			cpuCores                                               int
			memoryMB                                               int64
			nodeCount                                              int
			lastKnownStateAt                                       sql.NullTime
			projectionVersion                                      int64
		)
		if err := rows.Scan(&id, &tenant, &name, &displayName, &description, &targetType,
			&lifecycleState, &healthState, &connState, &freshState,
			&observedAt, &lastKnownStateAt,
			&labelsRaw, &observedAt, &threshold, &createdAt, &updatedAt,
			&kubeVersion, &cpuCores, &memoryMB, &nodeCount, &projectionVersion); err != nil {
			continue
		}
		kind := targetTypeToKind(targetType)
		if kind == "" {
			continue
		}
		labels := parseLabels(labelsRaw)
		source := labels["hnb.source"]
		if source != "created" && source != "imported" {
			source = "imported"
		}
		var observed *time.Time
		if observedAt.Valid {
			observed = &observedAt.Time
		}
		var lastKnown *time.Time
		if lastKnownStateAt.Valid {
			lastKnown = &lastKnownStateAt.Time
		}
		all = append(all, clusterSummary{
			ClusterID:         id,
			DisplayName:       displayName,
			Description:       description,
			Kind:              kind,
			Source:            source,
			Status:            mapCombinedStatus(lifecycleState, healthState, connState, freshState),
			LifecycleState:    lifecycleState,
			HealthState:       healthState,
			ConnectivityState: connState,
			FreshnessState:    freshState,
			ObservedAt:        formatTimePtr(observed),
			LastKnownStateAt:  formatTimePtr(lastKnown),
			RuntimeVersion:    kubeVersion,
			ExpectedVersion:   projectionVersion,
			NodeCount:         nodeCount,
			CPUTotal:          formatCPUTotal(cpuCores),
			MemoryTotal:       formatMemoryTotal(memoryMB),
			CapabilitySnapshot: &clusterCapabilitySnapshot{
				SnapshotVersion: capabilitySnapshotVersion(observed),
				ObservedAt:      formatTimePtr(observed),
				Freshness:       freshState,
			},
			TenantID:      tenant,
			EnvironmentID: labels["environmentId"],
			CreatedAt:     createdAt.UTC().Format(time.RFC3339),
			UpdatedAt:     updatedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSONRaw(w, clusterListPayload{Items: all, Total: total, Summary: aggregate})
}

func capabilitySnapshotVersion(observed *time.Time) int {
	if observed == nil {
		return 0
	}
	return int(observed.Unix())
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ---------------------------------------------------------------------------
// Get (detail)
// ---------------------------------------------------------------------------

func (h *ResourceClusterHandler) GetCluster(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if !h.allowClusterProjectionRead(r.Context(), trusted.TenantID) {
		response.ServiceUnavailable(w, "cluster read projection cutover blocked")
		return
	}
	id := r.PathValue("id")

	var (
		name, displayName, description, targetType, lifecycleState, healthState, connState, freshState string
		labelsRaw                                                                                      []byte
		observedAt                                                                                     sql.NullTime
		lastKnownStateAt                                                                               sql.NullTime
		threshold                                                                                      int
		createdAt, updatedAt                                                                           time.Time
		kubeVersion                                                                                    string
		cpuCores                                                                                       int
		memoryMB                                                                                       int64
		nodeCount                                                                                      int
		environmentID                                                                                  string
		projectionVersion                                                                              int64
	)
	err := h.db.QueryRowContext(r.Context(), `
		SELECT rt.name, COALESCE(rt.display_name, rt.name), COALESCE(rt.description, ''),
		       rt.target_type, rt.lifecycle_state, rt.health_state,
		       rt.connectivity_state, rt.freshness_state,
		       rt.labels, rt.observed_at, rt.last_known_state_at,
		       rt.stale_threshold_seconds, rt.created_at, rt.updated_at,
		       COALESCE(cs.kube_version, ''), COALESCE(cs.cpu_cores, 0), COALESCE(cs.memory_mb, 0),
		       (SELECT count(*) FROM runtime_target_nodes n WHERE n.target_id = rt.id AND n.deleted_at IS NULL),
		       COALESCE(rt.projection_version, 0)
		FROM runtime_targets rt
		LEFT JOIN LATERAL (
			SELECT kube_version, cpu_cores, memory_mb
			FROM capability_snapshots cs
			WHERE cs.target_id = rt.id
			ORDER BY cs.observed_at DESC
			LIMIT 1
		) cs ON true
		WHERE rt.id = $1 AND (rt.tenant_id = $2 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=rt.id AND tca.tenant_id=$2 AND tca.status='active'))
		  AND rt.target_type IN ('kubernetes','edge_runtime')`, id, trusted.TenantID).
		Scan(&name, &displayName, &description, &targetType, &lifecycleState, &healthState,
			&connState, &freshState, &labelsRaw, &observedAt, &lastKnownStateAt,
			&threshold, &createdAt, &updatedAt, &kubeVersion, &cpuCores, &memoryMB, &nodeCount,
			&projectionVersion)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "cluster not found")
		return
	}
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	kind := targetTypeToKind(targetType)
	if kind == "" {
		response.NotFound(w, "cluster not found")
		return
	}
	labels := parseLabels(labelsRaw)
	source := labels["hnb.source"]
	if source != "created" && source != "imported" {
		source = "imported"
	}
	if v := labels["environmentId"]; v != "" {
		environmentID = v
	}
	var observed *time.Time
	if observedAt.Valid {
		observed = &observedAt.Time
	}
	var lastKnown *time.Time
	if lastKnownStateAt.Valid {
		lastKnown = &lastKnownStateAt.Time
	}
	summary := clusterSummary{
		ClusterID:         id,
		DisplayName:       displayName,
		Description:       description,
		Kind:              kind,
		Source:            source,
		Status:            mapCombinedStatus(lifecycleState, healthState, connState, freshState),
		LifecycleState:    lifecycleState,
		HealthState:       healthState,
		ConnectivityState: connState,
		FreshnessState:    freshState,
		ObservedAt:        formatTimePtr(observed),
		LastKnownStateAt:  formatTimePtr(lastKnown),
		RuntimeVersion:    kubeVersion,
		ExpectedVersion:   projectionVersion,
		NodeCount:         nodeCount,
		CPUTotal:          formatCPUTotal(cpuCores),
		MemoryTotal:       formatMemoryTotal(memoryMB),
		CapabilitySnapshot: &clusterCapabilitySnapshot{
			SnapshotVersion: capabilitySnapshotVersion(observed),
			ObservedAt:      formatTimePtr(observed),
			Freshness:       freshState,
		},
		TenantID:      trusted.TenantID,
		EnvironmentID: environmentID,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339),
		UpdatedAt:     updatedAt.UTC().Format(time.RFC3339),
	}
	writeJSONRaw(w, summary)
}

// ---------------------------------------------------------------------------
// Nodes (read-only, tenant-scoped)
// ---------------------------------------------------------------------------

func (h *ResourceClusterHandler) ListClusterNodes(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if !h.allowClusterProjectionRead(r.Context(), trusted.TenantID) {
		response.ServiceUnavailable(w, "cluster read projection cutover blocked")
		return
	}
	id := r.PathValue("id")
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	pageSize := atoiDefault(r.URL.Query().Get("pageSize"), 50)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	// Validate target exists and belongs to tenant
	var targetType string
	if err := h.db.QueryRowContext(r.Context(), `
		SELECT target_type FROM runtime_targets WHERE id = $1 AND (tenant_id = $2 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=runtime_targets.id AND tca.tenant_id=$2 AND tca.status='active'))`,
		id, trusted.TenantID).Scan(&targetType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(w, "cluster not found")
		} else {
			response.InternalError(w, err.Error())
		}
		return
	}
	// Reject non-cluster target types
	if targetType != "kubernetes" && targetType != "edge_runtime" {
		response.NotFound(w, "cluster not found")
		return
	}

	// Exact count
	var total int
	if err := h.db.QueryRowContext(r.Context(), `
		SELECT count(*)
		FROM runtime_target_nodes n
		WHERE n.target_id = $1 AND n.deleted_at IS NULL`, id).Scan(&total); err != nil {
		response.InternalError(w, err.Error())
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT n.id, n.name, n.role, n.node_status, COALESCE(n.ip_address, ''),
		       n.os, n.arch, n.cpu_allocatable, n.memory_allocatable,
		       n.kubelet_version, n.last_heartbeat_at,
		       n.last_known_state_at
		FROM runtime_target_nodes n
		WHERE n.target_id = $1 AND n.deleted_at IS NULL
		ORDER BY n.name
		LIMIT $2 OFFSET $3`, id, pageSize, (page-1)*pageSize)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer rows.Close()

	items := make([]clusterNode, 0, 16)
	for rows.Next() {
		var (
			nodeID, name, role, nodeStatus, ip, osName, arch string
			cpu, memory, kubelet                             string
			heartbeat, lastKnown                             sql.NullTime
		)
		if err := rows.Scan(&nodeID, &name, &role, &nodeStatus, &ip, &osName, &arch,
			&cpu, &memory, &kubelet, &heartbeat, &lastKnown); err != nil {
			continue
		}
		// Normalize role: control_plane -> control-plane
		nodeRole := role
		if nodeRole == "control_plane" {
			nodeRole = "control-plane"
		}
		// Determine node freshness
		nodeFreshness := "fresh"
		if lastKnown.Valid {
			if time.Since(lastKnown.Time) > time.Duration(atoiDefault(r.URL.Query().Get("freshnessThreshold"), 300))*time.Second {
				nodeFreshness = "stale"
			}
		}
		items = append(items, clusterNode{
			NodeID:            nodeID,
			Name:              name,
			Role:              nodeRole,
			Status:            nodeStatus,
			IPAddress:         ip,
			OS:                osName,
			Arch:              arch,
			CPUAllocatable:    cpu,
			MemoryAllocatable: memory,
			KubeletVersion:    kubelet,
			LastHeartbeatAt:   formatTimePtr(timePtr(heartbeat)),
			LastKnownStateAt:  formatTimePtr(timePtr(lastKnown)),
			Freshness:         nodeFreshness,
		})
	}
	if items == nil {
		items = []clusterNode{}
	}
	writeJSONRaw(w, clusterNodeListPayload{Items: items, Total: total})
}

func timePtr(t sql.NullTime) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

// ---------------------------------------------------------------------------
// Plugin / platform capability status read model
// ---------------------------------------------------------------------------

// pluginStatus is one entry of the cluster plugin status read model.
// status follows the frontend ClusterPluginStatusKind enum:
// running | installed | not-installed | abnormal | unknown.
type pluginStatus struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
}

type clusterPluginStatusPayload struct {
	Items      []pluginStatus `json:"items"`
	ObservedAt *time.Time     `json:"observedAt"`
}

var cniPluginDisplayNames = map[string]string{
	"ovnk8s": "OVN网络插件", "ovn": "OVN网络插件", "calico": "Calico网络插件",
	"flannel": "Flannel网络插件", "cilium": "Cilium网络插件",
	"bridge": "Bridge网络插件", "hostport": "HostPort网络插件", "macvlan": "MacVLAN网络插件",
}

var csiDriverDisplayNames = map[string]string{
	"hostpath": "HostPath CSI 驱动", "nfs": "NFS CSI 驱动", "local": "本地存储 CSI 驱动",
	"cephfs.csi.ceph.com": "CephFS CSI 驱动", "rbd.csi.ceph.com": "RBD CSI 驱动",
	"ebs.csi.aws.com": "EBS CSI 驱动", "pd.csi.storage.gke.io": "GCE PD CSI 驱动",
	"disk.csi.azure.com": "Azure Disk CSI 驱动",
}

// parsePGTextArray parses a Postgres TEXT[] literal ("{a,b,c}") into a slice.
func parsePGTextArray(raw []byte) []string {
	s := strings.Trim(strings.TrimSpace(string(raw)), "{}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ListClusterPluginStatuses returns the plugin/capability status read model
// derived from the target's latest capability snapshot (cni_plugins /
// csi_drivers / snapshot_json). When no snapshot exists or nothing can be
// derived, an empty list is returned (frontend renders an empty state); the
// read model never invents plugin data.
func (h *ResourceClusterHandler) ListClusterPluginStatuses(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if !h.allowClusterProjectionRead(r.Context(), trusted.TenantID) {
		response.ServiceUnavailable(w, "cluster read projection cutover blocked")
		return
	}
	id := r.PathValue("id")

	// Validate target exists and belongs to tenant
	var targetType string
	if err := h.db.QueryRowContext(r.Context(), `
		SELECT target_type FROM runtime_targets WHERE id = $1 AND (tenant_id = $2 OR EXISTS (SELECT 1 FROM tenant_cluster_allocations tca WHERE tca.cluster_id=runtime_targets.id AND tca.tenant_id=$2 AND tca.status='active'))`,
		id, trusted.TenantID).Scan(&targetType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(w, "cluster not found")
		} else {
			response.InternalError(w, err.Error())
		}
		return
	}
	if targetType != "kubernetes" && targetType != "edge_runtime" {
		response.NotFound(w, "cluster not found")
		return
	}

	var (
		cniRaw, csiRaw, featuresRaw, snapshotRaw []byte
		observedAt                               sql.NullTime
	)
	err := h.db.QueryRowContext(r.Context(), `
		SELECT COALESCE(cni_plugins::text, '{}'), COALESCE(csi_drivers::text, '{}'),
		       COALESCE(features::text, '{}'), COALESCE(snapshot_json::text, '{}'),
		       observed_at
		FROM capability_snapshots cs
		WHERE cs.target_id = $1
		ORDER BY cs.observed_at DESC
		LIMIT 1`, id).Scan(&cniRaw, &csiRaw, &featuresRaw, &snapshotRaw, &observedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		response.InternalError(w, err.Error())
		return
	}

	items := []pluginStatus{}
	var observed *time.Time
	if err == nil {
		for _, name := range parsePGTextArray(cniRaw) {
			dn, okMap := cniPluginDisplayNames[strings.ToLower(name)]
			if !okMap {
				dn = "CNI插件 " + name
			}
			items = append(items, pluginStatus{Key: "cni/" + name, DisplayName: dn, Status: "installed"})
		}
		for _, driver := range parsePGTextArray(csiRaw) {
			dn, okMap := csiDriverDisplayNames[strings.ToLower(driver)]
			if !okMap {
				dn = "CSI驱动 " + driver
			}
			items = append(items, pluginStatus{Key: "csi/" + driver, DisplayName: dn, Status: "installed"})
		}
		if features := pluginStatussFromJSON(featuresRaw); len(features) > 0 {
			items = append(items, features...)
		}
		items = append(items, pluginStatussFromSnapshot(snapshotRaw)...)
		if observedAt.Valid {
			t := observedAt.Time
			observed = &t
		}
	}
	writeJSONRaw(w, clusterPluginStatusPayload{Items: items, ObservedAt: observed})
}

// pluginStatussFromSnapshot extracts known plugin/feature entries from the
// capability snapshot content: plugins: [{name, displayName, status}] and/or
// features: {name: state}.
func pluginStatussFromSnapshot(raw []byte) []pluginStatus {
	content, ok := unmarshalJSONObject(raw)
	if !ok {
		return nil
	}
	return derivePluginStatuss(content["plugins"], content["features"])
}

// pluginStatussFromJSON parses a JSONB column (features map) into entries.
func pluginStatussFromJSON(raw []byte) []pluginStatus {
	content, ok := unmarshalJSONObject(raw)
	if !ok {
		return nil
	}
	return derivePluginStatuss(nil, content["features"])
}

func unmarshalJSONObject(raw []byte) (map[string]any, bool) {
	var content map[string]any
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, false
	}
	return content, true
}

func derivePluginStatuss(plugins any, features any) []pluginStatus {
	out := []pluginStatus{}
	if list, ok := plugins.([]any); ok {
		for _, entry := range list {
			m, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			if name == "" {
				continue
			}
			out = append(out, pluginStatus{
				Key:         "plugin/" + name,
				DisplayName: stringOr(m["displayName"], name),
				Status:      normalizePluginStatus(m["status"]),
			})
		}
	}
	if featuresMap, ok := features.(map[string]any); ok {
		for name, state := range featuresMap {
			out = append(out, pluginStatus{
				Key:         "feature/" + name,
				DisplayName: name,
				Status:      normalizePluginStatus(state),
			})
		}
	}
	return out
}

func stringOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}

// normalizePluginStatus maps raw observation states onto the frontend enum
// (running/installed/not-installed/abnormal/unknown); unrecognized values
// stay "unknown" rather than being guessed.
func normalizePluginStatus(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "running"
		}
		return "not-installed"
	}
	switch s := strings.ToLower(strings.TrimSpace(stringOr(v, ""))); {
	case s == "":
		return "unknown"
	case strings.HasPrefix(s, "not-") || s == "absent" || s == "missing" || s == "disabled" || s == "false" || s == "0":
		return "not-installed"
	case s == "running" || s == "ready" || s == "active" || s == "up" || s == "healthy" || s == "online" || s == "true" || s == "1":
		return "running"
	case s == "installed" || s == "enabled" || s == "present":
		return "installed"
	case s == "abnormal" || s == "degraded" || s == "error" || s == "failed" || s == "down" || s == "unhealthy":
		return "abnormal"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Status dictionaries
// ---------------------------------------------------------------------------

var lifecycleDictionary = []dictionaryItem{
	{Code: "UNKNOWN", LabelKey: "resource.clusterMgmt.lifecycle.UNKNOWN", Semantic: "default", Icon: "question", Terminal: false},
	{Code: "REGISTERING", LabelKey: "resource.clusterMgmt.lifecycle.REGISTERING", Semantic: "processing", Icon: "loading", Terminal: false},
	{Code: "PROVISIONING", LabelKey: "resource.clusterMgmt.lifecycle.PROVISIONING", Semantic: "processing", Icon: "loading", Terminal: false},
	{Code: "ACTIVE", LabelKey: "resource.clusterMgmt.lifecycle.ACTIVE", Semantic: "success", Icon: "check-circle", Terminal: false},
	{Code: "UPGRADING", LabelKey: "resource.clusterMgmt.lifecycle.UPGRADING", Semantic: "processing", Icon: "loading", Terminal: false},
	{Code: "FAILED", LabelKey: "resource.clusterMgmt.lifecycle.FAILED", Semantic: "error", Icon: "alert", Terminal: false},
	{Code: "DELETING", LabelKey: "resource.clusterMgmt.lifecycle.DELETING", Semantic: "processing", Icon: "loading", Terminal: false},
	{Code: "TERMINATED", LabelKey: "resource.clusterMgmt.lifecycle.TERMINATED", Semantic: "default", Icon: "circle", Terminal: true},
}

var healthDictionary = []dictionaryItem{
	{Code: "UNKNOWN", LabelKey: "resource.clusterMgmt.health.UNKNOWN", Semantic: "default", Icon: "question", Terminal: false},
	{Code: "HEALTHY", LabelKey: "resource.clusterMgmt.health.HEALTHY", Semantic: "success", Icon: "check-circle", Terminal: false},
	{Code: "DEGRADED", LabelKey: "resource.clusterMgmt.health.DEGRADED", Semantic: "warning", Icon: "alert", Terminal: false},
	{Code: "UNHEALTHY", LabelKey: "resource.clusterMgmt.health.UNHEALTHY", Semantic: "error", Icon: "alert", Terminal: false},
}

var connectivityDictionary = []dictionaryItem{
	{Code: "UNKNOWN", LabelKey: "resource.clusterMgmt.connectivity.UNKNOWN", Semantic: "default", Icon: "question", Terminal: false},
	{Code: "CONNECTED", LabelKey: "resource.clusterMgmt.connectivity.CONNECTED", Semantic: "success", Icon: "check-circle", Terminal: false},
	{Code: "DISCONNECTED", LabelKey: "resource.clusterMgmt.connectivity.DISCONNECTED", Semantic: "warning", Icon: "alert", Terminal: false},
}

var freshnessDictionary = []dictionaryItem{
	{Code: "UNKNOWN", LabelKey: "resource.clusterMgmt.freshness.UNKNOWN", Semantic: "default", Icon: "question", Terminal: false},
	{Code: "FRESH", LabelKey: "resource.clusterMgmt.freshness.FRESH", Semantic: "success", Icon: "check-circle", Terminal: false},
	{Code: "STALE", LabelKey: "resource.clusterMgmt.freshness.STALE", Semantic: "warning", Icon: "alert", Terminal: false},
}

func (h *ResourceClusterHandler) StatusDictionary(w http.ResponseWriter, r *http.Request) {
	dictID := r.PathValue("id")
	var items []dictionaryItem
	switch dictID {
	case "resource.cluster.lifecycle":
		items = lifecycleDictionary
	case "resource.cluster.health":
		items = healthDictionary
	case "resource.cluster.connectivity":
		items = connectivityDictionary
	case "resource.cluster.freshness":
		items = freshnessDictionary
	case "resource.cluster.status":
		// Backward-compatible combined status dictionary
		items = []dictionaryItem{
			{Code: "UNKNOWN", LabelKey: "resource.clusterMgmt.status.UNKNOWN", Semantic: "default", Icon: "question", Terminal: false},
			{Code: "REGISTERING", LabelKey: "resource.clusterMgmt.status.REGISTERING", Semantic: "processing", Icon: "loading", Terminal: false},
			{Code: "PROVISIONING", LabelKey: "resource.clusterMgmt.status.PROVISIONING", Semantic: "processing", Icon: "loading", Terminal: false},
			{Code: "UPGRADING", LabelKey: "resource.clusterMgmt.status.UPGRADING", Semantic: "processing", Icon: "loading", Terminal: false},
			{Code: "RUNNING", LabelKey: "resource.clusterMgmt.status.RUNNING", Semantic: "success", Icon: "check-circle", Terminal: false},
			{Code: "DEGRADED", LabelKey: "resource.clusterMgmt.status.DEGRADED", Semantic: "warning", Icon: "alert", Terminal: false},
			{Code: "STALE", LabelKey: "resource.clusterMgmt.status.STALE", Semantic: "warning", Icon: "alert", Terminal: false},
			{Code: "FAILED", LabelKey: "resource.clusterMgmt.status.FAILED", Semantic: "danger", Icon: "error", Terminal: false},
			{Code: "DELETING", LabelKey: "resource.clusterMgmt.status.DELETING", Semantic: "processing", Icon: "loading", Terminal: false},
			{Code: "TERMINATED", LabelKey: "resource.clusterMgmt.status.TERMINATED", Semantic: "default", Icon: "circle", Terminal: true},
		}
	default:
		response.NotFound(w, "dictionary not found: "+dictID)
		return
	}
	writeJSONRaw(w, statusDictionaryPayload{DictionaryID: dictID, Items: items})
}

// ---------------------------------------------------------------------------
// RuntimeIntent submission
// ---------------------------------------------------------------------------

var clusterIntentKinds = map[string]bool{
	"CreateKubernetesTarget": true,
	"ImportRuntimeTarget":    true,
	"UpgradeRuntimeTarget":   true,
	"DeleteRuntimeTarget":    true,
}

type bffIntentEnvelope struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   bffIntentMetadata `json:"metadata"`
	Spec       bffIntentSpec     `json:"spec"`
}

type bffIntentMetadata struct {
	IdempotencyKey string `json:"idempotencyKey"`
	CorrelationID  string `json:"correlationId"`
}

type bffIntentSpec struct {
	TargetID                    string               `json:"targetId,omitempty"`
	TargetKind                  string               `json:"targetKind"`
	ExpectedVersion             int64                `json:"expectedVersion,omitempty"`
	BindingID                   string               `json:"bindingId,omitempty"`
	BindingVersion              int64                `json:"bindingVersion,omitempty"`
	OfferingID                  string               `json:"offeringId,omitempty"`
	OfferingVersion             int64                `json:"offeringVersion,omitempty"`
	StorageClassName            string               `json:"storageClassName,omitempty"`
	StorageClassUID             string               `json:"storageClassUid,omitempty"`
	StorageClassResourceVersion string               `json:"storageClassResourceVersion,omitempty"`
	InstallationID              string               `json:"installationId,omitempty"`
	PackageID                   string               `json:"packageId,omitempty"`
	PackageVersion              string               `json:"packageVersion,omitempty"`
	CurrentVersion              string               `json:"currentVersion,omitempty"`
	DesiredVersion              string               `json:"desiredVersion,omitempty"`
	DisplayName                 string               `json:"displayName,omitempty"`
	KubernetesVersion           string               `json:"kubernetesVersion,omitempty"`
	CloudCoreEndpoint           string               `json:"cloudCoreEndpoint,omitempty"`
	CredentialSecretRef         *bffIntentSecretRef  `json:"credentialSecretRef,omitempty"`
	NodeGroupMappings           map[string]string    `json:"nodeGroupMappings,omitempty"`
	RiskConfirmation            map[string]any       `json:"riskConfirmation,omitempty"`
	Parameters                  map[string]any       `json:"parameters,omitempty"`
	SecretReferences            []bffIntentSecretRef `json:"secretReferences,omitempty"`
	VolumeID                    string               `json:"volumeId,omitempty"`
	WorkflowProviderRef         string               `json:"workflowProviderRef,omitempty"`
	PersistentVolume            any                  `json:"persistentVolume,omitempty"`
	PersistentVolumeClaim       any                  `json:"persistentVolumeClaim,omitempty"`
	PodDependencies             []any                `json:"podDependencies,omitempty"`
	StatefulSetDependencies     []any                `json:"statefulSetDependencies,omitempty"`
}

type bffIntentSecretRef struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

type runtimeIntentRecord struct {
	APIVersion      string `json:"apiVersion"`
	IntentID        string `json:"intentId"`
	Status          string `json:"status"`
	SemanticDigest  string `json:"semanticDigest"`
	Intent          any    `json:"intent"`
	ExecutionPlanID string `json:"executionPlanId,omitempty"`
	OperationID     string `json:"operationId,omitempty"`
	CreatedAt       string `json:"createdAt"`
	CorrelationID   string `json:"correlationId,omitempty"`
	Replayed        bool   `json:"replayed"`
}

func (h *ResourceClusterHandler) SubmitRuntimeIntent(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "The request body is invalid.")
		return
	}
	var envelope bffIntentEnvelope
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "The request body is invalid.")
		return
	}
	if envelope.APIVersion != "hnb.io/v1" {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "One or more request fields are invalid.", consoleViolation{Field: "apiVersion", Code: "INVALID_VALUE"})
		return
	}
	if !clusterIntentKinds[envelope.Kind] {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "One or more request fields are invalid.", consoleViolation{Field: "kind", Code: "UNSUPPORTED_VALUE"})
		return
	}
	if strings.TrimSpace(envelope.Metadata.IdempotencyKey) == "" {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "One or more request fields are invalid.", consoleViolation{Field: "metadata.idempotencyKey", Code: "REQUIRED"})
		return
	}
	if err := validateBFFClusterIntent(envelope); err != nil {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "One or more request fields are invalid.")
		return
	}
	if !uuidStr(envelope.Metadata.CorrelationID) {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "One or more request fields are invalid.", consoleViolation{Field: "metadata.correlationId", Code: "INVALID_FORMAT"})
		return
	}

	// Validate Header/Body consistency for Idempotency-Key and Correlation-ID
	headerIdempotency := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	headerCorrelation := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if headerIdempotency != "" && headerIdempotency != envelope.Metadata.IdempotencyKey {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "Header and body values do not match.", consoleViolation{Field: "metadata.idempotencyKey", Code: "HEADER_BODY_MISMATCH"})
		return
	}
	if headerCorrelation != "" && headerCorrelation != envelope.Metadata.CorrelationID {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "Header and body values do not match.", consoleViolation{Field: "metadata.correlationId", Code: "HEADER_BODY_MISMATCH"})
		return
	}
	if trusted.CorrelationID == "" || trusted.CorrelationID != envelope.Metadata.CorrelationID {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "Trusted request context and body values do not match.", consoleViolation{Field: "metadata.correlationId", Code: "CONTEXT_BODY_MISMATCH"})
		return
	}

	semanticDigest := bffIntentSemanticDigest(envelope)

	if h.platformURL != "" {
		h.forwardIntent(w, r, body, envelope, trusted, semanticDigest)
		return
	}

	writeUpstreamUnavailable(w, r)
}

// SubmitRuntimeIntentBatch is a controlled BFF for parent/child unmanage
// batches. It validates every target against the caller's tenant before the
// platform API creates child DeleteRuntimeTarget intents.
func (h *ResourceClusterHandler) SubmitRuntimeIntentBatch(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		response.BadRequest(w, "invalid batch body")
		return
	}
	var req struct {
		TargetIDs      []string `json:"targetIds"`
		IdempotencyKey string   `json:"idempotencyKey"`
		CorrelationID  string   `json:"correlationId"`
	}
	if json.Unmarshal(body, &req) != nil || len(req.TargetIDs) == 0 || len(req.TargetIDs) > 100 || req.IdempotencyKey == "" || !uuidStr(req.CorrelationID) || req.CorrelationID != trusted.CorrelationID {
		response.BadRequest(w, "invalid batch request")
		return
	}
	seen := map[string]bool{}
	for _, id := range req.TargetIDs {
		if !uuidStr(id) || seen[id] || !clusterAccessibleTo(h.db, id, trusted.TenantID, "") {
			response.NotFound(w, "cluster not found")
			return
		}
		seen[id] = true
	}
	if h.platformURL == "" || h.delegationSigner == nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{Scope: iam.DelegationScope{ResourceKind: string(iam.ResourceCluster)}, Action: iam.ActionDelete, IntentKind: "BatchDeleteRuntimeTargets", SemanticDigest: sha256Hex(body), CorrelationID: trusted.CorrelationID})
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.platformURL+"/v1/runtime-intent-batches", strings.NewReader(string(body)))
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Authorization", "Bearer "+token)
	upstream.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	resp, err := h.client.Do(upstream)
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	defer resp.Body.Close()
	copy, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(copy)
}

func bffIntentSemanticDigest(envelope bffIntentEnvelope) string {
	document := core.IntentSemanticDocument{
		APIVersion: envelope.APIVersion, Kind: envelope.Kind, TargetID: envelope.Spec.TargetID,
		TargetKind: envelope.Spec.TargetKind, ExpectedVersion: envelope.Spec.ExpectedVersion,
		BindingID: envelope.Spec.BindingID, BindingVersion: envelope.Spec.BindingVersion,
		OfferingID: envelope.Spec.OfferingID, OfferingVersion: envelope.Spec.OfferingVersion,
		StorageClassName: envelope.Spec.StorageClassName, StorageClassUID: envelope.Spec.StorageClassUID,
		StorageClassVersion: envelope.Spec.StorageClassResourceVersion,
		InstallationID:      envelope.Spec.InstallationID, PackageID: envelope.Spec.PackageID,
		PackageVersion: envelope.Spec.PackageVersion, CurrentVersion: envelope.Spec.CurrentVersion,
		DesiredVersion: envelope.Spec.DesiredVersion, DisplayName: envelope.Spec.DisplayName,
		KubernetesVersion: envelope.Spec.KubernetesVersion, CloudCoreEndpoint: envelope.Spec.CloudCoreEndpoint,
		VolumeID: envelope.Spec.VolumeID, WorkflowProviderRef: envelope.Spec.WorkflowProviderRef, PersistentVolume: envelope.Spec.PersistentVolume,
		PersistentVolumeClaim: envelope.Spec.PersistentVolumeClaim, PodDependencies: envelope.Spec.PodDependencies, StatefulSetDependencies: envelope.Spec.StatefulSetDependencies,
		NodeGroupMappings: envelope.Spec.NodeGroupMappings, Parameters: envelope.Spec.Parameters,
	}
	if envelope.Spec.CredentialSecretRef != nil {
		ref := bffCanonicalSecretReference(*envelope.Spec.CredentialSecretRef)
		document.CredentialSecretRef = &ref
	}
	for _, ref := range envelope.Spec.SecretReferences {
		document.SecretReferences = append(document.SecretReferences, bffCanonicalSecretReference(ref))
	}
	return core.IntentSemanticDigest(document)
}

func bffCanonicalSecretReference(ref bffIntentSecretRef) core.IntentSecretReference {
	return core.IntentSecretReference{Provider: ref.Provider, Scope: ref.Scope, Name: ref.Name, Version: ref.Version}
}

func validateBFFClusterIntent(envelope bffIntentEnvelope) error {
	for key := range envelope.Spec.Parameters {
		switch strings.ToLower(key) {
		case "tenant", "tenantid", "provider", "providerid", "step", "steps", "command", "commands":
			return fmt.Errorf("spec.parameters.%s is server-owned", key)
		}
	}
	if envelope.Spec.TargetKind != "KubernetesTarget" && envelope.Spec.TargetKind != "EdgeRuntimeTarget" {
		return errors.New("spec.targetKind must be KubernetesTarget or EdgeRuntimeTarget")
	}
	switch envelope.Kind {
	case "ImportRuntimeTarget":
		return nil
	case "UpgradeRuntimeTarget", "DeleteRuntimeTarget":
		if !uuidStr(envelope.Spec.TargetID) {
			return errors.New("spec.targetId must be a valid UUID")
		}
		if envelope.Spec.ExpectedVersion < 1 {
			return errors.New("spec.expectedVersion must be at least 1")
		}
		if envelope.Kind == "UpgradeRuntimeTarget" && strings.TrimSpace(envelope.Spec.DesiredVersion) == "" {
			return errors.New("spec.desiredVersion required")
		}
	}
	return nil
}

func uuidStr(s string) bool {
	if s == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

// forwardIntent sends the intent to the canonical platform-api endpoint and
// maps the response to the RuntimeIntentRecord contract.
func (h *ResourceClusterHandler) forwardIntent(w http.ResponseWriter, r *http.Request, body []byte, envelope bffIntentEnvelope, trusted iam.TrustedContext, semanticDigest string) {
	action, ok := iam.ClusterActionForIntentKind(envelope.Kind)
	if !ok || h.delegationSigner == nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{
		Scope: iam.DelegationScope{
			ResourceKind: string(iam.ResourceCluster), ResourceID: envelope.Spec.TargetID,
			ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
		},
		Action: action, IntentKind: envelope.Kind, SemanticDigest: semanticDigest,
		CorrelationID: trusted.CorrelationID,
	})
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.platformURL+"/v1/intents", strings.NewReader(string(body)))
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Semantic-Digest", semanticDigest)
	req.Header.Set("Authorization", "Bearer "+token)
	copyHeader(req.Header, r.Header, "X-Trace-Id")
	req.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	copyHeader(req.Header, r.Header, "Idempotency-Key")

	resp, err := h.client.Do(req)
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	if resp.StatusCode >= 400 {
		writeMappedUpstreamProblem(w, r, resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}
	var platform struct {
		IntentID       string `json:"intentId"`
		OperationID    string `json:"operationId"`
		PlanID         string `json:"planId"`
		Kind           string `json:"kind"`
		Status         string `json:"status"`
		CorrelationID  string `json:"correlationId"`
		CreatedAt      string `json:"createdAt"`
		SemanticDigest string `json:"semanticDigest"`
		Replayed       bool   `json:"replayed"`
	}
	if err := json.Unmarshal(respBody, &platform); err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	record := runtimeIntentRecord{
		APIVersion:      "ui.hnb.io/v1",
		IntentID:        platform.IntentID,
		Status:          mapPlatformStatus(platform.Status),
		SemanticDigest:  platform.SemanticDigest,
		ExecutionPlanID: platform.PlanID,
		OperationID:     platform.OperationID,
		CreatedAt:       platform.CreatedAt,
		CorrelationID:   platform.CorrelationID,
		Replayed:        platform.Replayed,
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSONRaw(w, record)
}

func mapPlatformStatus(status string) string {
	switch status {
	case "queued", "queued_offline", "pending", "pending_approval", "in_progress", "succeeded":
		return "operationCommitted"
	case "failed", "cancelled":
		return "rejected"
	default:
		return "planned"
	}
}

// RegisterSecret forwards a secret-registration request to the canonical
// platform-api endpoint. The request body is relayed verbatim; platform-api is
// the authority that validates, encrypts and persists the reference. The
// response (the resolved SecretReference) is passed through unchanged.
func (h *ResourceClusterHandler) RegisterSecret(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if h.platformURL == "" || h.delegationSigner == nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+64))
	if err != nil {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "The request body is invalid.")
		return
	}
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceSecret)},
		Action:        iam.ActionCreate,
		CorrelationID: trusted.CorrelationID,
	})
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.platformURL+"/v1/secrets:register", strings.NewReader(string(body)))
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	copyHeader(req.Header, r.Header, "X-Trace-Id")

	resp, err := h.client.Do(req)
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	if resp.StatusCode >= 400 {
		writeMappedUpstreamProblem(w, r, resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}
	// Pass the registered reference through unchanged (already shaped by
	// platform-api to the console contract).
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(respBody)
}

// UpdateClusterDescription is the BFF for the console's cluster description
// editor. It signs a trusted-service delegation scoped to the cluster and
// forwards to platform-api, which updates the runtime target read model.
func (h *ResourceClusterHandler) UpdateClusterDescription(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if h.platformURL == "" || h.delegationSigner == nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	id := r.PathValue("id")
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<10))
	if err != nil {
		writeLocalClusterProblem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", "The request body is invalid.")
		return
	}
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceClusterMetadata), ResourceID: id},
		Action:        iam.ActionUpdate,
		CorrelationID: trusted.CorrelationID,
	})
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPatch, h.platformURL+"/v1/clusters/"+id+"/description", strings.NewReader(string(body)))
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	copyHeader(req.Header, r.Header, "X-Trace-Id")

	resp, err := h.client.Do(req)
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	if resp.StatusCode >= 400 {
		writeMappedUpstreamProblem(w, r, resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}

// DownloadKubeConfig is the BFF for the console's kubeconfig download action.
// It signs a read-scoped trusted-service delegation and forwards to
// platform-api, which returns the stored kubeconfig for the Kubernetes cluster.
func (h *ResourceClusterHandler) DownloadKubeConfig(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if h.platformURL == "" || h.delegationSigner == nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	id := r.PathValue("id")
	token, err := h.delegationSigner.Sign(r.Context(), trusted, iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceClusterMetadata), ResourceID: id},
		Action:        iam.ActionExecute,
		CorrelationID: trusted.CorrelationID,
	})
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.platformURL+"/v1/clusters/"+id+"/kubeconfig:issue", nil)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", trusted.CorrelationID)
	copyHeader(req.Header, r.Header, "X-Trace-Id")

	resp, err := h.client.Do(req)
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeUpstreamUnavailable(w, r)
		return
	}
	if resp.StatusCode >= 400 {
		writeMappedUpstreamProblem(w, r, resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if resp.Header.Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}

func intentKindToOperationType(kind string) string {
	switch kind {
	case "UninstallRelease", "DeleteRuntimeTarget":
		return "delete"
	case "UpgradeRelease", "UpgradeRuntimeTarget":
		return "upgrade"
	case "RollbackRelease":
		return "rollback"
	case "ChangeConfiguration":
		return "config_change"
	default:
		return "deploy"
	}
}

// persistIntent records the intent and creates a queued Operation in the
// standalone (no platform-api) mode. Returns the RuntimeIntentRecord.
func (h *ResourceClusterHandler) persistIntent(w http.ResponseWriter, r *http.Request, envelope bffIntentEnvelope, trusted iam.TrustedContext, semanticDigest string) {
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer tx.Rollback()

	paramsJSON, _ := json.Marshal(envelope.Spec.Parameters)
	secrets := append([]bffIntentSecretRef(nil), envelope.Spec.SecretReferences...)
	if envelope.Spec.CredentialSecretRef != nil {
		secrets = append(secrets, *envelope.Spec.CredentialSecretRef)
	}
	secretsJSON, _ := json.Marshal(secrets)
	targetRef := envelope.Spec.TargetID
	if targetRef == "" {
		targetRef = "pending:" + envelope.Metadata.IdempotencyKey
	}
	scopeRef := "tenant:" + trusted.TenantID

	operationType := intentKindToOperationType(envelope.Kind)
	initialStatus := "queued"
	if operationType == "delete" {
		initialStatus = "pending_approval"
	}
	operationID := uuid.New().String()
	status := "operation_committed"

	// The intent body must be replayed to the caller, so capture it.
	intentJSON, _ := json.Marshal(envelope)
	_ = semanticDigest // already computed before call

	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO bff_runtime_intents (
			api_version, kind, tenant_id, idempotency_key, correlation_id,
			target_ref, scope_ref, parameters, secret_references,
			status, operation_id, semantic_digest
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`,
		envelope.APIVersion, envelope.Kind, trusted.TenantID,
		envelope.Metadata.IdempotencyKey, envelope.Metadata.CorrelationID,
		targetRef, scopeRef,
		string(paramsJSON), string(secretsJSON),
		status, operationID, semanticDigest)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}

	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO operations (
			id, tenant_id, plan_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, plan_digest, total_steps, tags
		) VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`,
		operationID, trusted.TenantID, operationType, initialStatus, trusted.SubjectID,
		envelope.Metadata.CorrelationID, envelope.Metadata.IdempotencyKey,
		semanticDigest, 0, fmt.Sprintf(`{"intent_kind":%q}`, envelope.Kind))
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		response.InternalError(w, err.Error())
		return
	}

	record := runtimeIntentRecord{
		IntentID:       envelope.Metadata.IdempotencyKey,
		Status:         "operationCommitted",
		SemanticDigest: semanticDigest,
		Intent:         json.RawMessage(intentJSON),
		OperationID:    operationID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	writeJSONRaw(w, record)
}

func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n := 0
	_, err := fmt.Sscanf(value, "%d", &n)
	if err != nil {
		return fallback
	}
	return n
}

func writeJSONRaw(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

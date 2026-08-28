package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

const storageSchemaVersion = "1.0.0"

var storageEpoch = time.Unix(0, 0).UTC()

type StorageHandler struct{ store storageStore }

func NewStorageHandler(store storageStore) *StorageHandler { return &StorageHandler{store: store} }

type storageCondition struct {
	SchemaVersion string    `json:"schemaVersion"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message,omitempty"`
	Source        string    `json:"source"`
	ObservedAt    time.Time `json:"observedAt"`
	Freshness     string    `json:"freshness"`
}

type storageInventoryResponse struct {
	SchemaVersion        string              `json:"schemaVersion"`
	TenantID             string              `json:"tenantId"`
	TargetID             string              `json:"targetId"`
	Source               string              `json:"source"`
	ObservedAt           time.Time           `json:"observedAt"`
	Freshness            string              `json:"freshness"`
	StorageClasses       []storageClassItem  `json:"storageClasses"`
	CSIDrivers           []csiDriverItem     `json:"csiDrivers"`
	CSINodes             []csiNodeItem       `json:"csiNodes"`
	CSIStorageCapacities []csiCapacityItem   `json:"csiStorageCapacities"`
	VolumeAttachments    []volumeAttachItem  `json:"volumeAttachments"`
	SnapshotAPI          snapshotAPIResponse `json:"snapshotApi"`
}

type storageClassItem struct {
	UID                  string             `json:"uid"`
	ResourceVersion      string             `json:"resourceVersion"`
	Name                 string             `json:"name"`
	Provisioner          string             `json:"provisioner"`
	ReclaimPolicy        string             `json:"reclaimPolicy"`
	VolumeBindingMode    string             `json:"volumeBindingMode"`
	AllowVolumeExpansion bool               `json:"allowVolumeExpansion"`
	IsDefault            bool               `json:"isDefault"`
	Source               string             `json:"source"`
	ObservedAt           time.Time          `json:"observedAt"`
	Freshness            string             `json:"freshness"`
	Conditions           []storageCondition `json:"conditions"`
}

type csiDriverItem struct {
	UID             string             `json:"uid"`
	ResourceVersion string             `json:"resourceVersion"`
	Name            string             `json:"name"`
	AttachRequired  bool               `json:"attachRequired"`
	PodInfoOnMount  bool               `json:"podInfoOnMount"`
	StorageCapacity bool               `json:"storageCapacity"`
	Source          string             `json:"source"`
	ObservedAt      time.Time          `json:"observedAt"`
	Freshness       string             `json:"freshness"`
	Conditions      []storageCondition `json:"conditions"`
}

type csiNodeItem struct {
	UID             string    `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	Name            string    `json:"name"`
	Drivers         []string  `json:"drivers"`
	Source          string    `json:"source"`
	ObservedAt      time.Time `json:"observedAt"`
	Freshness       string    `json:"freshness"`
}

type csiCapacityItem struct {
	UID              string    `json:"uid"`
	ResourceVersion  string    `json:"resourceVersion"`
	Name             string    `json:"name"`
	Namespace        string    `json:"namespace"`
	StorageClassName string    `json:"storageClassName"`
	Status           string    `json:"status"`
	Value            *int64    `json:"value,omitempty"`
	Unit             string    `json:"unit"`
	Source           string    `json:"source"`
	ObservedAt       time.Time `json:"observedAt"`
	Freshness        string    `json:"freshness"`
}

type volumeAttachItem struct {
	UID                  string    `json:"uid"`
	ResourceVersion      string    `json:"resourceVersion"`
	Name                 string    `json:"name"`
	DriverName           string    `json:"driverName"`
	NodeName             string    `json:"nodeName"`
	PersistentVolumeName string    `json:"persistentVolumeName"`
	Attached             bool      `json:"attached"`
	Source               string    `json:"source"`
	ObservedAt           time.Time `json:"observedAt"`
	Freshness            string    `json:"freshness"`
}

type snapshotAPIResponse struct {
	Status     string    `json:"status"`
	APIVersion string    `json:"apiVersion,omitempty"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observedAt"`
	Freshness  string    `json:"freshness"`
}

func (h *StorageHandler) Overview(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	projection, err := h.store.Overview(r.Context(), tenantID)
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_READ_FAILED", "Storage projection could not be read.")
		return
	}
	observedAt, freshness := storageEpoch, "Unknown"
	if projection.ObservedAt != nil {
		observedAt = *projection.ObservedAt
		freshness = "Stale"
		if projection.Fresh {
			freshness = "Fresh"
		}
	}
	writeStorageJSON(w, map[string]any{
		"schemaVersion": storageSchemaVersion, "source": "runtime_target_storage_inventory",
		"observedAt": observedAt, "freshness": freshness,
		"counts":         map[string]int{"backends": 0, "offerings": 0, "driverInstallations": 0, "targets": projection.Targets, "bindings": 0},
		"capacityStates": map[string]int{"Known": projection.KnownCapacity, "Elastic": 0, "Unknown": 0, "NotReported": projection.NotReportedCapacity},
	})
}

func (h *StorageHandler) Backends(w http.ResponseWriter, r *http.Request) {
	h.emptyList(w, r, map[string]bool{"providerType": true, "healthState": true, "keyword": true})
}
func (h *StorageHandler) Offerings(w http.ResponseWriter, r *http.Request) {
	h.emptyList(w, r, map[string]bool{"scope": true, "serviceMode": true, "keyword": true})
}
func (h *StorageHandler) DriverInstallations(w http.ResponseWriter, r *http.Request) {
	h.emptyList(w, r, map[string]bool{"targetId": true, "healthState": true, "freshness": true, "keyword": true})
}

func (h *StorageHandler) OfferingBindings(w http.ResponseWriter, r *http.Request) {
	if _, ok := storageTenant(r); !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if _, err := uuid.Parse(r.PathValue("offeringId")); err != nil {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_OFFERING_NOT_FOUND", "Storage offering was not found.")
		return
	}
	if !validStorageListQuery(r, map[string]bool{"targetId": true, "syncState": true, "keyword": true}) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Pagination or filter parameters are invalid.")
		return
	}
	// Offering desired state is introduced by task 4.1. Do not infer bindings from StorageClasses.
	writeStorageJSON(w, map[string]any{"schemaVersion": storageSchemaVersion, "items": []any{}, "total": 0})
}

func (h *StorageHandler) emptyList(w http.ResponseWriter, r *http.Request, filters map[string]bool) {
	if _, ok := storageTenant(r); !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	if !validStorageListQuery(r, filters) {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Pagination or filter parameters are invalid.")
		return
	}
	writeStorageJSON(w, map[string]any{"schemaVersion": storageSchemaVersion, "items": []any{}, "total": 0})
}

func (h *StorageHandler) TargetInventory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	targetID := r.PathValue("targetId")
	if _, err := uuid.Parse(targetID); err != nil {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_TARGET_NOT_FOUND", "Storage target was not found.")
		return
	}
	filter, ok := parseStorageInventoryQuery(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Pagination or filter parameters are invalid.")
		return
	}
	owned, err := h.store.TargetOwned(r.Context(), tenantID, targetID)
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_READ_FAILED", "Storage projection could not be read.")
		return
	}
	if !owned {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_TARGET_NOT_FOUND", "Storage target was not found.")
		return
	}
	rows, registrations, snapshot, err := h.store.Inventory(r.Context(), tenantID, targetID, filter)
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_READ_FAILED", "Storage projection could not be read.")
		return
	}
	result := newEmptyStorageInventory(tenantID, targetID)
	now := time.Now()
	for _, row := range rows {
		if row.ObservedAt.After(result.ObservedAt) {
			result.ObservedAt, result.Source = row.ObservedAt.UTC(), row.Source
		}
		if !row.StaleAfter.After(now) {
			result.Freshness = "Stale"
		} else if result.Freshness == "Unknown" {
			result.Freshness = "Fresh"
		}
		if err := appendStorageProjection(&result, row, registrations, now); err != nil {
			writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_INVALID", "Storage projection contains invalid normalized data.")
			return
		}
	}
	if snapshot != nil {
		status := snapshot.Status
		if status == "Installed" {
			status = "Supported"
		}
		result.SnapshotAPI = snapshotAPIResponse{Status: status, APIVersion: snapshot.APIVersion, Source: snapshot.Source,
			ObservedAt: snapshot.ObservedAt.UTC(), Freshness: projectionFreshness(snapshot.StaleAfter, now)}
		if snapshot.ObservedAt.After(result.ObservedAt) {
			result.ObservedAt, result.Source = snapshot.ObservedAt.UTC(), snapshot.Source
		}
		if result.Freshness == "Unknown" {
			result.Freshness = result.SnapshotAPI.Freshness
		} else if result.SnapshotAPI.Freshness == "Stale" {
			result.Freshness = "Stale"
		}
	}
	writeStorageJSON(w, result)
}

func (h *StorageHandler) TargetMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := storageTenant(r)
	if !ok {
		writeStorageProblem(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
		return
	}
	targetID := r.PathValue("targetId")
	if _, err := uuid.Parse(targetID); err != nil {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_TARGET_NOT_FOUND", "Storage target was not found.")
		return
	}
	owned, err := h.store.TargetOwned(r.Context(), tenantID, targetID)
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_READ_FAILED", "Storage projection could not be read.")
		return
	}
	if !owned {
		writeStorageProblem(w, r, http.StatusNotFound, "STORAGE_TARGET_NOT_FOUND", "Storage target was not found.")
		return
	}
	rows, err := h.store.Metrics(r.Context(), tenantID, targetID)
	if err != nil {
		writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_READ_FAILED", "Storage metric projection could not be read.")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var metrics []map[string]any
		if err := json.Unmarshal(row.Metrics, &metrics); err != nil || len(metrics) != 6 {
			writeStorageProblem(w, r, http.StatusInternalServerError, "STORAGE_PROJECTION_INVALID", "Storage metric projection contains invalid normalized data.")
			return
		}
		if !row.StaleAfter.After(time.Now()) {
			for _, metric := range metrics {
				metric["freshness"] = "Stale"
			}
		}
		items = append(items, map[string]any{"schemaVersion": storageSchemaVersion, "providerId": row.ProviderID, "targetId": targetID,
			"resourceKind": row.ResourceKind, "resourceUid": row.ResourceUID, "metrics": metrics})
	}
	writeStorageJSON(w, map[string]any{"schemaVersion": storageSchemaVersion, "items": items, "total": len(items)})
}

func appendStorageProjection(result *storageInventoryResponse, row storageProjectionRow, registrations map[string]bool, now time.Time) error {
	freshness := projectionFreshness(row.StaleAfter, now)
	switch row.Kind {
	case "StorageClass":
		var raw struct {
			Provisioner, ReclaimPolicy, VolumeBindingMode string
			AllowVolumeExpansion                          *bool `json:"allowVolumeExpansion"`
			IsDefault                                     *bool `json:"isDefault"`
		}
		if err := json.Unmarshal(row.Attributes, &raw); err != nil {
			return err
		}
		conditions := []storageCondition{}
		if raw.Provisioner != "" && !registrations[raw.Provisioner] {
			conditions = append(conditions, storageCondition{SchemaVersion: storageSchemaVersion, Type: "DriverRegistered", Status: "False",
				Reason: "MissingDriverRegistration", Message: "No active CSIDriver registration was observed for this provisioner.",
				Source: "runtime_target_storage_driver_evidence", ObservedAt: row.ObservedAt.UTC(), Freshness: freshness})
		}
		result.StorageClasses = append(result.StorageClasses, storageClassItem{UID: row.UID, ResourceVersion: row.ResourceVersion, Name: row.Name,
			Provisioner: raw.Provisioner, ReclaimPolicy: defaultString(raw.ReclaimPolicy, "Delete"), VolumeBindingMode: defaultString(raw.VolumeBindingMode, "Immediate"),
			AllowVolumeExpansion: boolValue(raw.AllowVolumeExpansion), IsDefault: boolValue(raw.IsDefault), Source: row.Source,
			ObservedAt: row.ObservedAt.UTC(), Freshness: freshness, Conditions: conditions})
	case "CSIDriver":
		var raw struct {
			AttachRequired  *bool `json:"attachRequired"`
			PodInfoOnMount  *bool `json:"podInfoOnMount"`
			StorageCapacity *bool `json:"storageCapacity"`
		}
		if err := json.Unmarshal(row.Attributes, &raw); err != nil {
			return err
		}
		result.CSIDrivers = append(result.CSIDrivers, csiDriverItem{UID: row.UID, ResourceVersion: row.ResourceVersion, Name: row.Name,
			AttachRequired: boolValue(raw.AttachRequired), PodInfoOnMount: boolValue(raw.PodInfoOnMount), StorageCapacity: boolValue(raw.StorageCapacity),
			Source: row.Source, ObservedAt: row.ObservedAt.UTC(), Freshness: freshness, Conditions: []storageCondition{}})
	case "CSINode":
		var raw struct {
			Drivers []struct {
				Name string `json:"name"`
			} `json:"drivers"`
		}
		if err := json.Unmarshal(row.Attributes, &raw); err != nil {
			return err
		}
		drivers := make([]string, 0, len(raw.Drivers))
		for _, driver := range raw.Drivers {
			drivers = append(drivers, driver.Name)
		}
		result.CSINodes = append(result.CSINodes, csiNodeItem{UID: row.UID, ResourceVersion: row.ResourceVersion, Name: row.Name,
			Drivers: drivers, Source: row.Source, ObservedAt: row.ObservedAt.UTC(), Freshness: freshness})
	case "CSIStorageCapacity":
		var raw struct {
			StorageClassName string `json:"storageClassName"`
			CapacityBytes    *int64 `json:"capacityBytes"`
		}
		if err := json.Unmarshal(row.Attributes, &raw); err != nil {
			return err
		}
		status := "NotReported"
		if raw.CapacityBytes != nil {
			status = "Known"
		}
		result.CSIStorageCapacities = append(result.CSIStorageCapacities, csiCapacityItem{UID: row.UID, ResourceVersion: row.ResourceVersion,
			Name: row.Name, Namespace: row.Namespace, StorageClassName: raw.StorageClassName, Status: status, Value: raw.CapacityBytes,
			Unit: "By", Source: row.Source, ObservedAt: row.ObservedAt.UTC(), Freshness: freshness})
	case "VolumeAttachment":
		var raw struct {
			Attacher             string `json:"attacher"`
			NodeName             string `json:"nodeName"`
			PersistentVolumeName string `json:"persistentVolumeName"`
			Attached             *bool  `json:"attached"`
		}
		if err := json.Unmarshal(row.Attributes, &raw); err != nil {
			return err
		}
		result.VolumeAttachments = append(result.VolumeAttachments, volumeAttachItem{UID: row.UID, ResourceVersion: row.ResourceVersion,
			Name: row.Name, DriverName: raw.Attacher, NodeName: raw.NodeName, PersistentVolumeName: raw.PersistentVolumeName,
			Attached: boolValue(raw.Attached), Source: row.Source, ObservedAt: row.ObservedAt.UTC(), Freshness: freshness})
	}
	return nil
}

func newEmptyStorageInventory(tenantID, targetID string) storageInventoryResponse {
	return storageInventoryResponse{SchemaVersion: storageSchemaVersion, TenantID: tenantID, TargetID: targetID,
		Source: "runtime_target_storage_inventory", ObservedAt: storageEpoch, Freshness: "Unknown",
		StorageClasses: []storageClassItem{}, CSIDrivers: []csiDriverItem{}, CSINodes: []csiNodeItem{},
		CSIStorageCapacities: []csiCapacityItem{}, VolumeAttachments: []volumeAttachItem{},
		SnapshotAPI: snapshotAPIResponse{Status: "Unknown", Source: "runtime_target_storage_snapshot_api", ObservedAt: storageEpoch, Freshness: "Unknown"}}
}

func parseStorageInventoryQuery(r *http.Request) (storageInventoryQuery, bool) {
	page, pageSize, ok := parseStoragePage(r)
	if !ok {
		return storageInventoryQuery{}, false
	}
	allowed := map[string]bool{"page": true, "pageSize": true, "kind": true, "name": true, "driverName": true, "freshness": true}
	for key := range r.URL.Query() {
		if !allowed[key] {
			return storageInventoryQuery{}, false
		}
	}
	kind := r.URL.Query().Get("kind")
	validKinds := map[string]bool{"": true, "StorageClass": true, "CSIDriver": true, "CSINode": true, "CSIStorageCapacity": true, "VolumeAttachment": true}
	freshness := r.URL.Query().Get("freshness")
	if !validKinds[kind] || (freshness != "" && freshness != "Fresh" && freshness != "Stale") {
		return storageInventoryQuery{}, false
	}
	name, driver := strings.TrimSpace(r.URL.Query().Get("name")), strings.TrimSpace(r.URL.Query().Get("driverName"))
	if len(name) > 253 || len(driver) > 253 {
		return storageInventoryQuery{}, false
	}
	return storageInventoryQuery{Kind: kind, Name: name, DriverName: driver, Freshness: freshness, Limit: pageSize, Offset: (page - 1) * pageSize}, true
}

func validStorageListQuery(r *http.Request, filters map[string]bool) bool {
	if _, _, ok := parseStoragePage(r); !ok {
		return false
	}
	for key, values := range r.URL.Query() {
		if key != "page" && key != "pageSize" && !filters[key] {
			return false
		}
		for _, value := range values {
			if len(value) > 256 {
				return false
			}
		}
	}
	return true
}

func parseStoragePage(r *http.Request) (int, int, bool) {
	page, pageSize := 1, 100
	var err error
	if value := r.URL.Query().Get("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, false
		}
	}
	if value := r.URL.Query().Get("pageSize"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, false
		}
	}
	return page, pageSize, page >= 1 && pageSize >= 1 && pageSize <= 1000
}

func storageTenant(r *http.Request) (string, bool) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	return trusted.TenantID, ok && trusted.TenantID != ""
}
func projectionFreshness(staleAfter, now time.Time) string {
	if staleAfter.After(now) {
		return "Fresh"
	}
	return "Stale"
}
func boolValue(value *bool) bool { return value != nil && *value }
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func writeStorageJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(value)
}

func writeStorageProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	correlationID := strings.ToLower(r.Header.Get("X-Correlation-ID"))
	if parsed, err := uuid.Parse(correlationID); err != nil || parsed.String() != correlationID {
		correlationID = uuid.NewString()
	}
	traceID := strings.ReplaceAll(correlationID, "-", "")
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Correlation-ID", correlationID)
	w.Header().Set("X-Trace-Id", traceID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://hnb.cloud/problems/" + strings.ToLower(strings.ReplaceAll(code, "_", "-")),
		"title": http.StatusText(status), "status": status, "detail": detail, "code": code, "correlationId": correlationID, "traceId": traceID})
}

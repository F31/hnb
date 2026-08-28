package observer

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// The types below mirror the published runtime-target observation and
// source-reset JSON Schemas (contracts/schema/runtime-target/v1). They are
// intentionally local to the projector so it does not depend on the generated
// contracts Go module (which pins a newer Go toolchain than this workspace).

const (
	ObservationSchemaVersion = "1.0.0"
	MaxObservationPayload    = 1 << 20
	MaxClockSkew             = 300 * time.Second
)

type observationWire struct {
	SchemaVersion      string            `json:"schemaVersion"`
	EventID            string            `json:"eventId"`
	TenantID           string            `json:"tenantId"`
	TargetID           string            `json:"targetId"`
	TargetKind         string            `json:"targetKind"`
	ObserverID         string            `json:"observerId"`
	ObserverKind       string            `json:"observerKind"`
	ObserverGeneration int64             `json:"observerGeneration"`
	Sequence           int64             `json:"sequence"`
	ObservedAt         time.Time         `json:"observedAt"`
	InventoryMode      string            `json:"inventoryMode"`
	Target             *targetWire       `json:"target,omitempty"`
	Capability         *capabilityWire   `json:"capability,omitempty"`
	Nodes              []nodeWire        `json:"nodes,omitempty"`
	StorageInventory   *StorageInventory `json:"storageInventory,omitempty"`
}

type targetWire struct {
	LifecycleState        string    `json:"lifecycleState"`
	HealthState           string    `json:"healthState"`
	ConnectivityState     string    `json:"connectivityState"`
	LastKnownStateAt      time.Time `json:"lastKnownStateAt"`
	StaleThresholdSeconds int64     `json:"staleThresholdSeconds"`
	RuntimeVersion        string    `json:"runtimeVersion,omitempty"`
}

type capabilityWire struct {
	SnapshotID         string         `json:"snapshotId"`
	Digest             string         `json:"digest"`
	KubernetesVersion  string         `json:"kubernetesVersion,omitempty"`
	KubeEdgeVersion    string         `json:"kubeEdgeVersion,omitempty"`
	RuntimeVersion     string         `json:"runtimeVersion"`
	Architectures      []string       `json:"architectures"`
	Resources          resourceWire   `json:"resources"`
	CniPlugins         []string       `json:"cniPlugins,omitempty"`
	CsiDrivers         []string       `json:"csiDrivers,omitempty"`
	GatewayApiVersions []string       `json:"gatewayApiVersions,omitempty"`
	SecurityFeatures   []string       `json:"securityFeatures,omitempty"`
	StorageClasses     []string       `json:"storageClasses,omitempty"`
	Extra              map[string]any `json:"-"`
}

type resourceWire struct {
	CpuMillis   int64 `json:"cpuMillis"`
	MemoryBytes int64 `json:"memoryBytes"`
	GpuCount    int64 `json:"gpuCount,omitempty"`
	NpuCount    int64 `json:"npuCount,omitempty"`
}

type nodeWire struct {
	NodeID            string            `json:"nodeId"`
	Name              string            `json:"name,omitempty"`
	LifecycleState    string            `json:"lifecycleState"`
	HealthState       string            `json:"healthState"`
	ConnectivityState string            `json:"connectivityState"`
	Freshness         string            `json:"freshness"`
	ObservedAt        time.Time         `json:"observedAt"`
	LastKnownStateAt  time.Time         `json:"lastKnownStateAt"`
	Deleted           bool              `json:"deleted,omitempty"`
	RuntimeVersion    string            `json:"runtimeVersion,omitempty"`
	KubeletVersion    string            `json:"kubeletVersion,omitempty"`
	Architecture      string            `json:"architecture,omitempty"`
	Resources         resourceWire      `json:"resources"`
	Labels            map[string]string `json:"labels,omitempty"`
}

type sourceResetWire struct {
	SchemaVersion              string    `json:"schemaVersion"`
	EventID                    string    `json:"eventId"`
	TenantID                   string    `json:"tenantId"`
	TargetID                   string    `json:"targetId"`
	TargetKind                 string    `json:"targetKind"`
	ObserverID                 string    `json:"observerId"`
	ObserverKind               string    `json:"observerKind"`
	PreviousObserverGeneration int64     `json:"previousObserverGeneration"`
	NewObserverGeneration      int64     `json:"newObserverGeneration"`
	ObservedAt                 time.Time `json:"observedAt"`
	ObserverLeaseID            string    `json:"observerLeaseId"`
	Reason                     string    `json:"reason"`
}

// ValidateObservation parses and validates a runtime-target observation payload
// against the observer identity. It enforces schema version, identity binding,
// payload bounds, and clock skew before any projection.
func ValidateObservation(id Identity, payload []byte) (*Observation, error) {
	if !id.Valid() {
		return nil, fmt.Errorf("observer identity is incomplete")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty observation payload")
	}
	if len(payload) > MaxObservationPayload {
		return nil, fmt.Errorf("observation payload exceeds %d bytes", MaxObservationPayload)
	}
	if err := rejectUnknownFields(payload, observationAllowedFields); err != nil {
		return nil, err
	}
	var raw observationWire
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("invalid observation JSON: %w", err)
	}
	if raw.SchemaVersion != ObservationSchemaVersion {
		return nil, fmt.Errorf("unsupported observation schema version %q", raw.SchemaVersion)
	}
	if _, err := uuid.Parse(raw.EventID); err != nil {
		return nil, fmt.Errorf("eventId must be a UUID")
	}
	if _, err := uuid.Parse(raw.TargetID); err != nil {
		return nil, fmt.Errorf("targetId must be a UUID")
	}
	if raw.ObserverGeneration < 1 || raw.Sequence < 1 {
		return nil, fmt.Errorf("observerGeneration and sequence must be >= 1")
	}
	if raw.InventoryMode != "Full" && raw.InventoryMode != "Delta" {
		return nil, fmt.Errorf("inventoryMode must be Full or Delta")
	}
	if raw.ObservedAt.IsZero() {
		return nil, fmt.Errorf("observedAt is required")
	}
	if raw.ObservedAt.After(time.Now().UTC().Add(MaxClockSkew)) {
		return nil, fmt.Errorf("observedAt is too far in the future")
	}
	if err := validateKindSource(raw.TargetKind, raw.ObserverKind); err != nil {
		return nil, err
	}
	if err := validateIdentityBinding(id, raw); err != nil {
		return nil, err
	}
	o := &Observation{
		SchemaVersion:      raw.SchemaVersion,
		EventID:            raw.EventID,
		TenantID:           raw.TenantID,
		TargetID:           raw.TargetID,
		TargetKind:         raw.TargetKind,
		ObserverID:         raw.ObserverID,
		ObserverKind:       raw.ObserverKind,
		ObserverGeneration: raw.ObserverGeneration,
		Sequence:           raw.Sequence,
		ObservedAt:         raw.ObservedAt,
		InventoryMode:      raw.InventoryMode,
		StorageInventory:   raw.StorageInventory,
	}
	if raw.Target != nil {
		o.Target = &TargetState{
			LifecycleState:        raw.Target.LifecycleState,
			HealthState:           raw.Target.HealthState,
			ConnectivityState:     raw.Target.ConnectivityState,
			LastKnownStateAt:      raw.Target.LastKnownStateAt,
			StaleThresholdSeconds: raw.Target.StaleThresholdSeconds,
			RuntimeVersion:        raw.Target.RuntimeVersion,
		}
	}
	if raw.Capability != nil {
		content, err := capabilityContent(raw.Capability)
		if err != nil {
			return nil, err
		}
		o.Capability = &Capability{SnapshotID: raw.Capability.SnapshotID, Digest: raw.Capability.Digest, Content: content}
	}
	for _, node := range raw.Nodes {
		o.Nodes = append(o.Nodes, Node{
			NodeID:            node.NodeID,
			Name:              node.Name,
			LifecycleState:    node.LifecycleState,
			HealthState:       node.HealthState,
			ConnectivityState: node.ConnectivityState,
			Freshness:         node.Freshness,
			ObservedAt:        node.ObservedAt,
			LastKnownStateAt:  node.LastKnownStateAt,
			Deleted:           node.Deleted,
			RuntimeVersion:    node.RuntimeVersion,
			KubeletVersion:    node.KubeletVersion,
			Architecture:      node.Architecture,
			Resources: map[string]any{
				"cpuMillis": node.Resources.CpuMillis, "memoryBytes": node.Resources.MemoryBytes,
				"gpuCount": node.Resources.GpuCount, "npuCount": node.Resources.NpuCount,
			},
			Labels: node.Labels,
		})
	}
	if o.InventoryMode == "Full" {
		for _, node := range o.Nodes {
			if node.Deleted {
				return nil, fmt.Errorf("full inventory cannot declare a deleted node %q", node.NodeID)
			}
		}
	}
	if o.StorageInventory != nil {
		if o.TargetKind != "KubernetesTarget" || o.ObserverKind != "Agent" {
			return nil, fmt.Errorf("storageInventory is only valid for KubernetesTarget Agent observations")
		}
		if err := validateStorageInventory(o.StorageInventory, o.InventoryMode); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func validateStorageInventory(inventory *StorageInventory, mode string) error {
	if mode == "Full" && (inventory.StorageClasses == nil || inventory.CSIDrivers == nil || inventory.CSINodes == nil ||
		inventory.CSIStorageCapacities == nil || inventory.VolumeAttachments == nil || inventory.SnapshotAPI == nil) {
		return fmt.Errorf("full storageInventory requires all core resource collections")
	}
	if inventory.StorageClasses == nil && inventory.CSIDrivers == nil && inventory.CSINodes == nil &&
		inventory.CSIStorageCapacities == nil && inventory.VolumeAttachments == nil &&
		inventory.VolumeSnapshotClasses == nil && inventory.VolumeSnapshots == nil && inventory.VolumeSnapshotContents == nil && inventory.SnapshotAPI == nil {
		return fmt.Errorf("storageInventory must contain at least one resource collection")
	}
	if inventory.SnapshotAPI != nil {
		if inventory.SnapshotAPI.Status != "Installed" && inventory.SnapshotAPI.Status != "NotInstalled" && inventory.SnapshotAPI.Status != "Unsupported" {
			return fmt.Errorf("storageInventory snapshotApi has invalid status %q", inventory.SnapshotAPI.Status)
		}
		if inventory.SnapshotAPI.Status == "Installed" && inventory.SnapshotAPI.APIVersion == "" {
			return fmt.Errorf("installed snapshotApi requires apiVersion")
		}
		if inventory.SnapshotAPI.Source == "" || inventory.SnapshotAPI.ObservedAt.IsZero() || inventory.SnapshotAPI.ObservedAt.After(time.Now().UTC().Add(MaxClockSkew)) {
			return fmt.Errorf("storageInventory snapshotApi requires valid source and observedAt")
		}
	}
	resources := []struct {
		kind       string
		identities []KubernetesResourceIdentity
	}{
		{"StorageClass", identities(inventory.StorageClasses, func(v StorageClassFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"CSIDriver", identities(inventory.CSIDrivers, func(v CSIDriverFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"CSINode", identities(inventory.CSINodes, func(v CSINodeFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"CSIStorageCapacity", identities(inventory.CSIStorageCapacities, func(v CSIStorageCapacityFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"VolumeAttachment", identities(inventory.VolumeAttachments, func(v VolumeAttachmentFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"VolumeSnapshotClass", identities(inventory.VolumeSnapshotClasses, func(v VolumeSnapshotClassFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"VolumeSnapshot", identities(inventory.VolumeSnapshots, func(v VolumeSnapshotFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
		{"VolumeSnapshotContent", identities(inventory.VolumeSnapshotContents, func(v VolumeSnapshotContentFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity })},
	}
	for _, group := range resources {
		seen := make(map[string]struct{}, len(group.identities))
		for _, resource := range group.identities {
			if resource.UID == "" || resource.ResourceVersion == "" || resource.Name == "" || resource.Source == "" || resource.ObservedAt.IsZero() {
				return fmt.Errorf("storageInventory %s requires uid, resourceVersion, name, source, and observedAt", group.kind)
			}
			if resource.ObservedAt.After(time.Now().UTC().Add(MaxClockSkew)) {
				return fmt.Errorf("storageInventory %s %q observedAt is too far in the future", group.kind, resource.UID)
			}
			if mode == "Full" && resource.Deleted {
				return fmt.Errorf("full inventory cannot declare a deleted %s %q", group.kind, resource.UID)
			}
			if _, ok := seen[resource.UID]; ok {
				return fmt.Errorf("storageInventory contains duplicate %s UID %q", group.kind, resource.UID)
			}
			seen[resource.UID] = struct{}{}
		}
	}
	return nil
}

func identities[T any](values []T, identity func(T) KubernetesResourceIdentity) []KubernetesResourceIdentity {
	result := make([]KubernetesResourceIdentity, 0, len(values))
	for _, value := range values {
		result = append(result, identity(value))
	}
	return result
}

// ValidateSourceReset parses and validates a source-reset control message.
func ValidateSourceReset(id Identity, payload []byte) (*SourceReset, error) {
	if !id.Valid() {
		return nil, fmt.Errorf("observer identity is incomplete")
	}
	if len(payload) == 0 || len(payload) > MaxObservationPayload {
		return nil, fmt.Errorf("invalid source-reset payload size")
	}
	if err := rejectUnknownFields(payload, sourceResetAllowedFields); err != nil {
		return nil, err
	}
	var raw sourceResetWire
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("invalid source-reset JSON: %w", err)
	}
	if raw.SchemaVersion != ObservationSchemaVersion {
		return nil, fmt.Errorf("unsupported source-reset schema version")
	}
	if _, err := uuid.Parse(raw.EventID); err != nil {
		return nil, fmt.Errorf("eventId must be a UUID")
	}
	if _, err := uuid.Parse(raw.TargetID); err != nil {
		return nil, fmt.Errorf("targetId must be a UUID")
	}
	if _, err := uuid.Parse(raw.ObserverLeaseID); err != nil {
		return nil, fmt.Errorf("observerLeaseId must be a UUID")
	}
	if raw.PreviousObserverGeneration < 1 || raw.NewObserverGeneration <= raw.PreviousObserverGeneration {
		return nil, fmt.Errorf("newObserverGeneration must be greater than previousObserverGeneration")
	}
	if err := validateKindSource(raw.TargetKind, raw.ObserverKind); err != nil {
		return nil, err
	}
	if raw.TenantID != id.TenantID || raw.TargetID != id.TargetID || raw.TargetKind != id.TargetKind ||
		raw.ObserverID != id.ObserverID || raw.ObserverKind != id.ObserverKind {
		return nil, fmt.Errorf("source-reset identity mismatch")
	}
	return &SourceReset{
		SchemaVersion: raw.SchemaVersion, EventID: raw.EventID, TenantID: raw.TenantID,
		TargetID: raw.TargetID, TargetKind: raw.TargetKind, ObserverID: raw.ObserverID,
		ObserverKind: raw.ObserverKind, PreviousObserverGeneration: raw.PreviousObserverGeneration,
		NewObserverGeneration: raw.NewObserverGeneration, ObservedAt: raw.ObservedAt,
		ObserverLeaseID: raw.ObserverLeaseID, Reason: raw.Reason,
	}, nil
}

func validateKindSource(targetKind, observerKind string) error {
	if targetKind == "KubernetesTarget" && observerKind != "Agent" {
		return fmt.Errorf("KubernetesTarget requires observerKind Agent")
	}
	if targetKind == "EdgeRuntimeTarget" && observerKind != "CloudCore" {
		return fmt.Errorf("EdgeRuntimeTarget requires observerKind CloudCore")
	}
	if targetKind != "KubernetesTarget" && targetKind != "EdgeRuntimeTarget" {
		return fmt.Errorf("unsupported targetKind %q", targetKind)
	}
	return nil
}

func validateIdentityBinding(id Identity, raw observationWire) error {
	if raw.TenantID != id.TenantID {
		return fmt.Errorf("observation tenant %q does not match observer tenant %q", raw.TenantID, id.TenantID)
	}
	if raw.TargetID != id.TargetID {
		return fmt.Errorf("observation target %q does not match observer target %q", raw.TargetID, id.TargetID)
	}
	if raw.TargetKind != id.TargetKind {
		return fmt.Errorf("observation targetKind %q does not match observer targetKind %q", raw.TargetKind, id.TargetKind)
	}
	if raw.ObserverID != id.ObserverID {
		return fmt.Errorf("observation observerId %q does not match identity observerId %q", raw.ObserverID, id.ObserverID)
	}
	if raw.ObserverKind != id.ObserverKind {
		return fmt.Errorf("observation observerKind %q does not match identity observerKind %q", raw.ObserverKind, id.ObserverKind)
	}
	return nil
}

func capabilityContent(raw *capabilityWire) (map[string]any, error) {
	content := map[string]any{
		"snapshotId": raw.SnapshotID, "digest": raw.Digest,
		"runtimeVersion": raw.RuntimeVersion, "architectures": raw.Architectures,
		"resources": map[string]any{
			"cpuMillis": raw.Resources.CpuMillis, "memoryBytes": raw.Resources.MemoryBytes,
			"gpuCount": raw.Resources.GpuCount, "npuCount": raw.Resources.NpuCount,
		},
	}
	if raw.KubernetesVersion != "" {
		content["kubernetesVersion"] = raw.KubernetesVersion
	}
	if raw.KubeEdgeVersion != "" {
		content["kubeEdgeVersion"] = raw.KubeEdgeVersion
	}
	if len(raw.CniPlugins) > 0 {
		content["cniPlugins"] = raw.CniPlugins
	}
	if len(raw.CsiDrivers) > 0 {
		content["csiDrivers"] = raw.CsiDrivers
	}
	if len(raw.GatewayApiVersions) > 0 {
		content["gatewayApiVersions"] = raw.GatewayApiVersions
	}
	if len(raw.SecurityFeatures) > 0 {
		content["securityFeatures"] = raw.SecurityFeatures
	}
	if len(raw.StorageClasses) > 0 {
		content["storageClasses"] = raw.StorageClasses
	}
	return content, nil
}

func rejectUnknownFields(payload []byte, allowed map[string]bool) error {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

var observationAllowedFields = map[string]bool{
	"schemaVersion": true, "eventId": true, "tenantId": true, "targetId": true,
	"targetKind": true, "observerId": true, "observerKind": true,
	"observerGeneration": true, "sequence": true, "observedAt": true,
	"inventoryMode": true, "target": true, "capability": true, "nodes": true,
	"storageInventory": true,
}

var sourceResetAllowedFields = map[string]bool{
	"schemaVersion": true, "eventId": true, "tenantId": true, "targetId": true,
	"targetKind": true, "observerId": true, "observerKind": true,
	"previousObserverGeneration": true, "newObserverGeneration": true,
	"observedAt": true, "observerLeaseId": true, "reason": true,
}

var _ = url.Parse

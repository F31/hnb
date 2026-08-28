package observer

import "time"

// StorageInventory is normalized during ingestion but is intentionally not
// persisted until the storage projection introduced by task 2.4.
type StorageInventory struct {
	StorageClasses         []StorageClassFact          `json:"storageClasses,omitempty"`
	CSIDrivers             []CSIDriverFact             `json:"csiDrivers,omitempty"`
	CSINodes               []CSINodeFact               `json:"csiNodes,omitempty"`
	CSIStorageCapacities   []CSIStorageCapacityFact    `json:"csiStorageCapacities,omitempty"`
	VolumeAttachments      []VolumeAttachmentFact      `json:"volumeAttachments,omitempty"`
	VolumeSnapshotClasses  []VolumeSnapshotClassFact   `json:"volumeSnapshotClasses,omitempty"`
	VolumeSnapshots        []VolumeSnapshotFact        `json:"volumeSnapshots,omitempty"`
	VolumeSnapshotContents []VolumeSnapshotContentFact `json:"volumeSnapshotContents,omitempty"`
	SnapshotAPI            *SnapshotAPI                `json:"snapshotApi,omitempty"`
}

type SnapshotAPI struct {
	Status     string    `json:"status"`
	APIVersion string    `json:"apiVersion,omitempty"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observedAt"`
}

type KubernetesResourceIdentity struct {
	UID             string    `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	Name            string    `json:"name"`
	Source          string    `json:"source"`
	ObservedAt      time.Time `json:"observedAt"`
	Deleted         bool      `json:"deleted,omitempty"`
}

type StorageClassFact struct {
	KubernetesResourceIdentity
	Provisioner          string              `json:"provisioner,omitempty"`
	Parameters           map[string]string   `json:"parameters,omitempty"`
	ReclaimPolicy        string              `json:"reclaimPolicy,omitempty"`
	VolumeBindingMode    string              `json:"volumeBindingMode,omitempty"`
	AllowVolumeExpansion *bool               `json:"allowVolumeExpansion,omitempty"`
	IsDefault            *bool               `json:"isDefault,omitempty"`
	MountOptions         []string            `json:"mountOptions,omitempty"`
	AllowedTopologies    []map[string]string `json:"allowedTopologies,omitempty"`
}

type CSIDriverFact struct {
	KubernetesResourceIdentity
	AttachRequired       *bool    `json:"attachRequired,omitempty"`
	PodInfoOnMount       *bool    `json:"podInfoOnMount,omitempty"`
	StorageCapacity      *bool    `json:"storageCapacity,omitempty"`
	FSGroupPolicy        string   `json:"fsGroupPolicy,omitempty"`
	RequiresRepublish    *bool    `json:"requiresRepublish,omitempty"`
	SELinuxMount         *bool    `json:"seLinuxMount,omitempty"`
	VolumeLifecycleModes []string `json:"volumeLifecycleModes,omitempty"`
}

type CSINodeDriverFact struct {
	Name             string   `json:"name"`
	NodeID           string   `json:"nodeId"`
	AllocatableCount *int64   `json:"allocatableCount,omitempty"`
	TopologyKeys     []string `json:"topologyKeys"`
}

type CSINodeFact struct {
	KubernetesResourceIdentity
	Drivers []CSINodeDriverFact `json:"drivers,omitempty"`
}

type CSIStorageCapacityFact struct {
	KubernetesResourceIdentity
	Namespace              string            `json:"namespace,omitempty"`
	StorageClassName       string            `json:"storageClassName,omitempty"`
	CapacityBytes          *int64            `json:"capacityBytes,omitempty"`
	MaximumVolumeSizeBytes *int64            `json:"maximumVolumeSizeBytes,omitempty"`
	NodeTopology           map[string]string `json:"nodeTopology,omitempty"`
}

type VolumeAttachmentFact struct {
	KubernetesResourceIdentity
	Attacher             string `json:"attacher,omitempty"`
	NodeName             string `json:"nodeName,omitempty"`
	PersistentVolumeName string `json:"persistentVolumeName,omitempty"`
	Attached             *bool  `json:"attached,omitempty"`
	AttachError          string `json:"attachError,omitempty"`
	DetachError          string `json:"detachError,omitempty"`
}

type VolumeSnapshotClassFact struct {
	KubernetesResourceIdentity
	Driver         string            `json:"driver,omitempty"`
	DeletionPolicy string            `json:"deletionPolicy,omitempty"`
	Parameters     map[string]string `json:"parameters,omitempty"`
}

type VolumeSnapshotFact struct {
	KubernetesResourceIdentity
	Namespace                      string `json:"namespace,omitempty"`
	VolumeSnapshotClassName        string `json:"volumeSnapshotClassName,omitempty"`
	SourceKind                     string `json:"sourceKind,omitempty"`
	SourceName                     string `json:"sourceName,omitempty"`
	BoundVolumeSnapshotContentName string `json:"boundVolumeSnapshotContentName,omitempty"`
	ReadyToUse                     *bool  `json:"readyToUse,omitempty"`
	RestoreSizeBytes               *int64 `json:"restoreSizeBytes,omitempty"`
	Error                          string `json:"error,omitempty"`
}

type VolumeSnapshotContentFact struct {
	KubernetesResourceIdentity
	Driver                  string `json:"driver,omitempty"`
	DeletionPolicy          string `json:"deletionPolicy,omitempty"`
	SnapshotHandle          string `json:"snapshotHandle,omitempty"`
	VolumeSnapshotNamespace string `json:"volumeSnapshotNamespace,omitempty"`
	VolumeSnapshotName      string `json:"volumeSnapshotName,omitempty"`
	ReadyToUse              *bool  `json:"readyToUse,omitempty"`
	RestoreSizeBytes        *int64 `json:"restoreSizeBytes,omitempty"`
	Error                   string `json:"error,omitempty"`
}

package observer

import "time"

func diffStorageInventory(previous, current *StorageInventory, observedAt time.Time) *StorageInventory {
	if current == nil {
		return nil
	}
	if previous == nil {
		return cloneStorageInventory(current)
	}
	delta := &StorageInventory{
		StorageClasses: diffFacts(previous.StorageClasses, current.StorageClasses, observedAt,
			func(v StorageClassFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *StorageClassFact, identity KubernetesResourceIdentity) {
				v.KubernetesResourceIdentity = identity
			}),
		CSIDrivers: diffFacts(previous.CSIDrivers, current.CSIDrivers, observedAt,
			func(v CSIDriverFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *CSIDriverFact, identity KubernetesResourceIdentity) { v.KubernetesResourceIdentity = identity }),
		CSINodes: diffFacts(previous.CSINodes, current.CSINodes, observedAt,
			func(v CSINodeFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *CSINodeFact, identity KubernetesResourceIdentity) { v.KubernetesResourceIdentity = identity }),
		CSIStorageCapacities: diffFacts(previous.CSIStorageCapacities, current.CSIStorageCapacities, observedAt,
			func(v CSIStorageCapacityFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *CSIStorageCapacityFact, identity KubernetesResourceIdentity) {
				v.KubernetesResourceIdentity = identity
			}),
		VolumeAttachments: diffFacts(previous.VolumeAttachments, current.VolumeAttachments, observedAt,
			func(v VolumeAttachmentFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *VolumeAttachmentFact, identity KubernetesResourceIdentity) {
				v.KubernetesResourceIdentity = identity
			}),
		VolumeSnapshotClasses: diffFacts(previous.VolumeSnapshotClasses, current.VolumeSnapshotClasses, observedAt,
			func(v VolumeSnapshotClassFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *VolumeSnapshotClassFact, identity KubernetesResourceIdentity) {
				v.KubernetesResourceIdentity = identity
			}),
		VolumeSnapshots: diffFacts(previous.VolumeSnapshots, current.VolumeSnapshots, observedAt,
			func(v VolumeSnapshotFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *VolumeSnapshotFact, identity KubernetesResourceIdentity) {
				v.KubernetesResourceIdentity = identity
			}),
		VolumeSnapshotContents: diffFacts(previous.VolumeSnapshotContents, current.VolumeSnapshotContents, observedAt,
			func(v VolumeSnapshotContentFact) KubernetesResourceIdentity { return v.KubernetesResourceIdentity },
			func(v *VolumeSnapshotContentFact, identity KubernetesResourceIdentity) {
				v.KubernetesResourceIdentity = identity
			}),
	}
	if !snapshotAPIEqual(previous.SnapshotAPI, current.SnapshotAPI) {
		delta.SnapshotAPI = cloneSnapshotAPI(current.SnapshotAPI)
	}
	if storageInventoryEmpty(delta) {
		return nil
	}
	normalizeCoreStorageCollections(delta)
	return delta
}

func diffFacts[T any](previous, current []T, observedAt time.Time, identity func(T) KubernetesResourceIdentity, setIdentity func(*T, KubernetesResourceIdentity)) []T {
	old := make(map[string]T, len(previous))
	for _, fact := range previous {
		old[identity(fact).UID] = fact
	}
	changed := make([]T, 0)
	for _, fact := range current {
		id := identity(fact)
		if prior, ok := old[id.UID]; !ok || identity(prior).ResourceVersion != id.ResourceVersion {
			changed = append(changed, fact)
		}
		delete(old, id.UID)
	}
	for _, fact := range old {
		id := identity(fact)
		id.Deleted = true
		id.ObservedAt = observedAt
		setIdentity(&fact, id)
		changed = append(changed, fact)
	}
	if len(changed) == 0 {
		return nil
	}
	return changed
}

func cloneStorageInventory(inventory *StorageInventory) *StorageInventory {
	if inventory == nil {
		return nil
	}
	clone := *inventory
	clone.StorageClasses = append([]StorageClassFact(nil), inventory.StorageClasses...)
	clone.CSIDrivers = append([]CSIDriverFact(nil), inventory.CSIDrivers...)
	clone.CSINodes = append([]CSINodeFact(nil), inventory.CSINodes...)
	clone.CSIStorageCapacities = append([]CSIStorageCapacityFact(nil), inventory.CSIStorageCapacities...)
	clone.VolumeAttachments = append([]VolumeAttachmentFact(nil), inventory.VolumeAttachments...)
	clone.VolumeSnapshotClasses = append([]VolumeSnapshotClassFact(nil), inventory.VolumeSnapshotClasses...)
	clone.VolumeSnapshots = append([]VolumeSnapshotFact(nil), inventory.VolumeSnapshots...)
	clone.VolumeSnapshotContents = append([]VolumeSnapshotContentFact(nil), inventory.VolumeSnapshotContents...)
	clone.SnapshotAPI = cloneSnapshotAPI(inventory.SnapshotAPI)
	return &clone
}

func storageInventoryEmpty(inventory *StorageInventory) bool {
	return len(inventory.StorageClasses) == 0 && len(inventory.CSIDrivers) == 0 && len(inventory.CSINodes) == 0 &&
		len(inventory.CSIStorageCapacities) == 0 && len(inventory.VolumeAttachments) == 0 &&
		len(inventory.VolumeSnapshotClasses) == 0 && len(inventory.VolumeSnapshots) == 0 && len(inventory.VolumeSnapshotContents) == 0 && inventory.SnapshotAPI == nil
}

func cloneSnapshotAPI(snapshotAPI *SnapshotAPI) *SnapshotAPI {
	if snapshotAPI == nil {
		return nil
	}
	clone := *snapshotAPI
	return &clone
}

func snapshotAPIEqual(a, b *SnapshotAPI) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func normalizeCoreStorageCollections(inventory *StorageInventory) {
	if inventory == nil {
		return
	}
	if inventory.StorageClasses == nil {
		inventory.StorageClasses = []StorageClassFact{}
	}
	if inventory.CSIDrivers == nil {
		inventory.CSIDrivers = []CSIDriverFact{}
	}
	if inventory.CSINodes == nil {
		inventory.CSINodes = []CSINodeFact{}
	}
	if inventory.CSIStorageCapacities == nil {
		inventory.CSIStorageCapacities = []CSIStorageCapacityFact{}
	}
	if inventory.VolumeAttachments == nil {
		inventory.VolumeAttachments = []VolumeAttachmentFact{}
	}
}

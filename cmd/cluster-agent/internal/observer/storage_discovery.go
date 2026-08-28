package observer

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const storageAPISource = "kubernetes.storage.k8s.io/v1"

type objectMetadata struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	UID             string            `json:"uid"`
	ResourceVersion string            `json:"resourceVersion"`
	Annotations     map[string]string `json:"annotations"`
}

type kubernetesList[T any] struct {
	Metadata struct {
		Continue string `json:"continue"`
	} `json:"metadata"`
	Items []T `json:"items"`
}

type storageClassAPI struct {
	Metadata             objectMetadata    `json:"metadata"`
	Provisioner          string            `json:"provisioner"`
	Parameters           map[string]string `json:"parameters"`
	ReclaimPolicy        string            `json:"reclaimPolicy"`
	VolumeBindingMode    string            `json:"volumeBindingMode"`
	AllowVolumeExpansion *bool             `json:"allowVolumeExpansion"`
	MountOptions         []string          `json:"mountOptions"`
	AllowedTopologies    []struct {
		MatchLabelExpressions []struct {
			Key    string   `json:"key"`
			Values []string `json:"values"`
		} `json:"matchLabelExpressions"`
	} `json:"allowedTopologies"`
}

type csiDriverAPI struct {
	Metadata objectMetadata `json:"metadata"`
	Spec     struct {
		AttachRequired       *bool    `json:"attachRequired"`
		PodInfoOnMount       *bool    `json:"podInfoOnMount"`
		StorageCapacity      *bool    `json:"storageCapacity"`
		FSGroupPolicy        string   `json:"fsGroupPolicy"`
		RequiresRepublish    *bool    `json:"requiresRepublish"`
		SELinuxMount         *bool    `json:"seLinuxMount"`
		VolumeLifecycleModes []string `json:"volumeLifecycleModes"`
	} `json:"spec"`
}

type csiNodeAPI struct {
	Metadata objectMetadata `json:"metadata"`
	Spec     struct {
		Drivers []struct {
			Name        string `json:"name"`
			NodeID      string `json:"nodeID"`
			Allocatable *struct {
				Count *int64 `json:"count"`
			} `json:"allocatable"`
			TopologyKeys []string `json:"topologyKeys"`
		} `json:"drivers"`
	} `json:"spec"`
}

type csiStorageCapacityAPI struct {
	Metadata          objectMetadata `json:"metadata"`
	StorageClassName  string         `json:"storageClassName"`
	Capacity          string         `json:"capacity"`
	MaximumVolumeSize string         `json:"maximumVolumeSize"`
	NodeTopology      *struct {
		MatchLabels map[string]string `json:"matchLabels"`
	} `json:"nodeTopology"`
}

type volumeAttachmentAPI struct {
	Metadata objectMetadata `json:"metadata"`
	Spec     struct {
		Attacher string `json:"attacher"`
		NodeName string `json:"nodeName"`
		Source   struct {
			PersistentVolumeName *string `json:"persistentVolumeName"`
		} `json:"source"`
	} `json:"spec"`
	Status struct {
		Attached    *bool `json:"attached"`
		AttachError *struct {
			Message string `json:"message"`
		} `json:"attachError"`
		DetachError *struct {
			Message string `json:"message"`
		} `json:"detachError"`
	} `json:"status"`
}

// DiscoverStorageInventory returns a complete bounded core storage inventory.
// Optional snapshot APIs are intentionally left to task 2.3.
func (d *KubeDiscovery) DiscoverStorageInventory(ctx context.Context, observedAt time.Time) (*StorageInventory, error) {
	storageClasses, err := listPaginated[storageClassAPI](ctx, d, "/apis/storage.k8s.io/v1/storageclasses", 512)
	if err != nil {
		return nil, fmt.Errorf("list StorageClass: %w", err)
	}
	csiDrivers, err := listPaginated[csiDriverAPI](ctx, d, "/apis/storage.k8s.io/v1/csidrivers", 128)
	if err != nil {
		return nil, fmt.Errorf("list CSIDriver: %w", err)
	}
	csiNodes, err := listPaginated[csiNodeAPI](ctx, d, "/apis/storage.k8s.io/v1/csinodes", 5000)
	if err != nil {
		return nil, fmt.Errorf("list CSINode: %w", err)
	}
	capacities, err := listPaginated[csiStorageCapacityAPI](ctx, d, "/apis/storage.k8s.io/v1/csistoragecapacities", 4096)
	if err != nil {
		return nil, fmt.Errorf("list CSIStorageCapacity: %w", err)
	}
	attachments, err := listPaginated[volumeAttachmentAPI](ctx, d, "/apis/storage.k8s.io/v1/volumeattachments", 10000)
	if err != nil {
		return nil, fmt.Errorf("list VolumeAttachment: %w", err)
	}

	inventory := &StorageInventory{
		StorageClasses:       make([]StorageClassFact, 0, len(storageClasses)),
		CSIDrivers:           make([]CSIDriverFact, 0, len(csiDrivers)),
		CSINodes:             make([]CSINodeFact, 0, len(csiNodes)),
		CSIStorageCapacities: make([]CSIStorageCapacityFact, 0, len(capacities)),
		VolumeAttachments:    make([]VolumeAttachmentFact, 0, len(attachments)),
	}
	for _, item := range storageClasses {
		identity, err := storageIdentity("StorageClass", item.Metadata, observedAt)
		if err != nil {
			return nil, err
		}
		fact := StorageClassFact{
			KubernetesResourceIdentity: identity,
			Provisioner:                item.Provisioner, Parameters: item.Parameters, ReclaimPolicy: item.ReclaimPolicy,
			VolumeBindingMode: item.VolumeBindingMode, AllowVolumeExpansion: item.AllowVolumeExpansion,
			MountOptions: item.MountOptions,
		}
		isDefault := annotationTrue(item.Metadata.Annotations["storageclass.kubernetes.io/is-default-class"]) ||
			annotationTrue(item.Metadata.Annotations["storageclass.beta.kubernetes.io/is-default-class"])
		fact.IsDefault = &isDefault
		for _, term := range item.AllowedTopologies {
			topology := make(map[string]string, len(term.MatchLabelExpressions))
			for _, expression := range term.MatchLabelExpressions {
				topology[expression.Key] = strings.Join(expression.Values, ",")
			}
			fact.AllowedTopologies = append(fact.AllowedTopologies, topology)
		}
		inventory.StorageClasses = append(inventory.StorageClasses, fact)
	}
	for _, item := range csiDrivers {
		identity, err := storageIdentity("CSIDriver", item.Metadata, observedAt)
		if err != nil {
			return nil, err
		}
		inventory.CSIDrivers = append(inventory.CSIDrivers, CSIDriverFact{
			KubernetesResourceIdentity: identity,
			AttachRequired:             item.Spec.AttachRequired, PodInfoOnMount: item.Spec.PodInfoOnMount,
			StorageCapacity: item.Spec.StorageCapacity, FSGroupPolicy: item.Spec.FSGroupPolicy,
			RequiresRepublish: item.Spec.RequiresRepublish, SELinuxMount: item.Spec.SELinuxMount,
			VolumeLifecycleModes: item.Spec.VolumeLifecycleModes,
		})
	}
	for _, item := range csiNodes {
		identity, err := storageIdentity("CSINode", item.Metadata, observedAt)
		if err != nil {
			return nil, err
		}
		fact := CSINodeFact{KubernetesResourceIdentity: identity}
		for _, driver := range item.Spec.Drivers {
			mapped := CSINodeDriverFact{Name: driver.Name, NodeID: driver.NodeID, TopologyKeys: driver.TopologyKeys}
			if driver.Allocatable != nil {
				mapped.AllocatableCount = driver.Allocatable.Count
			}
			fact.Drivers = append(fact.Drivers, mapped)
		}
		inventory.CSINodes = append(inventory.CSINodes, fact)
	}
	for _, item := range capacities {
		identity, err := storageIdentity("CSIStorageCapacity", item.Metadata, observedAt)
		if err != nil {
			return nil, err
		}
		capacityBytes, err := quantityBytes(item.Capacity)
		if err != nil {
			return nil, fmt.Errorf("CSIStorageCapacity %q capacity: %w", item.Metadata.Name, err)
		}
		maximumVolumeSizeBytes, err := quantityBytes(item.MaximumVolumeSize)
		if err != nil {
			return nil, fmt.Errorf("CSIStorageCapacity %q maximumVolumeSize: %w", item.Metadata.Name, err)
		}
		fact := CSIStorageCapacityFact{
			KubernetesResourceIdentity: identity,
			Namespace:                  item.Metadata.Namespace, StorageClassName: item.StorageClassName,
			CapacityBytes: capacityBytes, MaximumVolumeSizeBytes: maximumVolumeSizeBytes,
		}
		if item.NodeTopology != nil {
			fact.NodeTopology = item.NodeTopology.MatchLabels
		}
		inventory.CSIStorageCapacities = append(inventory.CSIStorageCapacities, fact)
	}
	for _, item := range attachments {
		identity, err := storageIdentity("VolumeAttachment", item.Metadata, observedAt)
		if err != nil {
			return nil, err
		}
		fact := VolumeAttachmentFact{
			KubernetesResourceIdentity: identity,
			Attacher:                   item.Spec.Attacher, NodeName: item.Spec.NodeName, Attached: item.Status.Attached,
		}
		if item.Spec.Source.PersistentVolumeName != nil {
			fact.PersistentVolumeName = *item.Spec.Source.PersistentVolumeName
		}
		if item.Status.AttachError != nil {
			fact.AttachError = item.Status.AttachError.Message
		}
		if item.Status.DetachError != nil {
			fact.DetachError = item.Status.DetachError.Message
		}
		inventory.VolumeAttachments = append(inventory.VolumeAttachments, fact)
	}
	snapshots, err := d.discoverSnapshotInventory(ctx, observedAt)
	if err != nil {
		return nil, err
	}
	inventory.SnapshotAPI = snapshots.API
	inventory.VolumeSnapshotClasses = snapshots.Classes
	inventory.VolumeSnapshots = snapshots.Snapshots
	inventory.VolumeSnapshotContents = snapshots.Contents
	return inventory, nil
}

func listPaginated[T any](ctx context.Context, d *KubeDiscovery, path string, maxItems int) ([]T, error) {
	items := make([]T, 0)
	continueToken := ""
	seenTokens := make(map[string]struct{})
	for pageNumber := 1; pageNumber <= d.maxPages; pageNumber++ {
		query := url.Values{}
		limit := d.pageLimit
		if remaining := maxItems - len(items); remaining < limit {
			limit = remaining
		}
		if limit < 1 {
			return nil, fmt.Errorf("item limit %d exceeded", maxItems)
		}
		query.Set("limit", strconv.Itoa(limit))
		query.Set("timeoutSeconds", strconv.Itoa(max(1, int(d.requestTimeout/time.Second))))
		if continueToken != "" {
			query.Set("continue", continueToken)
		}
		pageCtx, cancel := context.WithTimeout(ctx, d.requestTimeout)
		var page kubernetesList[T]
		err := d.getJSON(pageCtx, path+"?"+query.Encode(), &page)
		cancel()
		if err != nil {
			return nil, err
		}
		if len(items)+len(page.Items) > maxItems {
			return nil, fmt.Errorf("item limit %d exceeded", maxItems)
		}
		items = append(items, page.Items...)
		if page.Metadata.Continue == "" {
			return items, nil
		}
		if len(items) == maxItems {
			return nil, fmt.Errorf("item limit %d exceeded", maxItems)
		}
		if _, duplicate := seenTokens[page.Metadata.Continue]; duplicate {
			return nil, fmt.Errorf("repeated continue token")
		}
		seenTokens[page.Metadata.Continue] = struct{}{}
		continueToken = page.Metadata.Continue
	}
	return nil, fmt.Errorf("page limit %d exceeded", d.maxPages)
}

func storageIdentity(kind string, metadata objectMetadata, observedAt time.Time) (KubernetesResourceIdentity, error) {
	return resourceIdentity(kind, metadata, storageAPISource, observedAt)
}

func resourceIdentity(kind string, metadata objectMetadata, source string, observedAt time.Time) (KubernetesResourceIdentity, error) {
	if metadata.UID == "" || metadata.ResourceVersion == "" || metadata.Name == "" {
		return KubernetesResourceIdentity{}, fmt.Errorf("%s %q is missing uid or resourceVersion", kind, metadata.Name)
	}
	return KubernetesResourceIdentity{
		UID: metadata.UID, ResourceVersion: metadata.ResourceVersion, Name: metadata.Name,
		Source: source, ObservedAt: observedAt,
	}, nil
}

func annotationTrue(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func quantityBytes(quantity string) (*int64, error) {
	if quantity == "" {
		return nil, nil
	}
	multipliers := []struct {
		suffix     string
		multiplier float64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50},
		{"k", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12}, {"P", 1e15},
	}
	multiplier := float64(1)
	number := quantity
	for _, candidate := range multipliers {
		if strings.HasSuffix(quantity, candidate.suffix) {
			number = strings.TrimSuffix(quantity, candidate.suffix)
			multiplier = candidate.multiplier
			break
		}
	}
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || value < 0 || value*multiplier > float64(^uint64(0)>>1) {
		return nil, fmt.Errorf("invalid byte quantity %q", quantity)
	}
	bytes := int64(value * multiplier)
	return &bytes, nil
}

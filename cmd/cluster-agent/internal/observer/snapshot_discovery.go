package observer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

const (
	snapshotAPIGroupPath = "/apis/snapshot.storage.k8s.io"
	snapshotAPIVersion   = "snapshot.storage.k8s.io/v1"
	snapshotAPISource    = "kubernetes.apidiscovery.k8s.io/v1"
	snapshotObjectSource = "kubernetes.snapshot.storage.k8s.io/v1"
)

type apiGroup struct {
	Versions []struct {
		GroupVersion string `json:"groupVersion"`
		Version      string `json:"version"`
	} `json:"versions"`
}

type apiResourceList struct {
	GroupVersion string `json:"groupVersion"`
	APIResources []struct {
		Name  string   `json:"name"`
		Verbs []string `json:"verbs"`
	} `json:"resources"`
}

type volumeSnapshotClassAPI struct {
	Metadata       objectMetadata    `json:"metadata"`
	Driver         string            `json:"driver"`
	DeletionPolicy string            `json:"deletionPolicy"`
	Parameters     map[string]string `json:"parameters"`
}

type volumeSnapshotAPI struct {
	Metadata objectMetadata `json:"metadata"`
	Spec     struct {
		VolumeSnapshotClassName *string `json:"volumeSnapshotClassName"`
		Source                  struct {
			PersistentVolumeClaimName *string `json:"persistentVolumeClaimName"`
			VolumeSnapshotContentName *string `json:"volumeSnapshotContentName"`
		} `json:"source"`
	} `json:"spec"`
	Status struct {
		BoundVolumeSnapshotContentName *string `json:"boundVolumeSnapshotContentName"`
		ReadyToUse                     *bool   `json:"readyToUse"`
		RestoreSize                    string  `json:"restoreSize"`
		Error                          *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"status"`
}

type volumeSnapshotContentAPI struct {
	Metadata objectMetadata `json:"metadata"`
	Spec     struct {
		Driver         string `json:"driver"`
		DeletionPolicy string `json:"deletionPolicy"`
		Source         struct {
			SnapshotHandle *string `json:"snapshotHandle"`
		} `json:"source"`
		VolumeSnapshotRef struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"volumeSnapshotRef"`
	} `json:"spec"`
	Status struct {
		SnapshotHandle *string `json:"snapshotHandle"`
		ReadyToUse     *bool   `json:"readyToUse"`
		RestoreSize    string  `json:"restoreSize"`
		Error          *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"status"`
}

type snapshotInventory struct {
	API       *SnapshotAPI
	Classes   []VolumeSnapshotClassFact
	Snapshots []VolumeSnapshotFact
	Contents  []VolumeSnapshotContentFact
}

func (d *KubeDiscovery) discoverSnapshotInventory(ctx context.Context, observedAt time.Time) (*snapshotInventory, error) {
	result := &snapshotInventory{API: &SnapshotAPI{Status: "NotInstalled", Source: snapshotAPISource, ObservedAt: observedAt}}
	var group apiGroup
	if err := d.getJSONWithTimeout(ctx, snapshotAPIGroupPath, &group); err != nil {
		if isKubernetesStatus(err, http.StatusNotFound) {
			return result, nil
		}
		return nil, fmt.Errorf("discover snapshot API group: %w", err)
	}
	hasV1 := false
	for _, version := range group.Versions {
		if version.GroupVersion == snapshotAPIVersion || version.Version == "v1" {
			hasV1 = true
			break
		}
	}
	if !hasV1 {
		result.API.Status = "Unsupported"
		return result, nil
	}

	var resources apiResourceList
	if err := d.getJSONWithTimeout(ctx, snapshotAPIGroupPath+"/v1", &resources); err != nil {
		if isKubernetesStatus(err, http.StatusNotFound) {
			return result, nil
		}
		return nil, fmt.Errorf("discover snapshot v1 resources: %w", err)
	}
	if resources.GroupVersion != snapshotAPIVersion {
		result.API.Status = "Unsupported"
		return result, nil
	}
	required := map[string]bool{
		"volumesnapshotclasses":  false,
		"volumesnapshots":        false,
		"volumesnapshotcontents": false,
	}
	for _, resource := range resources.APIResources {
		if _, expected := required[resource.Name]; expected && slices.Contains(resource.Verbs, "list") {
			required[resource.Name] = true
		}
	}
	for _, supported := range required {
		if !supported {
			result.API.Status = "Unsupported"
			return result, nil
		}
	}

	classes, err := listPaginated[volumeSnapshotClassAPI](ctx, d, snapshotAPIGroupPath+"/v1/volumesnapshotclasses", 512)
	if err != nil {
		if isKubernetesStatus(err, http.StatusNotFound) {
			return result, nil
		}
		return nil, fmt.Errorf("list VolumeSnapshotClass: %w", err)
	}
	snapshots, err := listPaginated[volumeSnapshotAPI](ctx, d, snapshotAPIGroupPath+"/v1/volumesnapshots", 10000)
	if err != nil {
		if isKubernetesStatus(err, http.StatusNotFound) {
			return result, nil
		}
		return nil, fmt.Errorf("list VolumeSnapshot: %w", err)
	}
	contents, err := listPaginated[volumeSnapshotContentAPI](ctx, d, snapshotAPIGroupPath+"/v1/volumesnapshotcontents", 10000)
	if err != nil {
		if isKubernetesStatus(err, http.StatusNotFound) {
			return result, nil
		}
		return nil, fmt.Errorf("list VolumeSnapshotContent: %w", err)
	}

	result.API.Status = "Installed"
	result.API.APIVersion = snapshotAPIVersion
	result.Classes = make([]VolumeSnapshotClassFact, 0, len(classes))
	result.Snapshots = make([]VolumeSnapshotFact, 0, len(snapshots))
	result.Contents = make([]VolumeSnapshotContentFact, 0, len(contents))
	for _, item := range classes {
		identity, err := resourceIdentity("VolumeSnapshotClass", item.Metadata, snapshotObjectSource, observedAt)
		if err != nil {
			return nil, err
		}
		result.Classes = append(result.Classes, VolumeSnapshotClassFact{
			KubernetesResourceIdentity: identity, Driver: item.Driver,
			DeletionPolicy: item.DeletionPolicy, Parameters: item.Parameters,
		})
	}
	for _, item := range snapshots {
		identity, err := resourceIdentity("VolumeSnapshot", item.Metadata, snapshotObjectSource, observedAt)
		if err != nil {
			return nil, err
		}
		restoreSize, err := quantityBytes(item.Status.RestoreSize)
		if err != nil {
			return nil, fmt.Errorf("VolumeSnapshot %q restoreSize: %w", item.Metadata.Name, err)
		}
		fact := VolumeSnapshotFact{
			KubernetesResourceIdentity: identity, Namespace: item.Metadata.Namespace,
			ReadyToUse: item.Status.ReadyToUse, RestoreSizeBytes: restoreSize,
		}
		if item.Spec.VolumeSnapshotClassName != nil {
			fact.VolumeSnapshotClassName = *item.Spec.VolumeSnapshotClassName
		}
		if item.Spec.Source.PersistentVolumeClaimName != nil {
			fact.SourceKind = "PersistentVolumeClaim"
			fact.SourceName = *item.Spec.Source.PersistentVolumeClaimName
		} else if item.Spec.Source.VolumeSnapshotContentName != nil {
			fact.SourceKind = "VolumeSnapshotContent"
			fact.SourceName = *item.Spec.Source.VolumeSnapshotContentName
		}
		if item.Status.BoundVolumeSnapshotContentName != nil {
			fact.BoundVolumeSnapshotContentName = *item.Status.BoundVolumeSnapshotContentName
		}
		if item.Status.Error != nil {
			fact.Error = item.Status.Error.Message
		}
		result.Snapshots = append(result.Snapshots, fact)
	}
	for _, item := range contents {
		identity, err := resourceIdentity("VolumeSnapshotContent", item.Metadata, snapshotObjectSource, observedAt)
		if err != nil {
			return nil, err
		}
		restoreSize, err := quantityBytes(item.Status.RestoreSize)
		if err != nil {
			return nil, fmt.Errorf("VolumeSnapshotContent %q restoreSize: %w", item.Metadata.Name, err)
		}
		fact := VolumeSnapshotContentFact{
			KubernetesResourceIdentity: identity, Driver: item.Spec.Driver, DeletionPolicy: item.Spec.DeletionPolicy,
			VolumeSnapshotNamespace: item.Spec.VolumeSnapshotRef.Namespace, VolumeSnapshotName: item.Spec.VolumeSnapshotRef.Name,
			ReadyToUse: item.Status.ReadyToUse, RestoreSizeBytes: restoreSize,
		}
		if item.Status.SnapshotHandle != nil {
			fact.SnapshotHandle = *item.Status.SnapshotHandle
		} else if item.Spec.Source.SnapshotHandle != nil {
			fact.SnapshotHandle = *item.Spec.Source.SnapshotHandle
		}
		if item.Status.Error != nil {
			fact.Error = item.Status.Error.Message
		}
		result.Contents = append(result.Contents, fact)
	}
	return result, nil
}

func (d *KubeDiscovery) getJSONWithTimeout(ctx context.Context, path string, out any) error {
	requestCtx, cancel := context.WithTimeout(ctx, d.requestTimeout)
	defer cancel()
	return d.getJSON(requestCtx, path, out)
}

func isKubernetesStatus(err error, status int) bool {
	var statusErr *kubernetesHTTPError
	return errors.As(err, &statusErr) && statusErr.StatusCode == status
}

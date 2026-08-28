package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

type IntentSecretReference struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

type IntentSemanticDocument struct {
	APIVersion              string                  `json:"apiVersion"`
	Kind                    string                  `json:"kind"`
	ReleaseID               string                  `json:"releaseId,omitempty"`
	TargetRef               string                  `json:"targetRef,omitempty"`
	ScopeRef                string                  `json:"scopeRef,omitempty"`
	TargetID                string                  `json:"targetId,omitempty"`
	TargetKind              string                  `json:"targetKind,omitempty"`
	ExpectedVersion         int64                   `json:"expectedVersion,omitempty"`
	BindingID               string                  `json:"bindingId,omitempty"`
	BindingVersion          int64                   `json:"bindingVersion,omitempty"`
	OfferingID              string                  `json:"offeringId,omitempty"`
	OfferingVersion         int64                   `json:"offeringVersion,omitempty"`
	StorageClassName        string                  `json:"storageClassName,omitempty"`
	StorageClassUID         string                  `json:"storageClassUid,omitempty"`
	StorageClassVersion     string                  `json:"storageClassResourceVersion,omitempty"`
	InstallationID          string                  `json:"installationId,omitempty"`
	PackageID               string                  `json:"packageId,omitempty"`
	PackageVersion          string                  `json:"packageVersion,omitempty"`
	CurrentVersion          string                  `json:"currentVersion,omitempty"`
	DesiredVersion          string                  `json:"desiredVersion,omitempty"`
	DisplayName             string                  `json:"displayName,omitempty"`
	KubernetesVersion       string                  `json:"kubernetesVersion,omitempty"`
	VolumeID                string                  `json:"volumeId,omitempty"`
	WorkflowProviderRef     string                  `json:"workflowProviderRef,omitempty"`
	PersistentVolume        any                     `json:"persistentVolume,omitempty"`
	PersistentVolumeClaim   any                     `json:"persistentVolumeClaim,omitempty"`
	PodDependencies         any                     `json:"podDependencies,omitempty"`
	StatefulSetDependencies any                     `json:"statefulSetDependencies,omitempty"`
	CloudCoreEndpoint       string                  `json:"cloudCoreEndpoint,omitempty"`
	CredentialSecretRef     *IntentSecretReference  `json:"credentialSecretRef,omitempty"`
	NodeGroupMappings       map[string]string       `json:"nodeGroupMappings,omitempty"`
	Parameters              map[string]any          `json:"parameters,omitempty"`
	SecretReferences        []IntentSecretReference `json:"secretReferences,omitempty"`
}

func IntentSemanticDigest(document IntentSemanticDocument) string {
	sort.Slice(document.SecretReferences, func(i, j int) bool {
		a, b := document.SecretReferences[i], document.SecretReferences[j]
		return a.Provider+"\x00"+a.Scope+"\x00"+a.Name+"\x00"+a.Version < b.Provider+"\x00"+b.Scope+"\x00"+b.Name+"\x00"+b.Version
	})
	data, _ := json.Marshal(document)
	digest := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", digest)
}

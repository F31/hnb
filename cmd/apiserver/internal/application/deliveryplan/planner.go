package deliveryplan

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const CompositionSchema = "application-composition/v1"

type Composition struct {
	SchemaVersion  string      `json:"schemaVersion"`
	Kind           string      `json:"kind"`
	Namespace      string      `json:"namespace"`
	TargetID       string      `json:"targetId"`
	ReleaseID      string      `json:"releaseId"`
	ReleaseVersion string      `json:"releaseVersion"`
	SnapshotDigest string      `json:"snapshotDigest"`
	Resources      Resources   `json:"resources,omitempty"`
	Components     []Component `json:"components"`
}

type Resources struct {
	CPU     int64 `json:"cpu,omitempty"`
	Memory  int64 `json:"memory,omitempty"`
	Storage int64 `json:"storage,omitempty"`
}

type Component struct {
	Name             string      `json:"name"`
	Kind             string      `json:"kind"`
	DependsOn        []string    `json:"dependsOn,omitempty"`
	Image            string      `json:"image,omitempty"`
	Ports            []int       `json:"ports,omitempty"`
	SecretRefs       []SecretRef `json:"secretRefs,omitempty"`
	StorageClassName string      `json:"storageClassName,omitempty"`
	StorageBytes     int64       `json:"storageBytes,omitempty"`
	Host             string      `json:"host,omitempty"`
}

type SecretRef struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

type Quota struct {
	CPU     int64
	Memory  int64
	Storage int64
}

type Snapshot struct {
	StorageClasses    []string
	CSIDrivers        []string
	GatewayAPIVersion []string
}

type Validator interface {
	NamespaceOwned(ctx context.Context, tenantID, namespace string) error
	SecretReferenceOwned(ctx context.Context, tenantID string, ref SecretRef) error
	Quota(ctx context.Context, tenantID, namespace string) (Quota, error)
	Snapshot(ctx context.Context, tenantID, targetID, digest string) (Snapshot, error)
}

type Step struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

type Plan struct {
	SchemaVersion  string    `json:"schemaVersion"`
	Namespace      string    `json:"namespace"`
	TargetID       string    `json:"targetId"`
	ReleaseID      string    `json:"releaseId"`
	ReleaseVersion string    `json:"releaseVersion"`
	SnapshotDigest string    `json:"snapshotDigest"`
	Resources      Resources `json:"resources,omitempty"`
	Steps          []Step    `json:"steps"`
}

type Problem struct {
	Code    string
	Message string
}

func (p *Problem) Error() string { return p.Code + ": " + p.Message }

func reject(code, message string) error { return &Problem{Code: code, Message: message} }

var allowedKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "Service": true,
	"PVC": true, "Ingress": true,
}

// Build validates a composition and returns a deterministic dependency order.
func Build(ctx context.Context, tenantID string, composition Composition, validator Validator) (Plan, error) {
	if composition.SchemaVersion != CompositionSchema || composition.Kind != "ApplicationComposition" {
		return Plan{}, reject("COMPOSITION_SCHEMA_INVALID", "schemaVersion/kind must describe an ApplicationComposition")
	}
	if !bounded(composition.Namespace, 1, 63) || !bounded(composition.ReleaseID, 1, 256) ||
		!bounded(composition.ReleaseVersion, 1, 128) || !bounded(composition.SnapshotDigest, 1, 256) ||
		!bounded(composition.TargetID, 1, 64) || len(composition.Components) == 0 || len(composition.Components) > 64 {
		return Plan{}, reject("COMPOSITION_FIELD_INVALID", "namespace, target, release, snapshot and 1..64 components are required")
	}
	if validator == nil {
		return Plan{}, reject("COMPOSITION_VALIDATOR_MISSING", "validation context is required")
	}
	if err := validator.NamespaceOwned(ctx, tenantID, composition.Namespace); err != nil {
		return Plan{}, reject("NAMESPACE_NOT_OWNED", err.Error())
	}
	quota, err := validator.Quota(ctx, tenantID, composition.Namespace)
	if err != nil {
		return Plan{}, reject("QUOTA_UNAVAILABLE", err.Error())
	}
	if (quota.CPU > 0 && composition.Resources.CPU > quota.CPU) ||
		(quota.Memory > 0 && composition.Resources.Memory > quota.Memory) ||
		(quota.Storage > 0 && composition.Resources.Storage > quota.Storage) {
		return Plan{}, reject("QUOTA_EXCEEDED", "requested resources exceed the namespace quota")
	}

	byName := make(map[string]Component, len(composition.Components))
	index := make(map[string]int, len(composition.Components))
	requiresStorage, requiresGateway := false, false
	for i, component := range composition.Components {
		if !bounded(component.Name, 1, 63) || !allowedKinds[component.Kind] {
			return Plan{}, reject("COMPONENT_KIND_INVALID", fmt.Sprintf("component %d has an unsupported kind", i))
		}
		if _, exists := byName[component.Name]; exists {
			return Plan{}, reject("COMPONENT_DUPLICATE", "component names must be unique")
		}
		if component.Kind == "Deployment" || component.Kind == "StatefulSet" {
			if !strings.Contains(component.Image, "@sha256:") {
				return Plan{}, reject("COMPONENT_IMAGE_UNPINNED", "workload images must be digest-pinned")
			}
		}
		if component.Kind == "PVC" {
			requiresStorage = true
			if !bounded(component.StorageClassName, 1, 253) || component.StorageBytes <= 0 {
				return Plan{}, reject("COMPONENT_PVC_INVALID", "PVC requires a storage class and positive storageBytes")
			}
		}
		if component.Kind == "Ingress" {
			requiresGateway = true
			if !bounded(component.Host, 1, 253) {
				return Plan{}, reject("COMPONENT_INGRESS_INVALID", "Ingress requires a host")
			}
		}
		for _, ref := range component.SecretRefs {
			if !bounded(ref.Provider, 1, 128) || !bounded(ref.Scope, 1, 128) || !bounded(ref.Name, 1, 253) || !bounded(ref.Version, 1, 32) {
				return Plan{}, reject("SECRET_REFERENCE_INVALID", "secret reference fields are required")
			}
			if err := validator.SecretReferenceOwned(ctx, tenantID, ref); err != nil {
				return Plan{}, reject("SECRET_REFERENCE_NOT_OWNED", err.Error())
			}
		}
		byName[component.Name] = component
		index[component.Name] = i
	}
	snapshot, err := validator.Snapshot(ctx, tenantID, composition.TargetID, composition.SnapshotDigest)
	if err != nil {
		return Plan{}, reject("SNAPSHOT_INVALID", err.Error())
	}
	if requiresStorage && (len(snapshot.StorageClasses) == 0 || len(snapshot.CSIDrivers) == 0) {
		return Plan{}, reject("CAPABILITY_MISSING", "target capability snapshot has no storage class/CSI driver")
	}
	if requiresGateway && len(snapshot.GatewayAPIVersion) == 0 {
		return Plan{}, reject("CAPABILITY_MISSING", "target capability snapshot has no Gateway API version")
	}

	order, err := topologicalOrder(composition.Components, index)
	if err != nil {
		return Plan{}, err
	}
	steps := make([]Step, 0, len(order))
	for _, name := range order {
		component := byName[name]
		deps := append([]string(nil), component.DependsOn...)
		steps = append(steps, Step{Name: component.Name, Kind: component.Kind, DependsOn: deps})
	}
	return Plan{
		SchemaVersion: "application-plan/v1", Namespace: composition.Namespace, TargetID: composition.TargetID,
		ReleaseID: composition.ReleaseID, ReleaseVersion: composition.ReleaseVersion,
		SnapshotDigest: composition.SnapshotDigest, Resources: composition.Resources, Steps: steps,
	}, nil
}

func topologicalOrder(components []Component, index map[string]int) ([]string, error) {
	indegree := make(map[string]int, len(components))
	dependents := make(map[string][]string, len(components))
	for _, component := range components {
		indegree[component.Name] = len(component.DependsOn)
		for _, dep := range component.DependsOn {
			if _, ok := index[dep]; !ok {
				return nil, reject("DEPENDENCY_UNKNOWN", "dependency "+dep+" does not exist")
			}
			dependents[dep] = append(dependents[dep], component.Name)
		}
	}
	ready := make([]string, 0, len(components))
	for name, degree := range indegree {
		if degree == 0 {
			ready = append(ready, name)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return index[ready[i]] < index[ready[j]] })
	order := make([]string, 0, len(components))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		order = append(order, name)
		next := append([]string(nil), dependents[name]...)
		sort.Slice(next, func(i, j int) bool { return index[next[i]] < index[next[j]] })
		for _, dependent := range next {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		sort.Slice(ready, func(i, j int) bool { return index[ready[i]] < index[ready[j]] })
	}
	if len(order) != len(components) {
		return nil, reject("DEPENDENCY_CYCLE", "component dependencies contain a cycle")
	}
	return order, nil
}

func bounded(value string, min, max int) bool {
	return len(value) >= min && len(value) <= max && strings.TrimSpace(value) == value
}

var ErrInvalid = errors.New("invalid composition")

package config

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

type LayerName string

const (
	LayerDefault     LayerName = "default"
	LayerTier        LayerName = "tier"
	LayerEnvironment LayerName = "environment"
	LayerTenant      LayerName = "tenant"
	LayerInstance    LayerName = "instance"
)

var layerPriority = map[LayerName]int{
	LayerDefault:     1,
	LayerTier:        2,
	LayerEnvironment: 3,
	LayerTenant:      4,
	LayerInstance:    5,
}

type ConfigEntry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	ValueType string    `json:"value_type"`
	Layer     LayerName `json:"layer"`
	SecretRef string    `json:"secret_ref,omitempty"`
}

type Layer struct {
	Name     LayerName              `json:"name"`
	Priority int                    `json:"priority"`
	Values   map[string]ConfigEntry `json:"values"`
}

func NewLayer(name LayerName) *Layer {
	return &Layer{
		Name:     name,
		Priority: layerPriority[name],
		Values:   make(map[string]ConfigEntry),
	}
}

type ConfigResolver struct {
	layers []*Layer
}

func NewConfigResolver() *ConfigResolver {
	return &ConfigResolver{
		layers: make([]*Layer, 0, 5),
	}
}

func (cr *ConfigResolver) AddLayer(layer *Layer) {
	cr.layers = append(cr.layers, layer)
}

func (cr *ConfigResolver) AddEntry(name LayerName, key, value, valueType, secretRef string) {
	entry := ConfigEntry{
		Key:       key,
		Value:     value,
		ValueType: valueType,
		Layer:     name,
		SecretRef: secretRef,
	}
	for _, l := range cr.layers {
		if l.Name == name {
			l.Values[key] = entry
			return
		}
	}
	layer := NewLayer(name)
	layer.Values[key] = entry
	cr.layers = append(cr.layers, layer)
}

func (cr *ConfigResolver) Resolve() map[string]ConfigEntry {
	sort.Slice(cr.layers, func(i, j int) bool {
		return cr.layers[i].Priority < cr.layers[j].Priority
	})

	result := make(map[string]ConfigEntry)

	for _, layer := range cr.layers {
		for key, entry := range layer.Values {
			result[key] = entry
		}
	}

	return result
}

type ConfigSnapshot struct {
	ID         string                 `json:"id"`
	EntityType string                 `json:"entity_type"`
	EntityID   string                 `json:"entity_id"`
	Config     map[string]ConfigEntry `json:"config"`
	Digest     string                 `json:"digest"`
	LayersUsed []string               `json:"layers_used"`
}

func ComputeSnapshotDigest(config map[string]ConfigEntry) string {
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		v := config[k]
		fmt.Fprintf(h, "%s:%s:%s\n", k, v.Value, v.ValueType)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func BuildSnapshot(entityType, entityID string, config map[string]ConfigEntry) *ConfigSnapshot {
	digest := ComputeSnapshotDigest(config)
	layersUsed := make([]string, 0)
	seen := make(map[LayerName]bool)

	for _, entry := range config {
		if !seen[entry.Layer] {
			seen[entry.Layer] = true
			layersUsed = append(layersUsed, string(entry.Layer))
		}
	}

	return &ConfigSnapshot{
		ID:         fmt.Sprintf("cs-%s", digest[:16]),
		EntityType: entityType,
		EntityID:   entityID,
		Config:     config,
		Digest:     digest,
		LayersUsed: layersUsed,
	}
}

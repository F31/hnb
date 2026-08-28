package handler

import (
	"fmt"
	"sort"
)

const storageBackendComponentType = "resource.storage.BackendConfigurationForm"

type storageProviderField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

type storageProviderSchema struct {
	SchemaVersion         string                 `json:"schemaVersion"`
	ProviderType          string                 `json:"providerType"`
	ProviderSchemaVersion string                 `json:"providerSchemaVersion"`
	ComponentType         string                 `json:"componentType"`
	Fields                []storageProviderField `json:"fields"`
}

var storageProviderSchemas = map[string]storageProviderSchema{
	"generic-csi@1.0.0": {
		SchemaVersion: "1.0.0", ProviderType: "generic-csi", ProviderSchemaVersion: "1.0.0", ComponentType: storageBackendComponentType,
		Fields: []storageProviderField{
			{Name: "provisioner", Label: "CSI provisioner", Type: "text", Required: true},
			{Name: "volumeBindingMode", Label: "Volume binding mode", Type: "select", Required: true, Options: []string{"Immediate", "WaitForFirstConsumer"}},
			{Name: "allowExpansion", Label: "Allow volume expansion", Type: "boolean", Required: false},
		},
	},
	"nfs@1.0.0": {
		SchemaVersion: "1.0.0", ProviderType: "nfs", ProviderSchemaVersion: "1.0.0", ComponentType: storageBackendComponentType,
		Fields: []storageProviderField{
			{Name: "server", Label: "NFS server", Type: "text", Required: true},
			{Name: "exportPath", Label: "Export path", Type: "text", Required: true},
			{Name: "readOnly", Label: "Read only", Type: "boolean", Required: false},
		},
	},
}

func listStorageProviderSchemas() []storageProviderSchema {
	result := make([]storageProviderSchema, 0, len(storageProviderSchemas))
	for _, schema := range storageProviderSchemas {
		result = append(result, schema)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProviderType < result[j].ProviderType })
	return result
}

func validateStorageProviderAttributes(providerType, version string, attributes map[string]any) bool {
	schema, ok := storageProviderSchemas[fmt.Sprintf("%s@%s", providerType, version)]
	if !ok || len(attributes) > len(schema.Fields) {
		return false
	}
	fields := make(map[string]storageProviderField, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.Name] = field
		if field.Required {
			if _, present := attributes[field.Name]; !present {
				return false
			}
		}
	}
	for name, value := range attributes {
		field, ok := fields[name]
		if !ok || !validStorageProviderValue(field, value) {
			return false
		}
	}
	return true
}

func validStorageProviderValue(field storageProviderField, value any) bool {
	switch field.Type {
	case "text":
		text, ok := value.(string)
		return ok && boundedStorage(text, 512, field.Required)
	case "number":
		_, ok := value.(float64)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "select":
		text, ok := value.(string)
		if !ok {
			return false
		}
		for _, option := range field.Options {
			if text == option {
				return true
			}
		}
	}
	return false
}

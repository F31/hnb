package observer

import "time"

// Observation is the RT-008 runtime-target observation envelope produced by
// the agent. Field names match the published runtime-target observation schema.
type Observation struct {
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
	Target             *TargetState      `json:"target,omitempty"`
	Capability         *Capability       `json:"capability,omitempty"`
	Nodes              []Node            `json:"nodes,omitempty"`
	StorageInventory   *StorageInventory `json:"storageInventory,omitempty"`
}

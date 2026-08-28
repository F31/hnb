package observer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ObserverIdentity struct {
	TenantID     string
	TargetID     string
	TargetKind   string
	ObserverID   string
	ObserverKind string
}

type Observation struct {
	SchemaVersion      string       `json:"schemaVersion"`
	EventID            string       `json:"eventId"`
	TenantID           string       `json:"tenantId"`
	TargetID           string       `json:"targetId"`
	TargetKind         string       `json:"targetKind"`
	ObserverID         string       `json:"observerId"`
	ObserverKind       string       `json:"observerKind"`
	ObserverGeneration int64        `json:"observerGeneration"`
	Sequence           int64        `json:"sequence"`
	ObservedAt         time.Time    `json:"observedAt"`
	InventoryMode      string       `json:"inventoryMode"`
	Target             *TargetState `json:"target,omitempty"`
	Capability         *Capability  `json:"capability,omitempty"`
	Nodes              []Node       `json:"nodes,omitempty"`
}

type TargetState struct {
	LifecycleState        string    `json:"lifecycleState"`
	HealthState           string    `json:"healthState"`
	ConnectivityState     string    `json:"connectivityState"`
	LastKnownStateAt      time.Time `json:"lastKnownStateAt"`
	StaleThresholdSeconds int64     `json:"staleThresholdSeconds"`
	RuntimeVersion        string    `json:"runtimeVersion,omitempty"`
}

type Capability struct {
	SnapshotID        string   `json:"snapshotId"`
	Digest            string   `json:"digest"`
	KubernetesVersion string   `json:"kubernetesVersion,omitempty"`
	KubeEdgeVersion   string   `json:"kubeEdgeVersion,omitempty"`
	RuntimeVersion    string   `json:"runtimeVersion"`
	Architectures     []string `json:"architectures"`
	Resources         struct {
		CpuMillis   int64 `json:"cpuMillis"`
		MemoryBytes int64 `json:"memoryBytes"`
		GpuCount    int64 `json:"gpuCount,omitempty"`
		NpuCount    int64 `json:"npuCount,omitempty"`
	} `json:"resources"`
	CniPlugins []string `json:"cniPlugins,omitempty"`
	CsiDrivers []string `json:"csiDrivers,omitempty"`
}

type Node struct {
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
	Resources         map[string]int64  `json:"resources"`
	Labels            map[string]string `json:"labels,omitempty"`
}

type Producer struct {
	identity ObserverIdentity

	mu                sync.Mutex
	generation        int64
	sequence          int64
	lastInventory     map[string]Node
	maxPayloadBytes   int
	persistGeneration func(int64) error
}

func NewProducer(identity ObserverIdentity, generation, sequence int64, persistGeneration func(int64) error) *Producer {
	if generation < 1 {
		generation = 1
	}
	if sequence < 1 {
		sequence = 1
	}
	return &Producer{
		identity:          identity,
		generation:        generation,
		sequence:          sequence,
		lastInventory:     make(map[string]Node),
		maxPayloadBytes:   1 << 20,
		persistGeneration: persistGeneration,
	}
}

func (p *Producer) Generation() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation
}

func (p *Producer) Sequence() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sequence
}

func (p *Producer) SourceReset(newGeneration int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if newGeneration <= p.generation {
		return fmt.Errorf("new observer generation %d must exceed current %d", newGeneration, p.generation)
	}
	if p.persistGeneration != nil {
		if err := p.persistGeneration(newGeneration); err != nil {
			return err
		}
	}
	p.generation = newGeneration
	p.sequence = 1
	p.lastInventory = make(map[string]Node)
	return nil
}

func (p *Producer) Full(observedAt time.Time, target *TargetState, capability *Capability, nodes []Node) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	o := p.newObservation(observedAt, "Full", target, capability, nodes)
	data, err := p.encode(o)
	if err != nil {
		return nil, err
	}
	replacement := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		replacement[node.NodeID] = node
	}
	p.lastInventory = replacement
	return data, nil
}

func (p *Producer) Delta(observedAt time.Time, target *TargetState, capability *Capability, added []Node, removed []string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	nodes := append([]Node(nil), added...)
	for _, id := range removed {
		nodes = append(nodes, Node{NodeID: id, Deleted: true})
	}
	o := p.newObservation(observedAt, "Delta", target, capability, nodes)
	data, err := p.encode(o)
	if err != nil {
		return nil, err
	}
	for _, node := range added {
		p.lastInventory[node.NodeID] = node
	}
	for _, id := range removed {
		delete(p.lastInventory, id)
	}
	return data, nil
}

func (p *Producer) DeltaFromCache(observedAt time.Time, target *TargetState, capability *Capability, current []Node) ([]byte, error) {
	p.mu.Lock()
	var changed []Node
	present := make(map[string]Node, len(current))
	for _, node := range current {
		present[node.NodeID] = node
		if prev, ok := p.lastInventory[node.NodeID]; !ok || !nodesEqual(prev, node) {
			changed = append(changed, node)
		}
	}
	var removed []string
	for id := range p.lastInventory {
		if _, ok := present[id]; !ok {
			removed = append(removed, id)
		}
	}
	p.mu.Unlock()
	return p.Delta(observedAt, target, capability, changed, removed)
}

func nodesEqual(a, b Node) bool {
	if a.NodeID != b.NodeID || a.HealthState != b.HealthState || a.ConnectivityState != b.ConnectivityState ||
		a.LifecycleState != b.LifecycleState || a.Architecture != b.Architecture || a.KubeletVersion != b.KubeletVersion {
		return false
	}
	if len(a.Resources) != len(b.Resources) {
		return false
	}
	for k, av := range a.Resources {
		if bv, ok := b.Resources[k]; !ok || av != bv {
			return false
		}
	}
	return true
}

func (p *Producer) LastInventory() map[string]Node {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]Node, len(p.lastInventory))
	for k, v := range p.lastInventory {
		out[k] = v
	}
	return out
}

func (p *Producer) newObservation(observedAt time.Time, mode string, target *TargetState, capability *Capability, nodes []Node) Observation {
	seq := p.sequence
	p.sequence++
	return Observation{
		SchemaVersion:      "1.0.0",
		EventID:            uuid.NewString(),
		TenantID:           p.identity.TenantID,
		TargetID:           p.identity.TargetID,
		TargetKind:         p.identity.TargetKind,
		ObserverID:         p.identity.ObserverID,
		ObserverKind:       p.identity.ObserverKind,
		ObserverGeneration: p.generation,
		Sequence:           seq,
		ObservedAt:         observedAt.UTC(),
		InventoryMode:      mode,
		Target:             target,
		Capability:         capability,
		Nodes:              nodes,
	}
}

func (p *Producer) encode(o Observation) ([]byte, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return nil, fmt.Errorf("marshal observation: %w", err)
	}
	if len(data) > p.maxPayloadBytes {
		return nil, fmt.Errorf("observation payload %d bytes exceeds limit %d; reduce inventory or use delta", len(data), p.maxPayloadBytes)
	}
	return data, nil
}

func capContentDigest(cap *Capability) string {
	clone := *cap
	clone.SnapshotID = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum)
}

func parseCPUToMillis(value string) int64 {
	if value == "" {
		return 0
	}
	if len(value) > 0 && value[len(value)-1] == 'm' {
		var millis int64
		if _, err := fmt.Sscanf(value[:len(value)-1], "%d", &millis); err == nil {
			return millis
		}
	}
	var cores float64
	if _, err := fmt.Sscanf(value, "%f", &cores); err == nil {
		return int64(cores * 1000)
	}
	return 0
}

func parseBytes(value string) int64 {
	if value == "" {
		return 0
	}
	suffixes := map[string]int64{"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40}
	for suffix, mult := range suffixes {
		if len(value) > len(suffix) && value[len(value)-len(suffix):] == suffix {
			var n float64
			if _, err := fmt.Sscanf(value[:len(value)-len(suffix)], "%f", &n); err == nil {
				return int64(n * float64(mult))
			}
		}
	}
	var n int64
	if _, err := fmt.Sscanf(value, "%d", &n); err == nil {
		return n
	}
	return 0
}

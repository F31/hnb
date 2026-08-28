package observer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ObserverIdentity binds the agent to a single target and tenant. It mirrors
// the identity carried in the signed observer token issued at handshake.
type ObserverIdentity struct {
	TenantID     string
	TargetID     string
	TargetKind   string
	ObserverID   string
	ObserverKind string
}

// Producer maintains the monotonic observer generation/sequence and emits
// Full and Delta observation envelopes that conform to the RT-008
// RuntimeTargetObservation contract.
type Producer struct {
	identity ObserverIdentity

	mu                   sync.Mutex
	generation           int64
	sequence             int64
	lastInventory        map[string]Node
	lastStorageInventory *StorageInventory
	maxPayloadBytes      int
	clockSkewAllowance   time.Duration
	persistGeneration    func(int64) error
}

// NewProducer returns a producer starting at the given observer generation and
// sequence. persistGeneration is invoked when the generation is advanced by a
// source reset; it may be nil.
func NewProducer(identity ObserverIdentity, generation, sequence int64, persistGeneration func(int64) error) *Producer {
	if generation < 1 {
		generation = 1
	}
	if sequence < 1 {
		sequence = 1
	}
	return &Producer{
		identity:           identity,
		generation:         generation,
		sequence:           sequence,
		lastInventory:      make(map[string]Node),
		maxPayloadBytes:    1 << 20,
		clockSkewAllowance: 5 * time.Minute,
		persistGeneration:  persistGeneration,
	}
}

// Generation returns the current observer generation.
func (p *Producer) Generation() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation
}

// Sequence returns the next sequence to be emitted.
func (p *Producer) Sequence() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sequence
}

// SourceReset advances to a new observer generation and resets the sequence to
// 1, mirroring the server-side source-reset contract. The previous generation
// is fenced.
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
	p.lastStorageInventory = nil
	return nil
}

// Full emits a complete inventory observation and replaces the local inventory
// cache. nodes must be the complete set of nodes currently observed.
func (p *Producer) Full(observedAt time.Time, target *TargetState, capability *Capability, nodes []Node) ([]byte, error) {
	return p.FullWithStorage(observedAt, target, capability, nodes, nil)
}

// FullWithStorage emits complete node and core Kubernetes storage inventories
// on the same RT-008 generation and sequence cursor.
func (p *Producer) FullWithStorage(observedAt time.Time, target *TargetState, capability *Capability, nodes []Node, storage *StorageInventory) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	storage = cloneStorageInventory(storage)
	normalizeCoreStorageCollections(storage)
	o := p.newObservation(observedAt, "Full", target, capability, nodes, storage)
	oBytes, err := p.encode(o)
	if err != nil {
		return nil, err
	}
	replacement := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		replacement[node.NodeID] = node
	}
	p.lastInventory = replacement
	p.lastStorageInventory = storage
	return oBytes, nil
}

// Delta emits a delta observation containing only changed nodes plus explicit
// tombstones for removed nodes.
func (p *Producer) Delta(observedAt time.Time, target *TargetState, capability *Capability, added []Node, removed []string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	nodes := append([]Node(nil), added...)
	for _, id := range removed {
		nodes = append(nodes, Node{NodeID: id, Deleted: true})
	}
	o := p.newObservation(observedAt, "Delta", target, capability, nodes, nil)
	oBytes, err := p.encode(o)
	if err != nil {
		return nil, err
	}
	for _, node := range added {
		p.lastInventory[node.NodeID] = node
	}
	for _, id := range removed {
		delete(p.lastInventory, id)
	}
	return oBytes, nil
}

// DeltaFromCache compares the given node set to the last full inventory and
// emits a delta containing added, changed, and removed nodes.
func (p *Producer) DeltaFromCache(observedAt time.Time, target *TargetState, capability *Capability, current []Node) ([]byte, error) {
	return p.DeltaFromCacheWithStorage(observedAt, target, capability, current, nil)
}

// DeltaFromCacheWithStorage emits node and storage facts whose Kubernetes
// resourceVersion changed, plus stable-identity tombstones for removals.
func (p *Producer) DeltaFromCacheWithStorage(observedAt time.Time, target *TargetState, capability *Capability, current []Node, storage *StorageInventory) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	storageDelta := diffStorageInventory(p.lastStorageInventory, storage, observedAt)
	nodes := append([]Node(nil), changed...)
	for _, id := range removed {
		nodes = append(nodes, Node{NodeID: id, Deleted: true})
	}
	o := p.newObservation(observedAt, "Delta", target, capability, nodes, storageDelta)
	oBytes, err := p.encode(o)
	if err != nil {
		return nil, err
	}
	p.lastInventory = make(map[string]Node, len(current))
	for _, node := range current {
		p.lastInventory[node.NodeID] = node
	}
	if storage != nil {
		p.lastStorageInventory = cloneStorageInventory(storage)
	}
	return oBytes, nil
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

// LastInventory returns the current full inventory cache.
func (p *Producer) LastInventory() map[string]Node {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]Node, len(p.lastInventory))
	for k, v := range p.lastInventory {
		out[k] = v
	}
	return out
}

func (p *Producer) newObservation(observedAt time.Time, mode string, target *TargetState, capability *Capability, nodes []Node, storage *StorageInventory) Observation {
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
		StorageInventory:   storage,
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

// Digest returns the canonical content digest for an emitted observation.
func (p *Producer) Digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum)
}

package observer

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrGap indicates a sequence gap for the same observer generation: the
// observation cannot be committed until the preceding sequence arrives.
var ErrGap = errors.New("observation sequence gap")

// ErrReplay indicates a duplicate / idempotent observation that was already
// committed; it is a successful no-op and not an error path.
var ErrReplay = errors.New("observation already committed")

// ErrFenced indicates the observation comes from a lower observer generation
// than the one currently established, or fails a source-reset lease check.
var ErrFenced = errors.New("observation fenced by newer observer generation")

// maxPayloadBytes bounds the raw encoded observation body.
const maxPayloadBytes = 1 << 20

// maxClockSkew bounds how far into the future observedAt may be.
const maxClockSkew = 300 * time.Second

const observationSchemaVersion = "1.0.0"

// Identity is the tenant-bound observer identity that authenticates an
// observation. It is always derived from the verified workload identity (mTLS
// or equivalent), never from the payload.
type Identity struct {
	TenantID     string
	TargetID     string
	TargetKind   string
	ObserverID   string
	ObserverKind string
}

// Valid reports whether the identity is fully populated.
func (i Identity) Valid() bool {
	return i.TenantID != "" && i.TargetID != "" && i.TargetKind != "" && i.ObserverID != "" && i.ObserverKind != ""
}

// Observation is the canonical, validated observation accepted by the
// projector. Fields are normalized from the generated runtime-target schema
// types so the projector never re-parses untrusted JSON.
type Observation struct {
	SchemaVersion      string
	EventID            string
	TenantID           string
	TargetID           string
	TargetKind         string
	ObserverID         string
	ObserverKind       string
	ObserverGeneration int64
	Sequence           int64
	ObservedAt         time.Time
	InventoryMode      string
	Target             *TargetState
	Capability         *Capability
	Nodes              []Node
	StorageInventory   *StorageInventory
}

// TargetState is the normalized target partition.
type TargetState struct {
	LifecycleState        string
	HealthState           string
	ConnectivityState     string
	LastKnownStateAt      time.Time
	StaleThresholdSeconds int64
	RuntimeVersion        string
}

// Capability is the normalized capability partition.
type Capability struct {
	SnapshotID string
	Digest     string
	Content    map[string]any
}

// Node is the normalized node partition entry.
type Node struct {
	NodeID            string
	Name              string
	LifecycleState    string
	HealthState       string
	ConnectivityState string
	Freshness         string
	ObservedAt        time.Time
	LastKnownStateAt  time.Time
	Deleted           bool
	RuntimeVersion    string
	KubeletVersion    string
	Architecture      string
	Resources         map[string]any
	Labels            map[string]string
}

// cursor is the (tenant, target, observer) cursor row.
type cursor struct {
	TenantID           string
	TargetID           string
	ObserverID         string
	ObserverKind       string
	ObserverGeneration int64
	Sequence           int64
	PayloadDigest      string
	LastMessageID      string
	ObservedAt         time.Time
}

// SourceReset is the normalized source-reset control message.
type SourceReset struct {
	SchemaVersion              string
	EventID                    string
	TenantID                   string
	TargetID                   string
	TargetKind                 string
	ObserverID                 string
	ObserverKind               string
	PreviousObserverGeneration int64
	NewObserverGeneration      int64
	ObservedAt                 time.Time
	ObserverLeaseID            string
	Reason                     string
}

// Digest computes the canonical sha256 digest of an observation's mutable
// content (all partitions), used for idempotent cursor persistence and
// capability snapshot dedup.
func (o *Observation) Digest() string {
	content := map[string]any{}
	if o.Target != nil {
		content["target"] = o.Target
	}
	if o.Capability != nil {
		content["capability"] = o.Capability.Content
	}
	if len(o.Nodes) > 0 {
		content["nodes"] = o.Nodes
	}
	if o.StorageInventory != nil {
		content["storageInventory"] = o.StorageInventory
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		// Canonical observation content always marshals.
		return ""
	}
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", sum)
}

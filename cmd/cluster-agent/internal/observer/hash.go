package observer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// newSnapshotID returns a fresh snapshot UUID.
func newSnapshotID() string {
	return uuid.NewString()
}

// capContentDigest computes a stable content digest for a capability snapshot
// so the projector can deduplicate identical snapshots. The snapshotId is a
// storage key, not content, and is excluded.
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

var _ = time.Now

package toolkit

import (
	"time"
)

// PhaseName is a stable install phase identifier used by status, resume and
// verification. Managed mode mirrors existing Operation steps instead.
type PhaseName string

const (
	PhaseDraft            PhaseName = "draft"
	PhaseValidating       PhaseName = "validating"
	PhasePlanning         PhaseName = "planning"
	PhasePreflight        PhaseName = "preflight"
	PhasePreparing        PhaseName = "preparing"
	PhaseBackup           PhaseName = "backup"
	PhaseInfraDelta       PhaseName = "infra-delta"
	PhaseNodePrereq       PhaseName = "node-prereq"
	PhaseRuntime          PhaseName = "runtime"
	PhaseInit             PhaseName = "init"
	PhaseJoin             PhaseName = "join"
	PhaseNetwork          PhaseName = "network"
	PhaseClusterVerified  PhaseName = "cluster-verified"
	PhaseIdentity         PhaseName = "identity"
	PhaseInfrastructure   PhaseName = "infrastructure"
	PhaseDatabaseMigrate  PhaseName = "database-migration"
	PhasePlatformRelease  PhaseName = "platform-release"
	PhasePlatformVerified PhaseName = "platform-verified"
	PhaseHandoff          PhaseName = "handoff"
	PhaseSucceeded        PhaseName = "succeeded"
	PhaseBlocked          PhaseName = "blocked"
	PhaseFailed           PhaseName = "failed"
)

// PhaseOutcome records one completed or in-flight phase.
type PhaseOutcome struct {
	Name       PhaseName `yaml:"name" json:"name"`
	Status     string    `yaml:"status" json:"status"` // pending | running | succeeded | blocked | failed
	StartedAt  time.Time `yaml:"startedAt" json:"startedAt,omitempty"`
	FinishedAt time.Time `yaml:"finishedAt" json:"finishedAt,omitempty"`
	ReasonCode string    `yaml:"reasonCode" json:"reasonCode,omitempty"`
}

// BootstrapReceipt is the local, permission-restricted record of a first-time
// HNB management-base install. It is an attachment for later handoff, never a
// replacement for the platform Operation store.
type BootstrapReceipt struct {
	SchemaVersion        string         `yaml:"schemaVersion" json:"schemaVersion"`
	RunID                string         `yaml:"runId" json:"runId"`
	ConfigDigest         string         `yaml:"configDigest" json:"configDigest"`
	LockDigest           string         `yaml:"lockDigest" json:"lockDigest"`
	ManagementClusterUID string         `yaml:"managementClusterUID" json:"managementClusterUID"`
	StartedAt            time.Time      `yaml:"startedAt" json:"startedAt"`
	FinishedAt           time.Time      `yaml:"finishedAt" json:"finishedAt"`
	Phases               []PhaseOutcome `yaml:"phases" json:"phases"`
	Ownership            []string       `yaml:"ownership" json:"ownership"`
	Evidence             []string       `yaml:"evidence" json:"evidence"`
}

// DecodeBootstrapReceipt parses a BootstrapReceipt with strict unknown-field rejection.
func DecodeBootstrapReceipt(data []byte) (*BootstrapReceipt, error) {
	var receipt BootstrapReceipt
	if err := decodeStrict(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

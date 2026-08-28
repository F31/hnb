package store

import "fmt"

// Operation status values. Kept in sync with
// cmd/operation-worker/internal/engine/state.go; platform-api may only
// initiate pending -> pending_approval/queued, approval transitions
// (pending_approval -> queued/cancelled) and cancellation. Execution-state
// transitions belong to operation-worker.
const (
	StatusPending         = "pending"
	StatusPendingApproval = "pending_approval"
	StatusQueued          = "queued"
	StatusQueuedOffline   = "queued_offline"
	StatusInProgress      = "in_progress"
	StatusPaused          = "paused"
	StatusCompensating    = "compensating"
	StatusSucceeded       = "succeeded"
	StatusFailed          = "failed"
	StatusCancelled       = "cancelled"
)

var validOperationTypes = map[string]bool{
	"deploy": true, "upgrade": true, "rollback": true, "scale": true,
	"backup": true, "restore": true, "switchover": true, "delete": true,
	"gc": true, "ota": true, "config_change": true,
	"storage_reclaim": true,
}

func IsValidOperationType(t string) bool { return validOperationTypes[t] }

func IsValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusPendingApproval, StatusQueued, StatusQueuedOffline,
		StatusInProgress, StatusPaused, StatusCompensating,
		StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// highRiskTypes enter pending_approval and require explicit approval before
// being queued (TENANT-003).
var highRiskTypes = map[string]bool{
	"delete":          true,
	"rollback":        true,
	"config_change":   true,
	"storage_reclaim": true,
}

func IsHighRisk(t string) bool { return highRiskTypes[t] }

// InitialStatus decides the state a freshly submitted Operation starts in.
func InitialStatus(operationType string) string {
	if IsHighRisk(operationType) {
		return StatusPendingApproval
	}
	return StatusQueued
}

// validTransitions mirrors engine/state.go. platform-api only ever targets
// StatusQueued (approve) and StatusCancelled (reject/cancel).
var validTransitions = map[string]map[string]bool{
	StatusPending: {
		StatusPendingApproval: true,
		StatusQueued:          true,
		StatusCancelled:       true,
	},
	StatusPendingApproval: {
		StatusQueued:    true,
		StatusCancelled: true,
	},
	StatusQueued: {
		StatusInProgress:    true,
		StatusQueuedOffline: true,
		StatusCancelled:     true,
	},
	StatusQueuedOffline: {
		StatusInProgress: true,
		StatusFailed:     true,
	},
	StatusInProgress: {
		StatusSucceeded:    true,
		StatusFailed:       true,
		StatusPaused:       true,
		StatusCompensating: true,
	},
	StatusPaused: {
		StatusInProgress:   true,
		StatusCompensating: true,
		StatusCancelled:    true,
	},
	StatusCompensating: {
		StatusFailed:    true,
		StatusCancelled: true,
	},
}

func CanTransition(from, to string) bool {
	if from == to {
		return false
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// StepIdempotencyKey derives the per-step idempotency key persisted on
// operation_steps and echoed in step-requested outbox events. The worker
// rejects keys longer than 128 characters, so callers must validate length.
func StepIdempotencyKey(operationKey, planStepID string) string {
	return fmt.Sprintf("%s:%s", operationKey, planStepID)
}

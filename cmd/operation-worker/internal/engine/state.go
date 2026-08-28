package engine

type OperationStatus string

const (
	StatusPending         OperationStatus = "pending"
	StatusPendingApproval OperationStatus = "pending_approval"
	StatusQueued          OperationStatus = "queued"
	StatusQueuedOffline   OperationStatus = "queued_offline"
	StatusInProgress      OperationStatus = "in_progress"
	StatusPaused          OperationStatus = "paused"
	StatusCompensating    OperationStatus = "compensating"
	StatusSucceeded       OperationStatus = "succeeded"
	StatusFailed          OperationStatus = "failed"
	StatusCancelled       OperationStatus = "cancelled"
)

var terminalStates = map[OperationStatus]bool{
	StatusSucceeded: true,
	StatusFailed:    true,
	StatusCancelled: true,
}

func IsTerminal(s OperationStatus) bool {
	return terminalStates[s]
}

var validTransitions = map[OperationStatus]map[OperationStatus]bool{
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

func CanTransition(from, to OperationStatus) bool {
	if from == to {
		return false
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

type StepStatus string

const (
	StepPending     StepStatus = "pending"
	StepRunning     StepStatus = "running"
	StepSucceeded   StepStatus = "succeeded"
	StepFailed      StepStatus = "failed"
	StepSkipped     StepStatus = "skipped"
	StepCompensated StepStatus = "compensated"
)

type OperationType string

const (
	OpDeploy       OperationType = "deploy"
	OpUpgrade      OperationType = "upgrade"
	OpRollback     OperationType = "rollback"
	OpScale        OperationType = "scale"
	OpBackup       OperationType = "backup"
	OpRestore      OperationType = "restore"
	OpSwitchover   OperationType = "switchover"
	OpDelete       OperationType = "delete"
	OpGC           OperationType = "gc"
	OpOTA          OperationType = "ota"
	OpConfigChange OperationType = "config_change"
)

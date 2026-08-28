package engine

type AuditEventType string

const (
	AuditCreated       AuditEventType = "created"
	AuditApproved      AuditEventType = "approved"
	AuditRejected      AuditEventType = "rejected"
	AuditStarted       AuditEventType = "started"
	AuditStepCompleted AuditEventType = "step_completed"
	AuditStepFailed    AuditEventType = "step_failed"
	AuditCompensated   AuditEventType = "compensated"
	AuditPaused        AuditEventType = "paused"
	AuditResumed       AuditEventType = "resumed"
	AuditCancelled     AuditEventType = "cancelled"
	AuditSucceeded     AuditEventType = "succeeded"
	AuditFailed        AuditEventType = "failed"
	AuditStateChanged  AuditEventType = "state_changed"
)

type AuditEntry struct {
	ID            string         `json:"id"`
	OperationID   string         `json:"operation_id"`
	EventType     AuditEventType `json:"event_type"`
	ActorID       string         `json:"actor_id"`
	PreviousState string         `json:"previous_state,omitempty"`
	NewState      string         `json:"new_state,omitempty"`
	Detail        map[string]any `json:"detail"`
	OccurredAt    string         `json:"occurred_at"`
}

type AuditEvidence struct {
	OperationID  string             `json:"operation_id"`
	InitiatedBy  string             `json:"initiated_by"`
	ApprovedBy   string             `json:"approved_by,omitempty"`
	ReleaseID    string             `json:"release_id"`
	PlanDigest   string             `json:"plan_digest"`
	PolicyResult *PolicyResult      `json:"policy_result"`
	ProviderInfo []ProviderRef      `json:"provider_info"`
	Steps        []StepEvidence     `json:"steps"`
	FinalStatus  OperationStatus    `json:"final_status"`
	RollbackInfo []RollbackEvidence `json:"rollback_evidence,omitempty"`
}

type ProviderRef struct {
	StepID     string `json:"step_id"`
	ProviderID string `json:"provider_id"`
	Version    string `json:"version"`
}

type StepEvidence struct {
	StepID      string     `json:"step_id"`
	Name        string     `json:"name"`
	Status      StepStatus `json:"status"`
	StartedAt   string     `json:"started_at,omitempty"`
	CompletedAt string     `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	OutputCount int        `json:"output_count"`
}

type RollbackEvidence struct {
	StepID       string `json:"step_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Action       string `json:"action"`
	Status       string `json:"status"`
}

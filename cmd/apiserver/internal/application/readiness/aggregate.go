package readiness

import "fmt"

const (
	ReasonExecutionFailed  = "EXECUTION_FAILED"
	ReasonExecutionPending = "EXECUTION_PENDING"
	ReasonObservationStale = "OBSERVATION_STALE"
	ReasonObservationOld   = "OBSERVATION_BEHIND"
	ReasonUIDMismatch      = "RESOURCE_UID_MISMATCH"
	ReasonRevisionMismatch = "REVISION_MISMATCH"
	ReasonExternalFailed   = "EXTERNAL_PROBE_FAILED"
	ReasonVolumeFailed     = "VOLUME_READ_WRITE_FAILED"
)

type Evidence struct {
	Revision                    int64
	ExpectedRevision            int64
	ResourceUID                 string
	ExpectedResourceUID         string
	ObservationGeneration       int64
	ObservationRevision         int64
	ExpectedGeneration          int64
	ExpectedObservationRevision int64
	ObservationFreshness        string
	ExecutionStatus             string
	ExternalProbeStatus         string
	ExternalProbeURL            string
	VolumeStatus                string
	PVCName                     string
	MarkerSHA256                string
}

type Result struct {
	Ready       bool     `json:"ready"`
	Revision    int64    `json:"revision"`
	ResourceUID string   `json:"resourceUid"`
	Execution   string   `json:"execution"`
	Observation string   `json:"observation"`
	External    string   `json:"externalProbe"`
	Volume      string   `json:"volume"`
	Reasons     []string `json:"reasons"`
}

// Evaluate keeps execution, observation, entry and volume dimensions separate.
// It never rewrites the execution terminal state when later verification fails.
func Evaluate(e Evidence) Result {
	result := Result{
		Revision: e.Revision, ResourceUID: e.ResourceUID,
		Execution: e.ExecutionStatus, Observation: e.ObservationFreshness,
		External: e.ExternalProbeStatus, Volume: e.VolumeStatus,
		Reasons: []string{},
	}
	if e.Revision != e.ExpectedRevision {
		result.Reasons = append(result.Reasons, ReasonRevisionMismatch)
	}
	if e.ExpectedResourceUID != "" && e.ResourceUID != e.ExpectedResourceUID {
		result.Reasons = append(result.Reasons, ReasonUIDMismatch)
	}
	switch e.ExecutionStatus {
	case "succeeded":
	case "failed", "unknown":
		result.Reasons = append(result.Reasons, ReasonExecutionFailed)
	default:
		result.Reasons = append(result.Reasons, ReasonExecutionPending)
	}
	if e.ObservationFreshness != "FRESH" {
		result.Reasons = append(result.Reasons, ReasonObservationStale)
	}
	if e.ObservationGeneration < e.ExpectedGeneration || e.ObservationRevision < e.ExpectedObservationRevision {
		result.Reasons = append(result.Reasons, ReasonObservationOld)
	}
	if e.ExternalProbeStatus != "passed" {
		result.Reasons = append(result.Reasons, ReasonExternalFailed)
	}
	if e.VolumeStatus != "passed" {
		result.Reasons = append(result.Reasons, ReasonVolumeFailed)
	}
	result.Ready = len(result.Reasons) == 0
	return result
}

func (r Result) Error() string {
	return fmt.Sprintf("application readiness failed: %v", r.Reasons)
}

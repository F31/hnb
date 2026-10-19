package readiness

import "testing"

func validEvidence() Evidence {
	return Evidence{
		Revision: 3, ExpectedRevision: 3, ResourceUID: "uid-3", ExpectedResourceUID: "uid-3",
		ObservationGeneration: 2, ObservationRevision: 9, ExpectedGeneration: 2, ExpectedObservationRevision: 7,
		ObservationFreshness: "FRESH", ExecutionStatus: "succeeded",
		ExternalProbeStatus: "passed", VolumeStatus: "passed",
	}
}

func TestEvaluateReadyOnlyWhenAllDimensionsPass(t *testing.T) {
	result := Evaluate(validEvidence())
	if !result.Ready || len(result.Reasons) != 0 {
		t.Fatalf("valid evidence = %+v", result)
	}
}

func TestEvaluateKeepsExecutionSucceededWhenProbeFails(t *testing.T) {
	evidence := validEvidence()
	evidence.ExternalProbeStatus = "failed"
	result := Evaluate(evidence)
	if result.Ready || result.Execution != "succeeded" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != ReasonExternalFailed {
		t.Fatalf("reasons = %v", result.Reasons)
	}
}

func TestEvaluateRejectsStaleOldOrWrongIdentity(t *testing.T) {
	evidence := validEvidence()
	evidence.ObservationFreshness = "STALE"
	evidence.ObservationRevision = 5
	evidence.ResourceUID = "uid-other"
	result := Evaluate(evidence)
	if result.Ready || len(result.Reasons) != 3 {
		t.Fatalf("result = %+v", result)
	}
}

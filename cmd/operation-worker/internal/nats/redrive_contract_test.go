package nats

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// A re-driven request is re-armed in place, so its envelope, payload and stored
// step all keep the same idempotency key. A derived-key copy was rejected by the
// worker at runtime; this pins the contract that makes the re-arm acceptable.
func TestReArmedStepRequestPassesEnvelopeValidation(t *testing.T) {
	operationID, stepID := uuid.NewString(), uuid.NewString()
	stepKey := "default:ImportRuntimeTarget:create:" + strings.Repeat("k", 40) +
		":runtime_target.kubernetes.register"

	payload, err := json.Marshal(stepRequestMessage{
		OperationID: operationID, StepID: stepID,
		StepType: "runtime_target.kubernetes.register", IdempotencyKey: stepKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(eventEnvelope{
		MessageID: uuid.NewString(), MessageType: stepRequestedSubject, SchemaVersion: "1.0.0",
		OccurredAt: time.Now().UTC(), TenantID: "default", CorrelationID: uuid.NewString(),
		IdempotencyKey: stepKey, OperationID: operationID, StepID: stepID,
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsedEnvelope eventEnvelope
	if err := json.Unmarshal(envelope, &parsedEnvelope); err != nil {
		t.Fatal(err)
	}
	var parsedRequest stepRequestMessage
	if err := json.Unmarshal(payload, &parsedRequest); err != nil {
		t.Fatal(err)
	}
	if err := validateStepRequest(parsedEnvelope, parsedRequest); err != nil {
		t.Fatalf("a re-armed request was rejected by the worker: %v", err)
	}

	// A copy carrying a derived key must fail, which is why the redrive re-arms.
	copyEnvelope := parsedEnvelope
	copyEnvelope.IdempotencyKey = parsedEnvelope.IdempotencyKey + "#redrive-1"
	if err := validateStepRequest(copyEnvelope, parsedRequest); err == nil ||
		!strings.Contains(err.Error(), "idempotencyKey differ") {
		t.Fatalf("derived-key copy was accepted: %v", err)
	}

	// The step key the platform derives must stay inside the worker limit; the API
	// rejects longer ones before an Operation is created.
	oversized := parsedEnvelope
	oversized.IdempotencyKey = strings.Repeat("k", 129)
	oversizedRequest := parsedRequest
	oversizedRequest.IdempotencyKey = oversized.IdempotencyKey
	if err := validateStepRequest(oversized, oversizedRequest); err == nil ||
		!strings.Contains(err.Error(), "128 characters") {
		t.Fatalf("an over-long step key was accepted: %v", err)
	}
}

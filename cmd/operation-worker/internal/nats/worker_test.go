package nats

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/operation-worker/internal/driver"
	"github.com/F31/hnb/cmd/operation-worker/internal/store"
	"github.com/google/uuid"
)

func TestValidateStepRequest(t *testing.T) {
	operationID := uuid.NewString()
	stepID := uuid.NewString()
	idempotencyKey := "step:" + stepID
	payload, err := json.Marshal(stepRequestMessage{
		OperationID:     operationID,
		StepID:          stepID,
		StepType:        "deploy",
		IdempotencyKey:  idempotencyKey,
		ExpectedVersion: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := eventEnvelope{
		MessageID:      uuid.NewString(),
		MessageType:    stepRequestedSubject,
		SchemaVersion:  "1.0.0",
		OccurredAt:     time.Now().UTC(),
		TenantID:       "tenant-test",
		CorrelationID:  uuid.NewString(),
		IdempotencyKey: idempotencyKey,
		OperationID:    operationID,
		StepID:         stepID,
		Payload:        payload,
	}
	var request stepRequestMessage
	if err := json.Unmarshal(envelope.Payload, &request); err != nil {
		t.Fatal(err)
	}
	if err := validateStepRequest(envelope, request); err != nil {
		t.Fatal(err)
	}

	envelope.StepID = uuid.NewString()
	if err := validateStepRequest(envelope, request); err == nil {
		t.Fatal("mismatched envelope and payload step IDs were accepted")
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 5 * time.Second},
		{attempt: 2, want: 10 * time.Second},
		{attempt: 10, want: 5 * time.Minute},
	}
	for _, test := range cases {
		if got := retryDelay(test.attempt); got != test.want {
			t.Fatalf("retryDelay(%d)=%s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestClassifyExecutionError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		permanent bool
		fenced    bool
	}{
		{name: "fenced provider", err: &driver.ProviderError{Code: driver.ErrorFenced}, fenced: true},
		{name: "lost lease", err: store.ErrLeaseLost, fenced: true},
		{name: "permanent provider", err: &driver.ProviderError{Code: driver.ErrorScopeDenied}, permanent: true},
		{name: "transient provider", err: &driver.ProviderError{Code: driver.ErrorInternal, Retryable: true}},
		{name: "untyped runner failure", err: errors.New("runner failure")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			permanent, fenced := classifyExecutionError(tt.err)
			if permanent != tt.permanent || fenced != tt.fenced {
				t.Fatalf("classifyExecutionError() = (%v, %v), want (%v, %v)", permanent, fenced, tt.permanent, tt.fenced)
			}
		})
	}
}

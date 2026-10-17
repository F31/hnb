package store

import (
	"strings"
	"testing"
)

// The worker refuses step requests whose idempotency key exceeds 128 characters.
// The derived key is always longer than the client key, so admission must reject
// it instead of creating an Operation that can never execute.
func TestValidateStepIdempotencyKeyRejectsUndeliverableKey(t *testing.T) {
	step := "runtime_target.kubernetes.register"
	if err := ValidateStepIdempotencyKey("short-key", step); err != nil {
		t.Fatalf("a normal key was rejected: %v", err)
	}
	longest := strings.Repeat("k", MaxStepIdempotencyKeyLength-len(step)-1)
	if err := ValidateStepIdempotencyKey(longest, step); err != nil {
		t.Fatalf("the longest deliverable key was rejected: %v", err)
	}
	tooLong := strings.Repeat("k", MaxStepIdempotencyKeyLength-len(step))
	err := ValidateStepIdempotencyKey(tooLong, step)
	if err == nil {
		t.Fatal("an undeliverable step key was accepted")
	}
	if !strings.Contains(err.Error(), "execution limit") {
		t.Fatalf("error does not explain the limit: %v", err)
	}
	// The same bound must hold for the real scoped operation key shape.
	scoped := "default:ImportRuntimeTarget:create:" + strings.Repeat("i", 60)
	if err := ValidateStepIdempotencyKey(scoped, step); err == nil {
		t.Fatal("a realistically scoped over-long key was accepted")
	}
}

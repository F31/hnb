package store

import "testing"

func TestInitialStatus(t *testing.T) {
	tests := []struct {
		operationType string
		want          string
	}{
		{"deploy", StatusQueued},
		{"upgrade", StatusQueued},
		{"scale", StatusQueued},
		{"backup", StatusQueued},
		{"delete", StatusPendingApproval},
		{"rollback", StatusPendingApproval},
		{"config_change", StatusPendingApproval},
	}
	for _, tt := range tests {
		if got := InitialStatus(tt.operationType); got != tt.want {
			t.Errorf("InitialStatus(%q) = %q, want %q", tt.operationType, got, tt.want)
		}
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"pending to approval", StatusPending, StatusPendingApproval, true},
		{"pending to queued", StatusPending, StatusQueued, true},
		{"approve", StatusPendingApproval, StatusQueued, true},
		{"reject", StatusPendingApproval, StatusCancelled, true},
		{"cancel queued", StatusQueued, StatusCancelled, true},
		{"cancel paused", StatusPaused, StatusCancelled, true},
		{"cancel compensating", StatusCompensating, StatusCancelled, true},
		{"cancel pending", StatusPending, StatusCancelled, true},
		{"in progress not cancellable", StatusInProgress, StatusCancelled, false},
		{"queued offline not cancellable", StatusQueuedOffline, StatusCancelled, false},
		{"terminal succeeded", StatusSucceeded, StatusCancelled, false},
		{"terminal failed", StatusFailed, StatusCancelled, false},
		{"terminal cancelled", StatusCancelled, StatusQueued, false},
		{"approve queued", StatusQueued, StatusQueued, false},
		{"api cannot start execution", StatusQueued, StatusInProgress, true}, // worker-owned, listed for parity
		{"approval cannot skip to progress", StatusPendingApproval, StatusInProgress, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestStepIdempotencyKey(t *testing.T) {
	if got := StepIdempotencyKey("op-key", "build"); got != "op-key:build" {
		t.Fatalf("StepIdempotencyKey = %q", got)
	}
}

func TestIsValidOperationType(t *testing.T) {
	for _, valid := range []string{"deploy", "rollback", "config_change", "ota"} {
		if !IsValidOperationType(valid) {
			t.Errorf("IsValidOperationType(%q) = false", valid)
		}
	}
	for _, invalid := range []string{"", "exec", "DEPLOY"} {
		if IsValidOperationType(invalid) {
			t.Errorf("IsValidOperationType(%q) = true", invalid)
		}
	}
}

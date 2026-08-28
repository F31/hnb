package stale

import "testing"

func TestPolicySupportsExactOutcomes(t *testing.T) {
	for _, outcome := range []string{"allow", "require_approval", "queued_offline", "deny"} {
		policy, err := NewPolicy(outcome, outcome)
		if err != nil {
			t.Fatalf("outcome %s: %v", outcome, err)
		}
		if got := policy.Evaluate("UpgradeRuntimeTarget"); string(got) != outcome {
			t.Fatalf("outcome = %s, want %s", got, outcome)
		}
	}
	if _, err := NewPolicy("unknown", "deny"); err == nil {
		t.Fatal("unknown policy accepted")
	}
}

package observer

import (
	"testing"
	"time"
)

func TestTargetFreshnessUsesObservedWindow(t *testing.T) {
	now := time.Unix(2_000_000_000, 0).UTC()
	if got := targetFreshness(time.Time{}, 300, now); got != "UNKNOWN" {
		t.Fatalf("zero observation = %s", got)
	}
	if got := targetFreshness(now.Add(-time.Minute), 300, now); got != "FRESH" {
		t.Fatalf("fresh observation = %s", got)
	}
	if got := targetFreshness(now.Add(-6*time.Minute), 300, now); got != "STALE" {
		t.Fatalf("stale observation = %s", got)
	}
}

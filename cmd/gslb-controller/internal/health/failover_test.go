package health

import (
	"testing"
	"time"
)

func TestFailoverTracker_HealthyStaysHealthy(t *testing.T) {
	cfg := DefaultFailoverConfig()
	ft := NewFailoverTracker(cfg)

	for i := 0; i < 10; i++ {
		state, wasHealthy := ft.RecordProbe(ProbeResult{Status: "healthy"})
		if state != StateHealthy {
			t.Errorf("expected healthy, got %s", state)
		}
		if !wasHealthy {
			t.Errorf("expected wasHealthy=true when previous state is healthy")
		}
	}
}

func TestFailoverTracker_HealthyToUnhealthy(t *testing.T) {
	cfg := DefaultFailoverConfig()
	cfg.ConsecutiveFailures = 3
	cfg.DebounceDuration = 0
	ft := NewFailoverTracker(cfg)

	for i := 0; i < cfg.ConsecutiveFailures-1; i++ {
		state, wasHealthy := ft.RecordProbe(ProbeResult{Status: "unreachable"})
		if state != StateHealthy {
			t.Errorf("expected healthy before threshold, got %s", state)
		}
		if !wasHealthy {
			t.Errorf("expected wasHealthy=true before threshold")
		}
	}

	state, wasHealthy := ft.RecordProbe(ProbeResult{Status: "unreachable"})
	if state != StateUnhealthy {
		t.Errorf("expected unhealthy after threshold, got %s", state)
	}
	if !wasHealthy {
		t.Errorf("expected wasHealthy=true when previous state was healthy")
	}
}

func TestFailoverTracker_UnhealthyToHealthy(t *testing.T) {
	cfg := DefaultFailoverConfig()
	cfg.ConsecutiveFailures = 2
	cfg.ConsecutiveSuccesses = 2
	cfg.DebounceDuration = 0
	ft := NewFailoverTracker(cfg)

	for i := 0; i < cfg.ConsecutiveFailures; i++ {
		ft.RecordProbe(ProbeResult{Status: "unreachable"})
	}
	if ft.State() != StateUnhealthy {
		t.Fatalf("expected unhealthy after failures, got %s", ft.State())
	}

	for i := 0; i < cfg.ConsecutiveSuccesses-1; i++ {
		state, wasHealthy := ft.RecordProbe(ProbeResult{Status: "healthy"})
		if state != StateUnhealthy {
			t.Errorf("expected unhealthy before recovery threshold, got %s", state)
		}
		if wasHealthy {
			t.Errorf("expected wasHealthy=false before recovery threshold")
		}
	}

	state, wasHealthy := ft.RecordProbe(ProbeResult{Status: "healthy"})
	if state != StateHealthy {
		t.Errorf("expected healthy after recovery, got %s", state)
	}
	if wasHealthy {
		t.Errorf("expected wasHealthy=false when previous state was unhealthy")
	}
}

func TestFailoverTracker_Debounce(t *testing.T) {
	cfg := DefaultFailoverConfig()
	cfg.ConsecutiveFailures = 1
	cfg.DebounceDuration = 10 * time.Second
	ft := NewFailoverTracker(cfg)

	state, wasHealthy := ft.RecordProbe(ProbeResult{Status: "unreachable"})
	if state != StateHealthy {
		t.Errorf("expected healthy during debounce, got %s", state)
	}
	if !wasHealthy {
		t.Errorf("expected wasHealthy=true when state is still healthy")
	}
}

func TestFailoverTracker_Reset(t *testing.T) {
	cfg := DefaultFailoverConfig()
	cfg.ConsecutiveFailures = 1
	cfg.DebounceDuration = 0
	ft := NewFailoverTracker(cfg)

	ft.RecordProbe(ProbeResult{Status: "unreachable"})
	if ft.State() != StateUnhealthy {
		t.Errorf("expected unhealthy before reset")
	}

	ft.Reset()
	if ft.State() != StateHealthy {
		t.Errorf("expected healthy after reset")
	}
}

func TestFailoverTracker_LastProbeResult(t *testing.T) {
	ft := NewFailoverTracker(DefaultFailoverConfig())
	ft.RecordProbe(ProbeResult{Status: "healthy"})
	if ft.LastProbeResult() != "healthy" {
		t.Errorf("expected last probe result healthy, got %s", ft.LastProbeResult())
	}
}
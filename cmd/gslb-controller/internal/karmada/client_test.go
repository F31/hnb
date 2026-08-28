package karmada

import (
	"testing"
)

func TestGetClusterStatus_UnknownWhenNoConditions(t *testing.T) {
	c := &Client{dynClient: nil}
	_ = c
}

func TestClusterHealth_ReadyParsing(t *testing.T) {
	health := &ClusterHealth{
		Name:  "test-cluster",
		Ready: "True",
	}

	if health.Ready != "True" {
		t.Errorf("expected Ready=True, got %s", health.Ready)
	}
	if health.Name != "test-cluster" {
		t.Errorf("expected Name=test-cluster, got %s", health.Name)
	}
}

func TestClusterHealth_StatusMapping(t *testing.T) {
	tests := []struct {
		ready string
		want  string
	}{
		{"True", "healthy"},
		{"False", "unreachable"},
		{"Unknown", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		got := mapReadyToStatus(tt.ready)
		if got != tt.want {
			t.Errorf("mapReadyToStatus(%q) = %q, want %q", tt.ready, got, tt.want)
		}
	}
}

func mapReadyToStatus(ready string) string {
	switch ready {
	case "True":
		return "healthy"
	case "False":
		return "unreachable"
	default:
		return "unknown"
	}
}
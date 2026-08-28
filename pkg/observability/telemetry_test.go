package observability

import (
	"context"
	"testing"
)

func TestTelemetryContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithTelemetry(ctx, "tenant-a", "corr-123", "op-456", "res-789")

	if got := GetTenantID(ctx); got != "tenant-a" {
		t.Fatalf("GetTenantID = %q, want tenant-a", got)
	}
	if got := GetCorrelationID(ctx); got != "corr-123" {
		t.Fatalf("GetCorrelationID = %q, want corr-123", got)
	}
	if got := GetOperationID(ctx); got != "op-456" {
		t.Fatalf("GetOperationID = %q, want op-456", got)
	}
	if got := GetResourceID(ctx); got != "res-789" {
		t.Fatalf("GetResourceID = %q, want res-789", got)
	}
}

func TestTelemetryContextEmpty(t *testing.T) {
	ctx := context.Background()
	if got := GetTenantID(ctx); got != "" {
		t.Fatalf("GetTenantID = %q, want empty", got)
	}
	if got := GetCorrelationID(ctx); got != "" {
		t.Fatalf("GetCorrelationID = %q, want empty", got)
	}
}

func TestNewTelemetryContext(t *testing.T) {
	ctx := WithTelemetry(context.Background(), "tenant-b", "corr-456", "", "")
	tc := NewTelemetryContext(ctx, "test-component")
	if tc.TenantID != "tenant-b" {
		t.Fatalf("TenantID = %q", tc.TenantID)
	}
	if tc.CorrelationID != "corr-456" {
		t.Fatalf("CorrelationID = %q", tc.CorrelationID)
	}
	if tc.Component != "test-component" {
		t.Fatalf("Component = %q", tc.Component)
	}
	if tc.Timestamp == "" {
		t.Fatal("Timestamp is empty")
	}
}
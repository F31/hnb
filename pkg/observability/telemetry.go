package observability

import (
	"context"
	"fmt"
	"log"
	"time"
)

type contextKey string

const (
	ctxTenantID      contextKey = "tenant_id"
	ctxCorrelationID contextKey = "correlation_id"
	ctxOperationID   contextKey = "operation_id"
	ctxResourceID    contextKey = "resource_id"
)

type TelemetryContext struct {
	TenantID      string `json:"tenant_id"`
	CorrelationID string `json:"correlation_id"`
	OperationID   string `json:"operation_id,omitempty"`
	ResourceID    string `json:"resource_id,omitempty"`
	Component     string `json:"component"`
	Timestamp     string `json:"timestamp"`
}

func WithTelemetry(ctx context.Context, tenantID, correlationID, operationID, resourceID string) context.Context {
	ctx = context.WithValue(ctx, ctxTenantID, tenantID)
	ctx = context.WithValue(ctx, ctxCorrelationID, correlationID)
	ctx = context.WithValue(ctx, ctxOperationID, operationID)
	ctx = context.WithValue(ctx, ctxResourceID, resourceID)
	return ctx
}

func GetTenantID(ctx context.Context) string {
	v, _ := ctx.Value(ctxTenantID).(string)
	return v
}

func GetCorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(ctxCorrelationID).(string)
	return v
}

func GetOperationID(ctx context.Context) string {
	v, _ := ctx.Value(ctxOperationID).(string)
	return v
}

func GetResourceID(ctx context.Context) string {
	v, _ := ctx.Value(ctxResourceID).(string)
	return v
}

func NewTelemetryContext(ctx context.Context, component string) TelemetryContext {
	return TelemetryContext{
		TenantID:      GetTenantID(ctx),
		CorrelationID: GetCorrelationID(ctx),
		OperationID:   GetOperationID(ctx),
		ResourceID:    GetResourceID(ctx),
		Component:     component,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
}

func LogTelemetry(ctx context.Context, component, format string, args ...any) {
	tc := NewTelemetryContext(ctx, component)
	log.Printf("[obs] tenant=%s correlation=%s operation=%s resource=%s component=%s %s",
		tc.TenantID, tc.CorrelationID, tc.OperationID, tc.ResourceID, tc.Component,
		fmtString(format, args...))
}

func fmtString(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
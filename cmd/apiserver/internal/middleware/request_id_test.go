package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDAcceptsTraceIDAndMirrorsCorrelationID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-Id", "11111111-1111-4111-8111-111111111111")
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: req, Response: recorder}

	NewRequestID().Handle(ctx, func() {})

	if ctx.RequestID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("request id = %q", ctx.RequestID)
	}
	if recorder.Header().Get("X-Trace-Id") != ctx.RequestID || recorder.Header().Get("X-Correlation-ID") != ctx.RequestID {
		t.Fatalf("headers not mirrored: trace=%q correlation=%q", recorder.Header().Get("X-Trace-Id"), recorder.Header().Get("X-Correlation-ID"))
	}
}

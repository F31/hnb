package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyHeadersAllowlist(t *testing.T) {
	source := http.Header{
		"Authorization":    []string{"Bearer secret"},
		"X-Tenant-Id":      []string{"tenant-a"},
		"X-Arbitrary":      []string{"value"},
		"Content-Type":     []string{"application/json"},
		"Accept":           []string{"application/json"},
		"If-Match":         []string{"etag"},
		"Idempotency-Key":  []string{"key"},
		"X-Correlation-Id": []string{"correlation"},
		"Traceparent":      []string{"trace"},
	}
	got := proxyHeaders(source)
	if len(got) != 6 {
		t.Fatalf("forwarded headers = %#v", got)
	}
	for _, forbidden := range []string{"Authorization", "X-Tenant-ID", "X-Arbitrary"} {
		if got[forbidden] != "" {
			t.Fatalf("forbidden header %s was forwarded", forbidden)
		}
	}
}

func TestProxyRequestPayloadPreservesRawQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/proxy/pods/p1/log?container=api&tailLines=200&timestamps=true", nil)
	payload := proxyRequestPayload(req, "api/v1/namespaces/default/pods/p1/log", proxyHeaders(req.Header), nil)
	if payload.Path != "api/v1/namespaces/default/pods/p1/log" {
		t.Fatalf("path = %q", payload.Path)
	}
	if payload.RawQuery != "container=api&tailLines=200&timestamps=true" {
		t.Fatalf("raw query = %q", payload.RawQuery)
	}
}

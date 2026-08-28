package observer

import (
	"net/http"
	"testing"
)

func TestReporterDoesNotUseEnvironmentProxyForInternalIngest(t *testing.T) {
	reporter := NewReporter("http://platform-api:8080/v1/observations", "token", nil, nil)
	transport, ok := reporter.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", reporter.client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("reporter must not proxy internal observation traffic")
	}
}

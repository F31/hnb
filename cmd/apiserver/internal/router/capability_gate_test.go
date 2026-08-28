package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/F31/hnb/cmd/apiserver/internal/capability"
)

func TestGateFailClosedWhenCapabilityDisabled(t *testing.T) {
	caps := capability.FromCSV("cluster.read")
	h := gate(caps, capability.Write)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime-intents", nil)
	recorder := httptest.NewRecorder()
	h(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestGatePassesWhenCapabilityEnabled(t *testing.T) {
	caps := capability.FromCSV("cluster.read,cluster.write")
	h := gate(caps, capability.Write)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", nil)
	recorder := httptest.NewRecorder()
	h(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestGateUnknownCapabilityFailClosed(t *testing.T) {
	caps := capability.AllStages()
	h := gate(caps, "cluster.nope")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	h(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 for unknown capability", recorder.Code)
	}
}

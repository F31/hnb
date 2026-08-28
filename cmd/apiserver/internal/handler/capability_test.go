package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/F31/hnb/cmd/apiserver/internal/capability"
)

func TestCapabilityHandlerListReturnsEnabledStages(t *testing.T) {
	h := NewCapabilityHandler(capability.FromCSV("cluster.read,cluster.schema"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	recorder := httptest.NewRecorder()
	h.List(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data struct {
			Stages []string `json:"stages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Stages) != 2 {
		t.Fatalf("stages = %v", body.Data.Stages)
	}
}

func TestCapabilityHandlerGetReportsAvailability(t *testing.T) {
	h := NewCapabilityHandler(capability.FromCSV("cluster.read"))
	cases := []struct {
		name string
		want bool
	}{
		{"cluster.read", true},
		{"cluster.write", false},
		{"cluster.nope", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities/"+tc.name, nil)
		req.SetPathValue("name", tc.name)
		recorder := httptest.NewRecorder()
		h.Get(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", tc.name, recorder.Code)
		}
		var body struct {
			Data map[string]bool `json:"data"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Data["available"] != tc.want {
			t.Fatalf("%s available = %v, want %v", tc.name, body.Data["available"], tc.want)
		}
	}
}

func TestCapabilityHandlerGetUnknownNameFailsClosed(t *testing.T) {
	h := NewCapabilityHandler(capability.AllStages())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities/cluster.nope", nil)
	req.SetPathValue("name", "cluster.nope")
	recorder := httptest.NewRecorder()
	h.Get(recorder, req)
	var body struct {
		Data map[string]bool `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data["available"] {
		t.Fatal("unknown capability must fail closed")
	}
}

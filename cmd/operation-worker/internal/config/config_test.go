package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseProviderEndpoints(t *testing.T) {
	got, err := parseProviderEndpoints(`{"k8s-prod":{"endpoint":"https://provider.example/v2/steps:execute","audience":"hnb-kubernetes-provider","tokenFile":"/var/run/secrets/k8s-token"}}`)
	if err != nil {
		t.Fatalf("parse endpoints: %v", err)
	}
	want := map[string]RuntimeProvider{"k8s-prod": {Endpoint: "https://provider.example/v2/steps:execute", Audience: "hnb-kubernetes-provider", TokenFile: "/var/run/secrets/k8s-token"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("endpoints = %#v, want %#v", got, want)
	}
}

func TestParseProviderEndpointsRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		value string
		part  string
	}{
		{name: "not object", value: `[]`, part: "JSON object"},
		{name: "invalid provider", value: `{"provider":42}`, part: "configuration object"},
		{name: "duplicate", value: `{"provider":"http://one","provider":"http://two"}`, part: "duplicate provider ID"},
		{name: "trailing", value: `{} {}`, part: "unexpected trailing token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseProviderEndpoints(tt.value)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("error = %v, want containing %q", err, tt.part)
			}
		})
	}
}

func TestLoadRuntimeProviders(t *testing.T) {
	t.Setenv("RUNTIME_PROVIDERS", `{"edge":{"endpoint":"http://edge-provider:8080/v2/steps:execute","audience":"hnb-edge-provider","tokenFile":"/var/run/secrets/edge-token"}}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.RuntimeProviders["edge"]; got.Endpoint != "http://edge-provider:8080/v2/steps:execute" || got.Audience != "hnb-edge-provider" {
		t.Fatalf("edge provider = %#v", got)
	}
}

func TestLoadRuntimeProvidersFailsClosedWithoutIdentityConfiguration(t *testing.T) {
	t.Setenv("RUNTIME_PROVIDERS", `{"edge":"http://edge-provider:8080/v2/steps:execute"}`)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "audience and tokenFile") {
		t.Fatalf("error = %v", err)
	}
}

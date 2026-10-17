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
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "non-wildcard audience") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadServiceIdentityBindings(t *testing.T) {
	t.Setenv("RUNTIME_PROVIDERS", `{"k8s":{"endpoint":"https://provider.example/v2/steps:execute","audience":"hnb-kubernetes-provider","serviceIdentities":{"tenant-a":{"identityId":"4e69ee9e-5a18-46e3-b330-1349e0d74166","subjectId":"c9114c57-4265-40fb-9b1b-7d5021637825","tokenFile":"/var/run/secrets/a"},"tenant-b":{"identityId":"a69a8f5f-0872-4c8e-8df9-d9e6179050af","subjectId":"2f2c9f5e-6a11-4b1e-8c4e-0c2b7a9e0f11","tokenFile":"/var/run/secrets/b"}}}}`)
	t.Setenv("SERVICE_IDENTITY_ISSUER_URL", "https://apiserver.example")
	t.Setenv("SERVICE_IDENTITY_ISSUER", "https://issuer.example")
	t.Setenv("SERVICE_IDENTITY_KEY_MANIFEST_FILE", "/var/run/secrets/keys.json")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.ServiceIdentitiesEnabled() {
		t.Fatal("service identities were not detected")
	}
	provider := cfg.RuntimeProviders["k8s"]
	if provider.TokenFile != "" || len(provider.ServiceIdentities) != 2 || provider.ServiceIdentities["tenant-a"].TokenFile != "/var/run/secrets/a" {
		t.Fatalf("unexpected provider: %#v", provider)
	}
}

func TestLoadServiceIdentityBindingsFailClosed(t *testing.T) {
	valid := `{"k8s":{"endpoint":"https://provider.example/v2/steps:execute","audience":"hnb-kubernetes-provider","serviceIdentities":{"tenant-a":{"identityId":"4e69ee9e-5a18-46e3-b330-1349e0d74166","subjectId":"c9114c57-4265-40fb-9b1b-7d5021637825","tokenFile":"/var/run/secrets/a"}}}}`
	for _, tc := range []struct {
		name   string
		value  string
		issuer string
		part   string
	}{
		{name: "missing global config", value: valid, part: "SERVICE_IDENTITY_ISSUER"},
		{name: "missing subject", value: `{"k8s":{"endpoint":"https://provider.example/v2/steps:execute","audience":"hnb-kubernetes-provider","serviceIdentities":{"tenant-a":{"identityId":"4e69ee9e-5a18-46e3-b330-1349e0d74166","tokenFile":"/var/run/secrets/a"}}}}`, issuer: "https://issuer.example", part: "UUID subjectId"},
		{name: "missing token file", value: `{"k8s":{"endpoint":"https://provider.example/v2/steps:execute","audience":"hnb-kubernetes-provider","serviceIdentities":{"tenant-a":{"identityId":"4e69ee9e-5a18-46e3-b330-1349e0d74166","subjectId":"c9114c57-4265-40fb-9b1b-7d5021637825"}}}}`, issuer: "https://issuer.example", part: "tokenFile bootstrap"},
		{name: "wildcard tenant", value: `{"k8s":{"endpoint":"https://provider.example/v2/steps:execute","audience":"hnb-kubernetes-provider","serviceIdentities":{"*":{"identityId":"4e69ee9e-5a18-46e3-b330-1349e0d74166","subjectId":"c9114c57-4265-40fb-9b1b-7d5021637825","tokenFile":"/var/run/secrets/a"}}}}`, issuer: "https://issuer.example", part: "invalid tenant"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RUNTIME_PROVIDERS", tc.value)
			t.Setenv("SERVICE_IDENTITY_ISSUER_URL", "https://apiserver.example")
			t.Setenv("SERVICE_IDENTITY_ISSUER", tc.issuer)
			t.Setenv("SERVICE_IDENTITY_KEY_MANIFEST_FILE", "/var/run/secrets/keys.json")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.part) {
				t.Fatalf("error = %v, want containing %q", err, tc.part)
			}
		})
	}
}

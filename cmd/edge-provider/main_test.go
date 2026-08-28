package main

import "testing"

func TestLoadServiceAuthenticatorRequiresManifestAndBoundedPolling(t *testing.T) {
	t.Setenv("API_TOKEN_ISSUER", "https://issuer.example")
	t.Setenv("API_TOKEN_AUDIENCE", "hnb-edge-provider")
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", "")
	if _, err := loadServiceAuthenticator("hnb-edge-provider"); err == nil {
		t.Fatal("missing key manifest was accepted")
	}
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", "/missing/manifest.json")
	t.Setenv("API_TOKEN_KEY_RELOAD_INTERVAL", "61s")
	if _, err := loadServiceAuthenticator("hnb-edge-provider"); err == nil {
		t.Fatal("polling over 60 seconds was accepted")
	}
}

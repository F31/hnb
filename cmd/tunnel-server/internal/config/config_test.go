package config

import "testing"

func TestValidateIdentityConfigRequiresManifestAndBoundedPolling(t *testing.T) {
	cfg := Config{TokenIssuer: "https://issuer.example", TokenAudience: "hnb-apiserver-tunnel", TokenKeyManifestPath: "/keys/manifest.json"}
	if err := validateIdentityConfig(&cfg, "1s"); err != nil {
		t.Fatal(err)
	}
	cfg.TokenKeyManifestPath = ""
	if err := validateIdentityConfig(&cfg, "5s"); err == nil {
		t.Fatal("missing key manifest was accepted")
	}
	cfg.TokenKeyManifestPath = "/keys/manifest.json"
	if err := validateIdentityConfig(&cfg, "500ms"); err == nil {
		t.Fatal("sub-second polling was accepted")
	}
}

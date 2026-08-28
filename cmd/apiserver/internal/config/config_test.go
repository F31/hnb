package config

import "testing"

func TestValidateIdentityConfigRequiresManifestAndBoundedPolling(t *testing.T) {
	base := Config{TokenIssuer: "https://issuer.example", TokenAudience: "hnb-apiserver", TokenPrivateKeyPath: "/keys/private.pem", TokenKeyManifestPath: "/keys/manifest.json"}
	if err := validateIdentityConfig(&base, "hnb-apiserver,hnb-platform-api", "5s"); err != nil {
		t.Fatal(err)
	}
	missing := base
	missing.TokenKeyManifestPath = ""
	if err := validateIdentityConfig(&missing, "hnb-apiserver", "5s"); err == nil {
		t.Fatal("missing key manifest was accepted")
	}
	invalid := base
	if err := validateIdentityConfig(&invalid, "hnb-apiserver", "61s"); err == nil {
		t.Fatal("polling over 60 seconds was accepted")
	}
}

package messaging

import (
	"strings"
	"testing"
)

func TestNATSConfigFailsClosedWithoutCredentials(t *testing.T) {
	for _, key := range []string{"NATS_CREDS_FILE", "NATS_NKEY_SEED_FILE", "NATS_TLS_CERT_FILE", "NATS_TLS_KEY_FILE", "NATS_TLS_CA_FILE", "NATS_INSECURE"} {
		t.Setenv(key, "")
	}
	_, err := NATSConfigFromEnv("nats://localhost:4222")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("error = %v", err)
	}
}

func TestNATSConfigAllowsExplicitDevelopmentInsecureMode(t *testing.T) {
	t.Setenv("NATS_INSECURE", "true")
	config, err := NATSConfigFromEnv("nats://localhost:4222")
	if err != nil || !config.AllowInsecure {
		t.Fatalf("config = %#v, err = %v", config, err)
	}
}

func TestNATSConfigAcceptsIndependentCredentialPaths(t *testing.T) {
	for name, config := range map[string]NATSConfig{
		"creds": {URL: "nats://nats:4222", Credentials: "/run/nats/client.creds"},
		"nkey":  {URL: "nats://nats:4222", NKeySeed: "/run/nats/client.nk"},
		"mtls":  {URL: "tls://nats:4222", TLSCert: "/run/nats/tls.crt", TLSKey: "/run/nats/tls.key", TLSCA: "/run/nats/ca.crt"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := config.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

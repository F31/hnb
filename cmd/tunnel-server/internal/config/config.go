package config

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

type Config struct {
	ListenAddr             string
	NATSURL                string
	TokenIssuer            string
	TokenAudience          string
	TokenKeyManifestPath   string
	TokenKeyReloadInterval time.Duration
	MetricsAddr            string
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.ListenAddr, "listen", envOrDefault("LISTEN_ADDR", ":9443"), "Tunnel WebSocket listen address")
	flag.StringVar(&cfg.NATSURL, "nats-url", envOrDefault("NATS_URL", "nats://localhost:4222"), "NATS server URL")
	flag.StringVar(&cfg.TokenIssuer, "token-issuer", os.Getenv("API_TOKEN_ISSUER"), "Access-token issuer")
	flag.StringVar(&cfg.TokenAudience, "token-audience", envOrDefault("API_TOKEN_AUDIENCE", "hnb-apiserver-tunnel"), "Exact tunnel token audience")
	flag.StringVar(&cfg.TokenKeyManifestPath, "token-key-manifest", os.Getenv("API_TOKEN_KEY_MANIFEST_FILE"), "Versioned signing-key manifest file")
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", envOrDefault("METRICS_ADDR", ":8080"), "Prometheus metrics address")

	flag.Parse()

	if err := validateIdentityConfig(cfg, envOrDefault("API_TOKEN_KEY_RELOAD_INTERVAL", "5s")); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateIdentityConfig(cfg *Config, reloadInterval string) error {
	if cfg.TokenIssuer == "" || cfg.TokenAudience != "hnb-apiserver-tunnel" || cfg.TokenKeyManifestPath == "" {
		return fmt.Errorf("API_TOKEN_ISSUER, API_TOKEN_AUDIENCE=hnb-apiserver-tunnel, and API_TOKEN_KEY_MANIFEST_FILE are required")
	}
	var err error
	cfg.TokenKeyReloadInterval, err = iam.ParseKeyReloadInterval(reloadInterval)
	if err != nil {
		return err
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

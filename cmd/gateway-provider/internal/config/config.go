package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	NATSURL        string
	NATSToken      string
	NATSCredsFile  string
	NATSCertFile   string
	NATSKeyFile    string
	NATSCAFile     string
	Kubeconfig     string
	GatewayAdapter string
}

func Load() (*Config, error) {
	cfg := &Config{
		NATSURL:        getEnv("NATS_URL", "nats://localhost:4222"),
		NATSToken:      getEnv("NATS_TOKEN", ""),
		NATSCredsFile:  getEnv("NATS_CREDS", ""),
		NATSCertFile:   getEnv("NATS_TLS_CERT", ""),
		NATSKeyFile:    getEnv("NATS_TLS_KEY", ""),
		NATSCAFile:     getEnv("NATS_TLS_CA", ""),
		Kubeconfig:     getEnv("KUBECONFIG", ""),
		GatewayAdapter: getEnv("GATEWAY_ADAPTER", "istio"),
	}

	if cfg.NATSURL == "" {
		return nil, fmt.Errorf("NATS_URL is required")
	}
	adapter := strings.ToLower(cfg.GatewayAdapter)
	if adapter != "istio" && adapter != "cilium" {
		return nil, fmt.Errorf("GATEWAY_ADAPTER must be 'istio' or 'cilium', got %q", cfg.GatewayAdapter)
	}
	cfg.GatewayAdapter = adapter
	if cfg.Kubeconfig != "" {
		if _, err := os.Stat(cfg.Kubeconfig); err != nil {
			return nil, fmt.Errorf("KUBECONFIG %q: %w", cfg.Kubeconfig, err)
		}
	}

	return cfg, nil
}

func (c *Config) DSN() string {
	return ""
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
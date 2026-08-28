package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	NATSURL    string
	ListenAddr string
	DBDSN      string
	ReconcileInterval int
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.NATSURL, "nats-url", envOrDefault("NATS_URL", "nats://localhost:4222"), "NATS server URL")
	flag.StringVar(&cfg.ListenAddr, "listen", envOrDefault("LISTEN_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&cfg.DBDSN, "db-dsn", envOrDefault("DB_DSN", ""), "PostgreSQL DSN")
	flag.IntVar(&cfg.ReconcileInterval, "reconcile-interval", envOrDefaultInt("RECONCILE_INTERVAL", 60), "Reconciliation interval in seconds")

	flag.Parse()

	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN is required")
	}
	if cfg.NATSURL == "" {
		return nil, fmt.Errorf("NATS_URL is required")
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

type Config struct {
	ListenAddr string
	DBDSN      string
	NATSURL    string
	EvalInterval time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.ListenAddr, "listen", envOrDefault("LISTEN_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&cfg.DBDSN, "db-dsn", envOrDefault("DB_DSN", ""), "PostgreSQL DSN")
	flag.StringVar(&cfg.NATSURL, "nats-url", envOrDefault("NATS_URL", "nats://localhost:4222"), "NATS server URL")
	flag.DurationVar(&cfg.EvalInterval, "eval-interval", envOrDefaultDuration("EVAL_INTERVAL", 30*time.Second), "Alert evaluation interval")

	flag.Parse()

	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN is required")
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrDefaultDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}
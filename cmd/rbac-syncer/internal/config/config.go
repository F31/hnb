package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Database
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	// Sync
	PollInterval  time.Duration
	SyncTimeout   time.Duration
	MaxRetries    int
	ShadowMode    bool
	NamespaceName string

	// K8s
	KubeConfigPath string
	KubeQPS        float32
	KubeBurst      int

	// Observability
	MetricsAddr string
	HealthAddr  string
}

func Load() (*Config, error) {
	port, err := strconv.Atoi(envOr("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}

	interval, err := time.ParseDuration(envOr("POLL_INTERVAL", "30s"))
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
	}

	timeout, err := time.ParseDuration(envOr("SYNC_TIMEOUT", "10s"))
	if err != nil {
		return nil, fmt.Errorf("invalid SYNC_TIMEOUT: %w", err)
	}

	maxRetries, err := strconv.Atoi(envOr("MAX_RETRIES", "3"))
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_RETRIES: %w", err)
	}

	qps, err := strconv.ParseFloat(envOr("KUBE_QPS", "50"), 32)
	if err != nil {
		return nil, fmt.Errorf("invalid KUBE_QPS: %w", err)
	}

	burst, err := strconv.Atoi(envOr("KUBE_BURST", "100"))
	if err != nil {
		return nil, fmt.Errorf("invalid KUBE_BURST: %w", err)
	}

	return &Config{
		DBHost:         envOr("DB_HOST", "localhost"),
		DBPort:         port,
		DBUser:         envOr("DB_USER", "hnb"),
		DBPassword:     envOr("DB_PASSWORD", ""),
		DBName:         envOr("DB_NAME", "hnb"),
		PollInterval:   interval,
		SyncTimeout:    timeout,
		MaxRetries:     maxRetries,
		ShadowMode:     envOr("SHADOW_MODE", "true") == "true",
		NamespaceName:  envOr("NAMESPACE", "hnb-system"),
		KubeConfigPath: envOr("KUBECONFIG", ""),
		KubeQPS:        float32(qps),
		KubeBurst:      burst,
		MetricsAddr:    envOr("METRICS_ADDR", ":8080"),
		HealthAddr:     envOr("HEALTH_ADDR", ":8081"),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

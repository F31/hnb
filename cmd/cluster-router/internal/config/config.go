package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

type Config struct {
	ListenAddr      string
	TunnelAPIURL    string
	MetricsAddr     string
	BalancerType    string
	PoolMaxSize     int
	PoolTTL         time.Duration
	HealthCheckInt  time.Duration
	CircuitThreshold int
	CircuitReset    time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.ListenAddr, "listen", envOrDefault("LISTEN_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&cfg.TunnelAPIURL, "tunnel-api", envOrDefault("TUNNEL_API_URL", "http://localhost:9443"), "Tunnel server API URL")
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", envOrDefault("METRICS_ADDR", ":8081"), "Prometheus metrics address")
	flag.StringVar(&cfg.BalancerType, "balancer", envOrDefault("BALANCER_TYPE", "round_robin"), "Load balancer type (round_robin, least_connections, random)")
	flag.IntVar(&cfg.PoolMaxSize, "pool-max", envOrDefaultInt("POOL_MAX_SIZE", 1000), "Connection pool max size")
	flag.DurationVar(&cfg.PoolTTL, "pool-ttl", envOrDefaultDuration("POOL_TTL", 5*time.Minute), "Connection pool TTL")
	flag.DurationVar(&cfg.HealthCheckInt, "health-interval", envOrDefaultDuration("HEALTH_INTERVAL", 10*time.Second), "Health check interval")
	flag.IntVar(&cfg.CircuitThreshold, "circuit-threshold", envOrDefaultInt("CIRCUIT_THRESHOLD", 3), "Circuit breaker failure threshold")
	flag.DurationVar(&cfg.CircuitReset, "circuit-reset", envOrDefaultDuration("CIRCUIT_RESET", 30*time.Second), "Circuit breaker reset timeout")

	flag.Parse()

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

func envOrDefaultDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}
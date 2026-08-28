package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TunnelURL            string
	TokenFile            string
	TenantID             string
	ClusterID            string
	KubeAPI              string
	KubeToken            string
	KubeTokenFile        string
	KubeCAFile           string
	Hostname             string
	ReconnectInt         int
	ObservationIngestURL string
	ObserverTokenFile    string
	ObserverGeneration   int64
	ObservationInterval  time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.TunnelURL, "tunnel-url", envOrDefault("TUNNEL_URL", "ws://localhost:9443/tunnel"), "Tunnel server WebSocket URL")
	flag.StringVar(&cfg.TokenFile, "agent-token-file", os.Getenv("AGENT_TOKEN_FILE"), "Rotating agent service-token file")
	flag.StringVar(&cfg.TenantID, "tenant-id", os.Getenv("TENANT_ID"), "Authorized tenant ID")
	flag.StringVar(&cfg.ClusterID, "cluster-id", os.Getenv("CLUSTER_ID"), "Cluster ID for this agent")
	flag.StringVar(&cfg.KubeAPI, "kube-api", envOrDefault("KUBE_API", "https://kubernetes.default.svc"), "Local Kubernetes API URL")
	flag.StringVar(&cfg.KubeToken, "kube-token", envOrDefault("KUBE_TOKEN", ""), "Kubernetes service account token")
	flag.StringVar(&cfg.KubeTokenFile, "kube-token-file", os.Getenv("KUBE_TOKEN_FILE"), "Kubernetes service account token file")
	flag.StringVar(&cfg.KubeCAFile, "kube-ca-file", os.Getenv("KUBE_CA_FILE"), "Kubernetes API CA certificate file")
	flag.StringVar(&cfg.Hostname, "hostname", envOrDefault("HOSTNAME", ""), "Agent hostname")
	flag.IntVar(&cfg.ReconnectInt, "reconnect-interval", envOrDefaultInt("RECONNECT_INTERVAL", 10), "Reconnection interval in seconds")
	flag.StringVar(&cfg.ObservationIngestURL, "observation-ingest-url", os.Getenv("OBSERVATION_INGEST_URL"), "Platform observation ingest endpoint")
	flag.StringVar(&cfg.ObserverTokenFile, "observer-token-file", os.Getenv("OBSERVER_TOKEN_FILE"), "Signed observer identity token file")
	flag.Int64Var(&cfg.ObserverGeneration, "observer-generation", envOrDefaultInt64("OBSERVER_GENERATION", 1), "Initial observer generation")
	flag.DurationVar(&cfg.ObservationInterval, "observation-interval", envOrDefaultDuration("OBSERVATION_INTERVAL", 60*time.Second), "Observation report interval")

	flag.Parse()
	if cfg.KubeTokenFile != "" {
		token, err := readKubeTokenFile(cfg.KubeTokenFile)
		if err != nil {
			return nil, err
		}
		cfg.KubeToken = token
	}

	if cfg.TokenFile == "" {
		return nil, fmt.Errorf("AGENT_TOKEN_FILE is required")
	}
	if cfg.TenantID == "" || cfg.TenantID == "*" || cfg.ClusterID == "" || cfg.ClusterID == "*" {
		return nil, fmt.Errorf("explicit TENANT_ID and CLUSTER_ID are required")
	}
	if cfg.ObservationIngestURL != "" {
		if cfg.ObserverTokenFile == "" {
			return nil, fmt.Errorf("OBSERVER_TOKEN_FILE is required when observation ingest is enabled")
		}
		if cfg.ObservationInterval < 5*time.Second {
			return nil, fmt.Errorf("OBSERVATION_INTERVAL must be at least 5s")
		}
	}

	if cfg.Hostname == "" {
		cfg.Hostname, _ = os.Hostname()
	}

	return cfg, nil
}

func readKubeTokenFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read KUBE_TOKEN_FILE: %w", err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", fmt.Errorf("KUBE_TOKEN_FILE is empty")
	}
	return token, nil
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

func envOrDefaultInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envOrDefaultDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

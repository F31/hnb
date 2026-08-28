package network

import (
	"os"
)

type Config struct {
	NATSURL    string
	Kubeconfig string
	LogLevel   string
	HelmPath   string
}

func Load() *Config {
	return &Config{
		NATSURL:    getEnv("NATS_URL", "nats://localhost:4222"),
		Kubeconfig: getEnv("KUBECONFIG", ""),
		LogLevel:   getEnv("LOG_LEVEL", "info"),
		HelmPath:   getEnv("HELM_PATH", "helm"),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
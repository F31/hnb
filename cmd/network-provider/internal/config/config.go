package config

import (
	"os"
)

type Config struct {
	NATSURL       string
	DBDSN         string
	MasterKeyHex  string
	Kubeconfig    string
	LogLevel      string
	HelmPath      string
	KubeconfigDir string
}

func Load() *Config {
	return &Config{
		NATSURL:       getEnv("NATS_URL", "nats://localhost:4222"),
		DBDSN:         getEnv("DB_DSN", ""),
		MasterKeyHex:  getEnv("HNB_MASTER_KEY", ""),
		Kubeconfig:    getEnv("KUBECONFIG", ""),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		HelmPath:      getEnv("HELM_PATH", "helm"),
		KubeconfigDir: getEnv("KUBECONFIG_DIR", "/tmp/hnb-network-provider"),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
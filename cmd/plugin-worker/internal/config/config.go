package config

import "os"

type Config struct {
	DBDSN         string
	NATSURL       string
	MasterKeyHex  string
	HelmPath      string
	ListenAddr    string
	KubeconfigDir string
}

func Load() Config {
	return Config{
		DBDSN:         getEnv("DB_DSN", ""),
		NATSURL:       getEnv("NATS_URL", ""),
		MasterKeyHex:  getEnv("HNB_MASTER_KEY", ""),
		HelmPath:      getEnv("HELM_PATH", "helm"),
		ListenAddr:    getEnv("LISTEN_ADDR", ":8080"),
		KubeconfigDir: getEnv("KUBECONFIG_DIR", "/tmp/hnb-plugin-worker"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

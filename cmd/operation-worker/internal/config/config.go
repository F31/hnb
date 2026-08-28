package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBHost           string
	DBPort           int
	DBUser           string
	DBPassword       string
	DBName           string
	DBSSLMode        string
	NATSURL          string
	LeaseDuration    time.Duration
	RuntimeProviders map[string]RuntimeProvider
	LogLevel         string
	WorkerPoolSize   int
}

type RuntimeProvider struct {
	Endpoint         string `json:"endpoint"`
	Audience         string `json:"audience"`
	TokenFile        string `json:"tokenFile"`
	ProtocolVersion  string `json:"protocolVersion"`
	ProviderVersion  string `json:"providerVersion"`
	ProviderDigest   string `json:"providerDigest"`
	RequiredProvider string `json:"requiredProvider"`
}

func Load() (*Config, error) {
	port, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	leaseSec, _ := strconv.Atoi(getEnv("LEASE_DURATION_SECONDS", "60"))
	poolSize, _ := strconv.Atoi(getEnv("WORKER_POOL_SIZE", "5"))
	providers, err := parseProviderEndpoints(getEnv("RUNTIME_PROVIDERS", "{}"))
	if err != nil {
		return nil, fmt.Errorf("RUNTIME_PROVIDERS: %w", err)
	}
	for providerID, provider := range providers {
		if provider.Audience == "" || provider.Audience == "*" || provider.TokenFile == "" {
			return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q requires a non-wildcard audience and tokenFile", providerID)
		}
		if provider.ProtocolVersion == "" {
			provider.ProtocolVersion = "2.0.0"
		}
		if provider.ProtocolVersion != "2.0.0" {
			return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q protocolVersion %q is not supported", providerID, provider.ProtocolVersion)
		}
		if provider.RequiredProvider != "" && provider.RequiredProvider != providerID {
			return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q requiredProvider %q must match provider ID", providerID, provider.RequiredProvider)
		}
		if provider.RequiredProvider != "" {
			if provider.ProviderVersion == "" || provider.ProviderDigest == "" {
				return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q requires providerVersion and providerDigest", providerID)
			}
		}
		providers[providerID] = provider
	}

	return &Config{
		DBHost:           getEnv("DB_HOST", "localhost"),
		DBPort:           port,
		DBUser:           getEnv("DB_USER", "hnb"),
		DBPassword:       getEnv("DB_PASSWORD", ""),
		DBName:           getEnv("DB_NAME", "hnb"),
		DBSSLMode:        getEnv("DB_SSLMODE", "disable"),
		NATSURL:          getEnv("NATS_URL", "nats://localhost:4222"),
		LeaseDuration:    time.Duration(leaseSec) * time.Second,
		RuntimeProviders: providers,
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		WorkerPoolSize:   poolSize,
	}, nil
}

func parseProviderEndpoints(value string) (map[string]RuntimeProvider, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	start, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := start.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("must be a JSON object")
	}

	providers := make(map[string]RuntimeProvider)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		providerID, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("provider ID must be a string")
		}
		if _, exists := providers[providerID]; exists {
			return nil, fmt.Errorf("duplicate provider ID %q", providerID)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("provider %q: %w", providerID, err)
		}
		var provider RuntimeProvider
		if err := json.Unmarshal(raw, &provider); err != nil {
			var endpoint string
			if stringErr := json.Unmarshal(raw, &endpoint); stringErr != nil {
				return nil, fmt.Errorf("provider %q must be an endpoint string or configuration object: %w", providerID, err)
			}
			provider.Endpoint = endpoint
		}
		providers[providerID] = provider
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected trailing token %v", token)
	}
	return providers, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

func (c *Config) NATSOptions() []string {
	return []string{c.NATSURL}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

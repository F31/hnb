package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/db"
	"github.com/google/uuid"
)

type Config struct {
	DBDSN                            string
	DBHost                           string
	DBPort                           int
	DBUser                           string
	DBPassword                       string
	DBName                           string
	DBSSLMode                        string
	NATSURL                          string
	LeaseDuration                    time.Duration
	RuntimeProviders                 map[string]RuntimeProvider
	ServiceIdentityURL               string
	ServiceIdentityIssuer            string
	ServiceIdentityKeyManifest       string
	ServiceIdentityKeyReloadInterval time.Duration
	LogLevel                         string
	WorkerPoolSize                   int
}

type RuntimeProvider struct {
	Endpoint          string                     `json:"endpoint"`
	Audience          string                     `json:"audience"`
	TokenFile         string                     `json:"tokenFile"`
	ServiceIdentities map[string]ServiceIdentity `json:"serviceIdentities"`
	ProtocolVersion   string                     `json:"protocolVersion"`
	ProviderVersion   string                     `json:"providerVersion"`
	ProviderDigest    string                     `json:"providerDigest"`
	RequiredProvider  string                     `json:"requiredProvider"`
}

// ServiceIdentity declares the owning-plane identity and bootstrap credential
// for one execution tenant. identityId is the registered service identity;
// subjectId is its canonical subject. The bootstrap file is only the first
// credential: the worker renews it and never falls back to the file on failure.
type ServiceIdentity struct {
	IdentityID string `json:"identityId"`
	SubjectID  string `json:"subjectId"`
	TokenFile  string `json:"tokenFile"`
}

func Load() (*Config, error) {
	port, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	leaseSec, _ := strconv.Atoi(getEnv("LEASE_DURATION_SECONDS", "60"))
	poolSize, _ := strconv.Atoi(getEnv("WORKER_POOL_SIZE", "5"))
	providers, err := parseProviderEndpoints(getEnv("RUNTIME_PROVIDERS", "{}"))
	if err != nil {
		return nil, fmt.Errorf("RUNTIME_PROVIDERS: %w", err)
	}
	serviceIdentityConfigured := false
	for providerID, provider := range providers {
		if strings.TrimSpace(providerID) == "" {
			return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider ID cannot be empty")
		}
		if provider.Audience == "" || provider.Audience == "*" {
			return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q requires a non-wildcard audience", providerID)
		}
		if len(provider.ServiceIdentities) == 0 && strings.TrimSpace(provider.TokenFile) == "" {
			return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q requires tokenFile or serviceIdentities", providerID)
		}
		if len(provider.ServiceIdentities) > 0 {
			serviceIdentityConfigured = true
			for tenantID, identity := range provider.ServiceIdentities {
				if strings.TrimSpace(tenantID) == "" || tenantID == "*" {
					return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q has an invalid tenant", providerID)
				}
				if _, err := uuid.Parse(identity.IdentityID); err != nil {
					return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q tenant %q requires a UUID identityId", providerID, tenantID)
				}
				if _, err := uuid.Parse(identity.SubjectID); err != nil {
					return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q tenant %q requires a UUID subjectId", providerID, tenantID)
				}
				if strings.TrimSpace(identity.TokenFile) == "" {
					return nil, fmt.Errorf("RUNTIME_PROVIDERS: provider %q tenant %q requires a tokenFile bootstrap", providerID, tenantID)
				}
			}
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

	serviceIdentityURL := strings.TrimSpace(os.Getenv("SERVICE_IDENTITY_ISSUER_URL"))
	serviceIdentityIssuer := strings.TrimSpace(os.Getenv("SERVICE_IDENTITY_ISSUER"))
	serviceIdentityKeyManifest := strings.TrimSpace(os.Getenv("SERVICE_IDENTITY_KEY_MANIFEST_FILE"))
	serviceIdentityReload, err := time.ParseDuration(getEnv("SERVICE_IDENTITY_KEY_RELOAD_INTERVAL", "5s"))
	if err != nil || serviceIdentityReload <= 0 {
		return nil, fmt.Errorf("SERVICE_IDENTITY_KEY_RELOAD_INTERVAL must be a positive duration")
	}
	if serviceIdentityConfigured {
		if serviceIdentityIssuer == "" || serviceIdentityKeyManifest == "" {
			return nil, fmt.Errorf("SERVICE_IDENTITY_ISSUER and SERVICE_IDENTITY_KEY_MANIFEST_FILE are required when serviceIdentities are configured")
		}
		parsed, err := url.Parse(serviceIdentityURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return nil, fmt.Errorf("SERVICE_IDENTITY_ISSUER_URL must be an absolute HTTP(S) URL without credentials")
		}
	}

	return &Config{
		DBDSN:                            os.Getenv("DB_DSN"),
		DBHost:                           getEnv("DB_HOST", "localhost"),
		DBPort:                           port,
		DBUser:                           getEnv("DB_USER", "hnb"),
		DBPassword:                       getEnv("DB_PASSWORD", ""),
		DBName:                           getEnv("DB_NAME", "hnb"),
		DBSSLMode:                        getEnv("DB_SSLMODE", "disable"),
		NATSURL:                          getEnv("NATS_URL", "nats://localhost:4222"),
		LeaseDuration:                    time.Duration(leaseSec) * time.Second,
		RuntimeProviders:                 providers,
		ServiceIdentityURL:               serviceIdentityURL,
		ServiceIdentityIssuer:            serviceIdentityIssuer,
		ServiceIdentityKeyManifest:       serviceIdentityKeyManifest,
		ServiceIdentityKeyReloadInterval: serviceIdentityReload,
		LogLevel:                         getEnv("LOG_LEVEL", "info"),
		WorkerPoolSize:                   poolSize,
	}, nil
}

func (c *Config) ServiceIdentitiesEnabled() bool {
	for _, provider := range c.RuntimeProviders {
		if len(provider.ServiceIdentities) > 0 {
			return true
		}
	}
	return false
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

// DatabaseConfig resolves the connection parameters. Deployment charts bind the
// credential as a single DB_DSN so it never has to appear as discrete variables
// in a Pod spec; prefer that when it is present.
func (c *Config) DatabaseConfig() db.DatabaseConfig {
	if c.DBDSN != "" {
		if parsed, err := db.ParsePostgresURL(c.DBDSN); err == nil {
			return parsed
		}
	}
	return db.DatabaseConfig{
		Driver: "postgres",
		Host:   c.DBHost, Port: c.DBPort, User: c.DBUser,
		Password: c.DBPassword, DBName: c.DBName, SSLMode: c.DBSSLMode,
		MaxOpenConns: 25, MaxIdleConns: 10,
		ConnMaxLifetime: 5 * time.Minute, ConnMaxIdleTime: time.Minute,
	}
}

func (c *Config) DSN() string {
	if c.DBDSN != "" {
		return c.DBDSN
	}
	config := c.DatabaseConfig()
	return config.DSN()
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

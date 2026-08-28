package config

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

type Config struct {
	ListenAddr             string
	DBDSN                  string
	NATSURL                string
	PlatformAPIURL         string
	AppMarketURL           string
	RequirePlatformAPI     bool
	ClusterProjectionMode  string
	MetricsAddr            string
	TokenIssuer            string
	TokenAudience          string
	TokenAudiences         []string
	TokenKeyManifestPath   string
	TokenPrivateKeyPath    string
	TokenKeyReloadInterval time.Duration
	ConfigDir              string
	HarborURL              string
	HarborUser             string
	HarborPass             string
	ClusterCapabilities    string
	BootstrapAdminPassword string
	PublicBaseURL          string
	AgentImage             string
}

func Load() (*Config, error) {
	cfg := &Config{}
	var audiences, reloadInterval string

	flag.StringVar(&cfg.ListenAddr, "listen", envOrDefault("LISTEN_ADDR", ":8080"), "API server listen address")
	flag.StringVar(&cfg.DBDSN, "db-dsn", envOrDefault("DB_DSN", ""), "PostgreSQL DSN")
	flag.StringVar(&cfg.NATSURL, "nats-url", envOrDefault("NATS_URL", "nats://localhost:4222"), "NATS server URL")
	flag.StringVar(&cfg.PlatformAPIURL, "platform-api-url", os.Getenv("PLATFORM_API_URL"), "Platform API base URL for northbound BFF aggregation")
	flag.StringVar(&cfg.AppMarketURL, "app-market-url", os.Getenv("APP_MARKET_URL"), "App Market API base URL for market BFF aggregation")
	flag.BoolVar(&cfg.RequirePlatformAPI, "require-platform-api", envOrDefault("REQUIRE_PLATFORM_API", "") == "true", "Require PLATFORM_API_URL and disable direct SQL fallback for platform-domain resources")
	flag.StringVar(&cfg.ClusterProjectionMode, "cluster-projection-mode", envOrDefault("CLUSTER_READ_PROJECTION_MODE", "shadow"), "Cluster read projection mode: disabled, shadow, or cutover")
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", envOrDefault("METRICS_ADDR", ":8081"), "Metrics listen address")
	flag.StringVar(&cfg.TokenIssuer, "token-issuer", os.Getenv("API_TOKEN_ISSUER"), "Access token issuer")
	cfg.TokenAudience = "hnb-apiserver"
	flag.StringVar(&audiences, "token-audiences", os.Getenv("API_TOKEN_AUDIENCES"), "Comma-separated approved access token audiences")
	flag.StringVar(&cfg.TokenPrivateKeyPath, "token-private-key", os.Getenv("API_TOKEN_PRIVATE_KEY_FILE"), "PEM ES256 private key file")
	flag.StringVar(&cfg.TokenKeyManifestPath, "token-key-manifest", os.Getenv("API_TOKEN_KEY_MANIFEST_FILE"), "Versioned signing-key manifest file")
	flag.StringVar(&reloadInterval, "token-key-reload-interval", envOrDefault("API_TOKEN_KEY_RELOAD_INTERVAL", "5s"), "Signing-key manifest reload interval")
	flag.StringVar(&cfg.ConfigDir, "config-dir", envOrDefault("CONFIG_DIR", "config"), "Config directory for routes.yaml")
	flag.StringVar(&cfg.HarborURL, "harbor-url", os.Getenv("HARBOR_URL"), "Harbor API base URL")
	flag.StringVar(&cfg.HarborUser, "harbor-user", envOrDefault("HARBOR_USERNAME", "admin"), "Harbor admin username")
	flag.StringVar(&cfg.HarborPass, "harbor-pass", os.Getenv("HARBOR_PASSWORD"), "Harbor admin password")
	flag.StringVar(&cfg.ClusterCapabilities, "cluster-capabilities", envOrDefault("CLUSTER_CAPABILITIES", ""), "Comma-separated enabled cluster capability stages (empty = all enabled): contract,schema,provider,projector,read,write")
	flag.StringVar(&cfg.BootstrapAdminPassword, "bootstrap-admin-password", os.Getenv("HNB_BOOTSTRAP_ADMIN_PASSWORD"), "If set, create the initial admin user with this password when the users table is empty")
	flag.StringVar(&cfg.PublicBaseURL, "public-base-url", os.Getenv("PUBLIC_BASE_URL"), "Public base URL of the apiserver (used to render agent TUNNEL_URL in onboarding manifests); empty derives from the request Host")
	flag.StringVar(&cfg.AgentImage, "agent-image", envOrDefault("AGENT_IMAGE", "hnb/cluster-agent:latest"), "cluster-agent image referenced by onboarding manifests")

	flag.Parse()

	if cfg.DBDSN == "" {
		cfg.DBDSN = envOrDefault("DB_DSN", "postgres://postgres:postgres@localhost:5432/hnb?sslmode=disable")
	}
	if err := validateIdentityConfig(cfg, audiences, reloadInterval); err != nil {
		return nil, err
	}
	if cfg.RequirePlatformAPI && cfg.PlatformAPIURL == "" {
		return nil, errors.New("REQUIRE_PLATFORM_API=true requires PLATFORM_API_URL")
	}
	switch cfg.ClusterProjectionMode {
	case "disabled", "shadow", "cutover":
	default:
		return nil, errors.New("CLUSTER_READ_PROJECTION_MODE must be disabled, shadow, or cutover")
	}
	return cfg, nil
}

func validateIdentityConfig(cfg *Config, audiences, reloadInterval string) error {
	if cfg.TokenIssuer == "" || audiences == "" || cfg.TokenPrivateKeyPath == "" || cfg.TokenKeyManifestPath == "" {
		return errors.New("API_TOKEN_ISSUER, API_TOKEN_AUDIENCES, API_TOKEN_PRIVATE_KEY_FILE, and API_TOKEN_KEY_MANIFEST_FILE are required")
	}
	seenAudiences := make(map[string]struct{})
	for _, audience := range strings.Split(audiences, ",") {
		audience = strings.TrimSpace(audience)
		if audience == "" || audience == "*" {
			return errors.New("API_TOKEN_AUDIENCES must contain explicit non-wildcard audiences")
		}
		if _, exists := seenAudiences[audience]; exists {
			return errors.New("API_TOKEN_AUDIENCES must not contain duplicates")
		}
		seenAudiences[audience] = struct{}{}
		cfg.TokenAudiences = append(cfg.TokenAudiences, audience)
	}
	if _, ok := seenAudiences[cfg.TokenAudience]; !ok {
		return errors.New("API_TOKEN_AUDIENCES must include hnb-apiserver")
	}
	var err error
	cfg.TokenKeyReloadInterval, err = iam.ParseKeyReloadInterval(reloadInterval)
	if err != nil {
		return err
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

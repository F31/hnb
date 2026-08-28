package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/db"
	"github.com/F31/hnb/pkg/iam"
)

type Config struct {
	DBDriver                    string
	DBHost                      string
	DBPort                      int
	DBUser                      string
	DBPassword                  string
	DBName                      string
	DBSSLMode                   string
	ListenAddr                  string
	LogLevel                    string
	TokenIssuer                 string
	TokenAudience               string
	TokenKeyManifestPath        string
	TokenKeyReloadInterval      time.Duration
	StaleChallengeKeyFile       string
	StaleChallengeTTL           time.Duration
	StaleUpgradePolicy          string
	StaleUnmanagePolicy         string
	Environment                 string
	AllowUnimplementedDBBackend bool
}

func Load() (*Config, error) {
	port, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))

	cfg := &Config{
		DBDriver:                    getEnv("DB_DRIVER", "postgres"),
		DBHost:                      getEnv("DB_HOST", "localhost"),
		DBPort:                      port,
		DBUser:                      getEnv("DB_USER", "hnb"),
		DBPassword:                  getEnv("DB_PASSWORD", ""),
		DBName:                      getEnv("DB_NAME", "hnb"),
		DBSSLMode:                   getEnv("DB_SSLMODE", "disable"),
		ListenAddr:                  getEnv("LISTEN_ADDR", ":8080"),
		LogLevel:                    getEnv("LOG_LEVEL", "info"),
		TokenIssuer:                 os.Getenv("API_TOKEN_ISSUER"),
		TokenAudience:               getEnv("API_TOKEN_AUDIENCE", "hnb-platform-api"),
		TokenKeyManifestPath:        os.Getenv("API_TOKEN_KEY_MANIFEST_FILE"),
		StaleChallengeKeyFile:       os.Getenv("STALE_CHALLENGE_KEY_FILE"),
		StaleUpgradePolicy:          getEnv("STALE_UPGRADE_POLICY", "require_approval"),
		StaleUnmanagePolicy:         getEnv("STALE_UNMANAGE_POLICY", "require_approval"),
		Environment:                 getEnv("APP_ENV", "development"),
		AllowUnimplementedDBBackend: os.Getenv("ALLOW_UNIMPLEMENTED_DB_BACKEND") == "true",
	}
	if cfg.TokenIssuer == "" || cfg.TokenAudience != "hnb-platform-api" || cfg.TokenKeyManifestPath == "" || cfg.StaleChallengeKeyFile == "" {
		return nil, errors.New("API token verifier configuration and STALE_CHALLENGE_KEY_FILE are required")
	}
	var err error
	cfg.TokenKeyReloadInterval, err = iam.ParseKeyReloadInterval(getEnv("API_TOKEN_KEY_RELOAD_INTERVAL", "5s"))
	if err != nil {
		return nil, err
	}
	cfg.StaleChallengeTTL, err = time.ParseDuration(getEnv("STALE_CHALLENGE_TTL", "5m"))
	if err != nil || cfg.StaleChallengeTTL < time.Minute || cfg.StaleChallengeTTL > 15*time.Minute {
		return nil, errors.New("STALE_CHALLENGE_TTL must be between 1m and 15m")
	}
	return cfg, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

func (c *Config) Validate() error {
	var errs []string

	if c.DBDriver == "postgres" && c.DBHost == "" {
		errs = append(errs, "DB_HOST is required for postgres driver")
	}
	if c.DBPort <= 0 || c.DBPort > 65535 {
		errs = append(errs, "DB_PORT must be between 1 and 65535")
	}
	if c.DBName == "" {
		errs = append(errs, "DB_NAME is required")
	}
	if c.ListenAddr == "" {
		errs = append(errs, "LISTEN_ADDR is required")
	}
	switch c.Environment {
	case "development", "staging", "production":
	default:
		errs = append(errs, "APP_ENV must be one of: development, staging, production")
	}
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.LogLevel] {
		errs = append(errs, "LOG_LEVEL must be one of: debug, info, warn, error")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// DBConfig returns a pkg/db.DatabaseConfig populated from the Config fields.
func (c *Config) DBConfig() db.DatabaseConfig {
	return db.DatabaseConfig{
		Driver:          c.DBDriver,
		Host:            c.DBHost,
		Port:            c.DBPort,
		User:            c.DBUser,
		Password:        c.DBPassword,
		DBName:          c.DBName,
		SSLMode:         c.DBSSLMode,
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

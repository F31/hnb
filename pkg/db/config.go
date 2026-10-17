package db

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DatabaseConfig holds the common database connection parameters used by every
// service that connects to a PostgreSQL (or MySQL) database.
type DatabaseConfig struct {
	Driver          string // "postgres" (default) or "mysql"
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int           // default 25
	MaxIdleConns    int           // default 10
	ConnMaxLifetime time.Duration // default 5m
	ConnMaxIdleTime time.Duration // default 1m
}

// ParsePostgresURL maps a postgres:// URL onto a DatabaseConfig. Deployment
// charts bind the database credential as a single DSN so it never has to appear
// as separate environment variables in a Pod spec, and this lets a service that
// expects discrete fields consume that DSN unchanged.
func ParsePostgresURL(raw string) (DatabaseConfig, error) {
	config := DefaultConfig()
	parsed, err := url.Parse(raw)
	if err != nil {
		return config, fmt.Errorf("parse database URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return config, fmt.Errorf("database URL scheme must be postgres")
	}
	if parsed.Hostname() == "" {
		return config, errors.New("database URL must declare a host")
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return config, errors.New("database URL must declare a user")
	}
	if strings.TrimPrefix(parsed.Path, "/") == "" {
		return config, errors.New("database URL must declare a database")
	}
	config.Host = parsed.Hostname()
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number <= 0 || number > 65535 {
			return config, fmt.Errorf("database URL port is invalid")
		}
		config.Port = number
	}
	config.User = parsed.User.Username()
	if password, ok := parsed.User.Password(); ok {
		config.Password = password
	}
	config.DBName = strings.TrimPrefix(parsed.Path, "/")
	if sslmode := parsed.Query().Get("sslmode"); sslmode != "" {
		config.SSLMode = sslmode
	}
	return config, nil
}

// DefaultConfig returns a DatabaseConfig with sensible defaults.
func DefaultConfig() DatabaseConfig {
	return DatabaseConfig{
		Driver:          "postgres",
		Host:            "localhost",
		Port:            5432,
		User:            "hnb",
		DBName:          "hnb",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}

// DSN returns the PostgreSQL key-value connection string.
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// PostgresDSN is an alias for DSN.
func (c *DatabaseConfig) PostgresDSN() string { return c.DSN() }

// MySQLDSN returns a MySQL DSN for the config. This is a placeholder for
// future MySQL support; the postgres defaults are not meaningful for MySQL.
func (c *DatabaseConfig) MySQLDSN() string {
	tls := "false"
	if c.SSLMode == "require" || c.SSLMode == "verify-ca" || c.SSLMode == "verify-full" {
		tls = "true"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s&parseTime=true&multiStatements=true",
		c.User, c.Password, c.Host, c.Port, c.DBName, tls)
}

// Open opens a database connection and configures the connection pool.
func (c *DatabaseConfig) Open() (*sql.DB, error) {
	driverName := c.Driver
	if driverName == "" {
		driverName = "postgres"
	}

	var dsn string
	switch driverName {
	case "postgres":
		dsn = c.PostgresDSN()
	case "mysql":
		dsn = c.MySQLDSN()
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driverName)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	ConfigurePool(db, c)
	return db, nil
}

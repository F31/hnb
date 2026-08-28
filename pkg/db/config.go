package db

import (
	"database/sql"
	"fmt"
	"time"
)

// DatabaseConfig holds the common database connection parameters used by every
// service that connects to a PostgreSQL (or MySQL) database.
type DatabaseConfig struct {
	Driver          string        // "postgres" (default) or "mysql"
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
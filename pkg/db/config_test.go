package db

import (
	"net/url"
	"strings"
	"testing"
)

// Deployment charts bind the database credential as one DB_DSN so it never has
// to appear as discrete environment variables in a Pod spec. Services that
// expect discrete fields must be able to consume that DSN unchanged.
func TestParsePostgresURLMapsEveryConnectionField(t *testing.T) {
	config, err := ParsePostgresURL("postgres://plane_user:s3cr%40t@db.internal:6543/hnb_plane?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "db.internal" || config.Port != 6543 {
		t.Fatalf("host/port = %s:%d", config.Host, config.Port)
	}
	if config.User != "plane_user" || config.Password != "s3cr@t" {
		t.Fatalf("credentials = %q/%q", config.User, config.Password)
	}
	if config.DBName != "hnb_plane" || config.SSLMode != "require" {
		t.Fatalf("database = %q sslmode = %q", config.DBName, config.SSLMode)
	}
	// Defaults that the caller owns are preserved so the pool stays bounded.
	if config.MaxOpenConns != 25 || config.MaxIdleConns != 10 {
		t.Fatalf("pool defaults lost: %+v", config)
	}
}

func TestParsePostgresURLAppliesDefaultsForOmittedFields(t *testing.T) {
	config, err := ParsePostgresURL("postgresql://hnb@localhost/hnb")
	if err != nil {
		t.Fatal(err)
	}
	if config.Port != 5432 || config.SSLMode != "disable" {
		t.Fatalf("defaults not applied: %+v", config)
	}
}

func TestParsePostgresURLRejectsIncompleteOrWrongSchemes(t *testing.T) {
	for name, raw := range map[string]string{
		"mysql":             "mysql://hnb:pw@localhost:3306/hnb",
		"no host":           "postgres:///hnb",
		"no user":           "postgres://localhost:5432/hnb",
		"no database":       "postgres://hnb@localhost:5432",
		"bad port":          "postgres://hnb@localhost:notaport/hnb",
		"port out of range": "postgres://hnb@localhost:99999/hnb",
		"empty":             "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePostgresURL(raw); err == nil {
				t.Fatalf("invalid DSN %q was accepted", raw)
			}
		})
	}
}

// The DSN must not be silently accepted with a credential that a re-render
// could expose, so verify a round trip preserves the exact password.
func TestParsePostgresURLRoundTripsPasswordExactly(t *testing.T) {
	for _, password := range []string{"p@ss w0rd", "quote\"d", "semi;colon", "100%"} {
		source := DatabaseConfig{Host: "h", Port: 5432, User: "u", Password: password, DBName: "d", SSLMode: "disable"}
		escaped := url.QueryEscape(password)
		escaped = strings.ReplaceAll(escaped, "+", "%20")
		config, err := ParsePostgresURL("postgres://u:" + escaped + "@h:5432/d?sslmode=disable")
		if err != nil {
			t.Fatalf("password %q: %v", password, err)
		}
		if config.Password != source.Password {
			t.Fatalf("password %q became %q", source.Password, config.Password)
		}
	}
}

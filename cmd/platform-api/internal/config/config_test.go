package config

import "testing"

func setRequiredIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("API_TOKEN_ISSUER", "https://issuer.example")
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", "/keys/manifest.json")
	t.Setenv("STALE_CHALLENGE_KEY_FILE", "/keys/stale-challenge.key")
}

func TestLoadDefaults(t *testing.T) {
	setRequiredIdentity(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("listen addr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.DBPort != 5432 {
		t.Fatalf("db port = %d, want 5432", cfg.DBPort)
	}
}

func TestLoadFromEnv(t *testing.T) {
	setRequiredIdentity(t)
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("DB_HOST", "postgres.internal")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "hnb_test")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("listen addr = %q, want :9090", cfg.ListenAddr)
	}
	if cfg.DBHost != "postgres.internal" || cfg.DBPort != 5433 || cfg.DBName != "hnb_test" {
		t.Fatalf("unexpected db config: %+v", cfg)
	}
	want := "host=postgres.internal port=5433 user=hnb password= dbname=hnb_test sslmode=disable"
	if got := cfg.DSN(); got != want {
		t.Fatalf("dsn = %q, want %q", got, want)
	}
}

func TestLoadRequiresPublicVerifierConfiguration(t *testing.T) {
	t.Setenv("API_TOKEN_ISSUER", "")
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing verifier configuration was accepted")
	}
}

func TestLoadRejectsReloadIntervalOverPropagationBound(t *testing.T) {
	setRequiredIdentity(t)
	t.Setenv("API_TOKEN_KEY_RELOAD_INTERVAL", "61s")
	if _, err := Load(); err == nil {
		t.Fatal("polling over 60 seconds was accepted")
	}
}

package config

import "testing"

func TestLoadChartDSN(t *testing.T) {
	want := "postgres://plane:private-pass@postgresql:5432/platform?sslmode=require"
	t.Setenv("DB_DSN", want)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBDSN != want {
		t.Fatal("Secret-bound database URL was not preserved")
	}
}
